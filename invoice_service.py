"""
invoice_service.py — Invoice generation for Callified AI billing.

Generates HTML invoices that can be viewed in-browser and printed/saved as PDF.
"""
from datetime import datetime
from database import get_conn
from typing import List, Dict, Optional
import logging

logger = logging.getLogger("uvicorn.error")


# ─── DB Init ────────────────────────────────────────────────────────────────

def init_invoices_table():
    """Create invoices table. Called from init_billing_tables()."""
    conn = get_conn()
    cursor = conn.cursor()
    cursor.execute('''
        CREATE TABLE IF NOT EXISTS invoices (
            id INT AUTO_INCREMENT PRIMARY KEY,
            org_id INT NOT NULL,
            invoice_number VARCHAR(50) UNIQUE NOT NULL,
            payment_id INT,
            amount_paise BIGINT NOT NULL,
            status ENUM('paid','pending','void') DEFAULT 'paid',
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE
        )
    ''')
    conn.close()


# ─── Invoice Number Generation ──────────────────────────────────────────────

def _generate_invoice_number() -> str:
    """Generate sequential invoice number: CAL-{year}-{sequential}."""
    year = datetime.utcnow().year
    prefix = f"CAL-{year}-"

    conn = get_conn()
    cursor = conn.cursor()
    cursor.execute(
        "SELECT invoice_number FROM invoices WHERE invoice_number LIKE %s ORDER BY id DESC LIMIT 1",
        (f"{prefix}%",)
    )
    row = cursor.fetchone()
    conn.close()

    if row:
        try:
            last_seq = int(row['invoice_number'].split('-')[-1])
            next_seq = last_seq + 1
        except (ValueError, IndexError):
            next_seq = 1
    else:
        next_seq = 1

    return f"{prefix}{next_seq:04d}"


# ─── DB Helpers ─────────────────────────────────────────────────────────────

def create_invoice(org_id: int, payment_id: Optional[int], amount_paise: int) -> dict:
    """Create an invoice record and return it. payment_id may be None for plan-only invoices."""
    invoice_number = _generate_invoice_number()

    conn = get_conn()
    cursor = conn.cursor()
    cursor.execute("""
        INSERT INTO invoices (org_id, invoice_number, payment_id, amount_paise, status)
        VALUES (%s, %s, %s, %s, 'paid')
    """, (org_id, invoice_number, payment_id, amount_paise))
    invoice_id = cursor.lastrowid
    conn.close()

    logger.info(f"[INVOICE] Created {invoice_number} for org={org_id}, amount={amount_paise}")
    return {
        "id": invoice_id,
        "org_id": org_id,
        "invoice_number": invoice_number,
        "payment_id": payment_id,
        "amount_paise": amount_paise,
        "status": "paid",
    }


def get_invoices_by_org(org_id: int) -> List[Dict]:
    """Get all invoices for an organisation.

    Lazy-backfills any captured payments that never received an invoice row
    (e.g. when create_invoice raised inside verify_razorpay_payment and the
    exception was swallowed).  The backfill is idempotent — it only inserts
    rows that are genuinely missing — so calling this on every page-load is safe.
    """
    conn = get_conn()
    cursor = conn.cursor()

    # Find captured payments that have no corresponding invoice row yet
    cursor.execute("""
        SELECT bp.id, bp.amount_paise
        FROM billing_payments bp
        LEFT JOIN invoices i ON i.payment_id = bp.id
        WHERE bp.org_id = %s AND bp.status = 'captured' AND i.id IS NULL
        ORDER BY bp.created_at ASC
    """, (org_id,))
    orphans = cursor.fetchall()
    conn.close()

    for pay in orphans:
        try:
            create_invoice(org_id, pay['id'], pay['amount_paise'])
            logger.info(f"[INVOICE] Backfilled missing invoice for payment_id={pay['id']}, org={org_id}")
        except Exception as e:
            logger.error(f"[INVOICE] Backfill failed for payment_id={pay['id']}, org={org_id}: {e}")

    # Fallback: if still no invoices, synthesize one from the active subscription (covers
    # direct-activation path where no billing_payments row was ever created).
    conn = get_conn()
    cursor = conn.cursor()
    cursor.execute("SELECT COUNT(*) as cnt FROM invoices WHERE org_id = %s", (org_id,))
    if cursor.fetchone()['cnt'] == 0:
        cursor.execute("""
            SELECT s.id as sub_id, s.created_at as sub_created_at,
                   p.price_paise, p.name as plan_name
            FROM subscriptions s
            JOIN billing_plans p ON s.plan_id = p.id
            WHERE s.org_id = %s AND s.status IN ('active', 'trialing', 'past_due')
            ORDER BY s.created_at DESC LIMIT 1
        """, (org_id,))
        sub = cursor.fetchone()
        conn.close()
        if sub:
            try:
                create_invoice(org_id, None, sub['price_paise'])
                logger.info(
                    f"[INVOICE] Synthesized invoice from subscription sub_id={sub['sub_id']} "
                    f"plan='{sub['plan_name']}' org={org_id}"
                )
            except Exception as e:
                logger.error(f"[INVOICE] Synthetic invoice failed for org={org_id}: {e}")
    else:
        conn.close()

    # Return all invoices (including any just backfilled/synthesized)
    conn = get_conn()
    cursor = conn.cursor()
    cursor.execute("""
        SELECT i.*, o.name as org_name
        FROM invoices i
        JOIN organizations o ON i.org_id = o.id
        WHERE i.org_id = %s
        ORDER BY i.created_at DESC
    """, (org_id,))
    rows = cursor.fetchall()
    conn.close()
    return rows


def get_invoice(invoice_id: int) -> Optional[Dict]:
    """Get a single invoice by ID."""
    conn = get_conn()
    cursor = conn.cursor()
    cursor.execute("""
        SELECT i.*, o.name as org_name,
               bp.razorpay_payment_id, bp.description as payment_description,
               p.name as plan_name
        FROM invoices i
        JOIN organizations o ON i.org_id = o.id
        LEFT JOIN billing_payments bp ON i.payment_id = bp.id
        LEFT JOIN subscriptions s ON s.org_id = i.org_id
            AND s.status IN ('active','trialing','past_due')
        LEFT JOIN billing_plans p ON p.id = s.plan_id
        WHERE i.id = %s
        ORDER BY s.created_at DESC
        LIMIT 1
    """, (invoice_id,))
    row = cursor.fetchone()
    conn.close()
    return row


# ─── HTML Invoice Generation ───────────────────────────────────────────────

def generate_invoice_html(
    org_name: str,
    plan_name: str,
    amount_inr: float,
    payment_id: str,
    payment_date: str,
    invoice_number: str,
) -> str:
    """Generate a clean, professional invoice as HTML string."""

    amount_formatted = f"{amount_inr:,.2f}"

    return f"""<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Invoice {invoice_number}</title>
<style>
  @media print {{
    body {{ margin: 0; }}
    .no-print {{ display: none; }}
  }}
  * {{ margin: 0; padding: 0; box-sizing: border-box; }}
  body {{
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    color: #1a1a1a;
    background: #f5f5f5;
    padding: 20px;
  }}
  .invoice-container {{
    max-width: 800px;
    margin: 0 auto;
    background: #fff;
    border: 1px solid #e0e0e0;
    border-radius: 8px;
    overflow: hidden;
  }}
  .header {{
    background: linear-gradient(135deg, #1a73e8, #0d47a1);
    color: #fff;
    padding: 32px 40px;
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
  }}
  .header h1 {{
    font-size: 28px;
    font-weight: 700;
    letter-spacing: -0.5px;
  }}
  .header .company {{
    font-size: 14px;
    opacity: 0.9;
    margin-top: 4px;
  }}
  .header .invoice-meta {{
    text-align: right;
    font-size: 14px;
  }}
  .header .invoice-meta .inv-number {{
    font-size: 18px;
    font-weight: 600;
    margin-bottom: 4px;
  }}
  .body {{
    padding: 32px 40px;
  }}
  .parties {{
    display: flex;
    justify-content: space-between;
    margin-bottom: 32px;
  }}
  .parties .from, .parties .to {{
    width: 48%;
  }}
  .parties h3 {{
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 1px;
    color: #888;
    margin-bottom: 8px;
  }}
  .parties p {{
    font-size: 14px;
    line-height: 1.6;
  }}
  .parties .org-name {{
    font-size: 16px;
    font-weight: 600;
    color: #1a1a1a;
  }}
  table {{
    width: 100%;
    border-collapse: collapse;
    margin-bottom: 24px;
  }}
  thead th {{
    background: #f8f9fa;
    text-align: left;
    padding: 12px 16px;
    font-size: 12px;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    color: #555;
    border-bottom: 2px solid #e0e0e0;
  }}
  thead th:last-child, tbody td:last-child {{
    text-align: right;
  }}
  tbody td {{
    padding: 14px 16px;
    font-size: 14px;
    border-bottom: 1px solid #f0f0f0;
  }}
  .totals {{
    display: flex;
    justify-content: flex-end;
    margin-bottom: 32px;
  }}
  .totals table {{
    width: 280px;
    margin-bottom: 0;
  }}
  .totals td {{
    padding: 8px 16px;
    font-size: 14px;
    border-bottom: 1px solid #f0f0f0;
  }}
  .totals .total-row td {{
    font-size: 18px;
    font-weight: 700;
    color: #1a73e8;
    border-top: 2px solid #1a73e8;
    border-bottom: none;
    padding-top: 12px;
  }}
  .paid-stamp {{
    display: inline-block;
    border: 3px solid #2e7d32;
    color: #2e7d32;
    font-size: 22px;
    font-weight: 800;
    padding: 6px 20px;
    border-radius: 8px;
    transform: rotate(-5deg);
    letter-spacing: 2px;
    opacity: 0.8;
  }}
  .payment-info {{
    background: #f8f9fa;
    border-radius: 6px;
    padding: 16px 20px;
    margin-bottom: 24px;
    font-size: 13px;
    color: #555;
  }}
  .payment-info span {{
    font-weight: 600;
    color: #1a1a1a;
  }}
  .footer {{
    border-top: 1px solid #e0e0e0;
    padding: 16px 40px;
    font-size: 12px;
    color: #999;
    text-align: center;
  }}
  .print-btn {{
    display: block;
    max-width: 800px;
    margin: 16px auto;
    text-align: right;
  }}
  .print-btn button {{
    background: #1a73e8;
    color: #fff;
    border: none;
    padding: 10px 24px;
    border-radius: 6px;
    font-size: 14px;
    cursor: pointer;
  }}
  .print-btn button:hover {{
    background: #1558b0;
  }}
</style>
</head>
<body>

<div class="print-btn no-print">
  <button onclick="window.print()">Print / Save as PDF</button>
</div>

<div class="invoice-container">
  <div class="header">
    <div>
      <h1>INVOICE</h1>
      <div class="company">Callified AI</div>
    </div>
    <div class="invoice-meta">
      <div class="inv-number">{invoice_number}</div>
      <div>Date: {payment_date}</div>
    </div>
  </div>

  <div class="body">
    <div class="parties">
      <div class="from">
        <h3>From</h3>
        <p class="org-name">Callified AI</p>
        <p>by Globussoft Technologies Pvt. Ltd.</p>
      </div>
      <div class="to">
        <h3>Bill To</h3>
        <p class="org-name">{org_name}</p>
      </div>
    </div>

    <table>
      <thead>
        <tr>
          <th>Description</th>
          <th>Qty</th>
          <th>Amount (INR)</th>
        </tr>
      </thead>
      <tbody>
        <tr>
          <td>{plan_name}</td>
          <td>1</td>
          <td>&#8377; {amount_formatted}</td>
        </tr>
      </tbody>
    </table>

    <div class="totals">
      <table>
        <tr>
          <td>Subtotal</td>
          <td>&#8377; {amount_formatted}</td>
        </tr>
        <tr class="total-row">
          <td>Total</td>
          <td>&#8377; {amount_formatted}</td>
        </tr>
      </table>
    </div>

    <div style="margin-bottom: 24px;">
      <span class="paid-stamp">PAID</span>
    </div>

    <div class="payment-info">
      Payment Reference: <span>{payment_id}</span>
    </div>
  </div>

  <div class="footer">
    This is a computer-generated invoice and does not require a signature.
  </div>
</div>

</body>
</html>"""

"""
billing.py — Billing module for Callified AI.

Plans, subscriptions, Razorpay payments, and per-minute usage tracking.
Adapted from EMP Billing patterns (Razorpay gateway + usage metering).
"""
import os
import hmac
import hashlib
import uuid
from datetime import datetime, timedelta
from typing import List, Dict, Optional
from database import get_conn
import logging
import httpx

logger = logging.getLogger("uvicorn.error")

# ─── Razorpay Config ─────────────────────────────────────────────────────────

RAZORPAY_KEY_ID = os.getenv("RAZORPAY_KEY_ID", "")
RAZORPAY_KEY_SECRET = os.getenv("RAZORPAY_KEY_SECRET", "")
RAZORPAY_WEBHOOK_SECRET = os.getenv("RAZORPAY_WEBHOOK_SECRET", "")
RAZORPAY_API = "https://api.razorpay.com/v1"


# ─── DB Init ─────────────────────────────────────────────────────────────────

def init_billing_tables():
    """Create billing tables. Called from init_db() in database.py."""
    from invoice_service import init_invoices_table
    conn = get_conn()
    cursor = conn.cursor()

    cursor.execute('''
        CREATE TABLE IF NOT EXISTS billing_plans (
            id INT AUTO_INCREMENT PRIMARY KEY,
            name VARCHAR(100) NOT NULL,
            description TEXT,
            price_paise BIGINT NOT NULL,
            minutes_included INT NOT NULL DEFAULT 0,
            extra_minute_paise INT NOT NULL DEFAULT 1000,
            billing_interval ENUM('monthly','quarterly','annual') DEFAULT 'monthly',
            trial_days INT DEFAULT 0,
            is_active BOOLEAN DEFAULT TRUE,
            sort_order INT DEFAULT 0,
            features JSON,
            max_campaigns INT DEFAULT NULL,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )
    ''')

    cursor.execute('''
        CREATE TABLE IF NOT EXISTS subscriptions (
            id INT AUTO_INCREMENT PRIMARY KEY,
            org_id INT NOT NULL,
            plan_id INT NOT NULL,
            status ENUM('trialing','active','past_due','cancelled','expired') DEFAULT 'active',
            current_period_start DATETIME,
            current_period_end DATETIME,
            trial_end DATETIME,
            next_billing_date DATE,
            auto_renew BOOLEAN DEFAULT TRUE,
            cancelled_at DATETIME,
            cancel_reason TEXT,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
            FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE,
            FOREIGN KEY (plan_id) REFERENCES billing_plans (id)
        )
    ''')

    cursor.execute('''
        CREATE TABLE IF NOT EXISTS billing_payments (
            id INT AUTO_INCREMENT PRIMARY KEY,
            org_id INT NOT NULL,
            subscription_id INT,
            amount_paise BIGINT NOT NULL,
            currency VARCHAR(10) DEFAULT 'INR',
            status ENUM('created','authorized','captured','failed','refunded') DEFAULT 'created',
            razorpay_order_id VARCHAR(255),
            razorpay_payment_id VARCHAR(255) UNIQUE,
            razorpay_signature VARCHAR(255),
            method VARCHAR(50),
            description TEXT,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE,
            FOREIGN KEY (subscription_id) REFERENCES subscriptions (id) ON DELETE SET NULL
        )
    ''')

    cursor.execute('''
        CREATE TABLE IF NOT EXISTS usage_records (
            id INT AUTO_INCREMENT PRIMARY KEY,
            org_id INT NOT NULL,
            call_id INT,
            minutes_used DECIMAL(10,2) NOT NULL DEFAULT 0,
            recorded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE
        )
    ''')

    conn.close()

    # --- Add max_campaigns to billing_plans if missing ---
    try:
        cursor.execute("ALTER TABLE billing_plans ADD COLUMN max_campaigns INT DEFAULT NULL")
    except Exception:
        pass  # Column already exists
    # Back-fill limits for existing named plans that have none set
    cursor.execute("UPDATE billing_plans SET max_campaigns = 1   WHERE name = 'Starter'    AND max_campaigns IS NULL")
    cursor.execute("UPDATE billing_plans SET max_campaigns = 5   WHERE name = 'Growth'     AND max_campaigns IS NULL")
    # Enterprise stays NULL (unlimited)

    # Create invoices table
    init_invoices_table()


# ─── Seed default plans ──────────────────────────────────────────────────────

def seed_default_plans():
    """Insert default plans if none exist."""
    conn = get_conn()
    cursor = conn.cursor()
    cursor.execute("SELECT COUNT(*) as cnt FROM billing_plans")
    if cursor.fetchone()['cnt'] == 0:
        plans = [
            ("Starter", "1,000 minutes for small teams", 1000000, 1000, 1000, 'monthly', 0, 1,
             '["AI Voice Agent","1 Active campaign","Call recordings & transcripts","CSV lead import","Email support"]', 1),
            ("Growth", "5,000 minutes for scaling teams", 4500000, 5000, 900, 'monthly', 7, 2,
             '["Everything in Starter","Up to 5 concurrent campaigns","CRM integration","Multi-language","Analytics dashboard","Priority support"]', 5),
            ("Enterprise", "20,000 minutes for large teams", 16000000, 20000, 800, 'monthly', 0, 3,
             '["Everything in Growth","Unlimited campaigns","Salesforce + custom CRM","Custom AI voice & persona","Dedicated account manager","SLA & onboarding"]', None),
        ]
        for name, desc, price, mins, extra, interval, trial, sort, features, max_camp in plans:
            cursor.execute(
                "INSERT INTO billing_plans (name, description, price_paise, minutes_included, extra_minute_paise, billing_interval, trial_days, sort_order, features, max_campaigns) VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s)",
                (name, desc, price, mins, extra, interval, trial, sort, features, max_camp)
            )
    conn.close()


# ─── Plan queries ─────────────────────────────────────────────────────────────

def get_all_plans() -> List[Dict]:
    conn = get_conn()
    cursor = conn.cursor()
    cursor.execute("SELECT * FROM billing_plans WHERE is_active = TRUE ORDER BY sort_order")
    rows = cursor.fetchall()
    conn.close()
    return rows


def get_growth_plan_id() -> Optional[int]:
    """Return the plan_id for the Growth plan (used for trial provisioning)."""
    conn = get_conn()
    cursor = conn.cursor()
    cursor.execute("SELECT id FROM billing_plans WHERE name = 'Growth' AND is_active = TRUE LIMIT 1")
    row = cursor.fetchone()
    conn.close()
    return row['id'] if row else None


def get_plan(plan_id: int) -> Optional[Dict]:
    conn = get_conn()
    cursor = conn.cursor()
    cursor.execute("SELECT * FROM billing_plans WHERE id = %s", (plan_id,))
    row = cursor.fetchone()
    conn.close()
    return row


# ─── Subscription management ─────────────────────────────────────────────────

def get_org_subscription(org_id: int) -> Optional[Dict]:
    conn = get_conn()
    cursor = conn.cursor()
    cursor.execute("""
        SELECT s.*, p.name as plan_name, p.minutes_included, p.extra_minute_paise, p.price_paise,
               p.features, p.billing_interval, p.max_campaigns
        FROM subscriptions s
        JOIN billing_plans p ON s.plan_id = p.id
        WHERE s.org_id = %s AND s.status IN ('active','trialing','past_due')
        ORDER BY s.created_at DESC LIMIT 1
    """, (org_id,))
    row = cursor.fetchone()
    conn.close()
    return row


def create_subscription(org_id: int, plan_id: int) -> int:
    plan = get_plan(plan_id)
    if not plan:
        raise ValueError("Plan not found")

    now = datetime.utcnow()
    if plan['trial_days'] > 0:
        status = 'trialing'
        trial_end = now + timedelta(days=plan['trial_days'])
        period_start = now
        period_end = trial_end
        next_billing = trial_end.date()
    else:
        status = 'active'
        trial_end = None
        period_start = now
        if plan['billing_interval'] == 'monthly':
            period_end = now + timedelta(days=30)
        elif plan['billing_interval'] == 'quarterly':
            period_end = now + timedelta(days=90)
        else:
            period_end = now + timedelta(days=365)
        next_billing = period_end.date()

    conn = get_conn()
    cursor = conn.cursor()
    cursor.execute("""
        INSERT INTO subscriptions (org_id, plan_id, status, current_period_start, current_period_end,
                                   trial_end, next_billing_date)
        VALUES (%s, %s, %s, %s, %s, %s, %s)
    """, (org_id, plan_id, status, period_start, period_end, trial_end, next_billing))
    sub_id = cursor.lastrowid
    conn.close()
    return sub_id


def cancel_subscription(subscription_id: int, reason: str = ""):
    conn = get_conn()
    cursor = conn.cursor()
    cursor.execute("""
        UPDATE subscriptions SET status = 'cancelled', cancelled_at = NOW(), cancel_reason = %s
        WHERE id = %s
    """, (reason, subscription_id))
    conn.close()


# ─── Usage tracking ──────────────────────────────────────────────────────────

def record_usage(org_id: int, minutes: float, call_id: int = None):
    conn = get_conn()
    cursor = conn.cursor()
    cursor.execute(
        "INSERT INTO usage_records (org_id, call_id, minutes_used) VALUES (%s, %s, %s)",
        (org_id, call_id, minutes)
    )
    conn.close()


def get_org_usage(org_id: int, period_start: datetime = None, period_end: datetime = None) -> Dict:
    conn = get_conn()
    cursor = conn.cursor()
    if period_start and period_end:
        cursor.execute("""
            SELECT COALESCE(SUM(minutes_used), 0) as total_minutes, COUNT(*) as total_calls
            FROM usage_records WHERE org_id = %s AND recorded_at BETWEEN %s AND %s
        """, (org_id, period_start, period_end))
    else:
        # Current month
        cursor.execute("""
            SELECT COALESCE(SUM(minutes_used), 0) as total_minutes, COUNT(*) as total_calls
            FROM usage_records WHERE org_id = %s
            AND recorded_at >= DATE_FORMAT(NOW(), '%%Y-%%m-01')
        """, (org_id,))
    row = cursor.fetchone()
    conn.close()
    return {
        "total_minutes": float(row['total_minutes']),
        "total_calls": row['total_calls'],
    }


def get_usage_summary(org_id: int) -> Dict:
    """Get usage vs plan limits for the current subscription."""
    sub = get_org_subscription(org_id)
    if not sub:
        return {"has_subscription": False, "total_minutes": 0, "minutes_included": 0, "minutes_remaining": 0}

    usage = get_org_usage(org_id, sub['current_period_start'], sub['current_period_end'])
    minutes_included = sub['minutes_included']
    minutes_used = usage['total_minutes']
    minutes_remaining = max(0, minutes_included - minutes_used)
    overage = max(0, minutes_used - minutes_included)
    overage_cost_paise = int(overage * sub['extra_minute_paise'])

    return {
        "has_subscription": True,
        "plan_name": sub['plan_name'],
        "status": sub['status'],
        "minutes_included": minutes_included,
        "minutes_used": round(minutes_used, 2),
        "minutes_remaining": round(minutes_remaining, 2),
        "overage_minutes": round(overage, 2),
        "overage_cost_paise": overage_cost_paise,
        "period_start": sub['current_period_start'].isoformat() if sub['current_period_start'] else None,
        "period_end": sub['current_period_end'].isoformat() if sub['current_period_end'] else None,
        "next_billing_date": str(sub['next_billing_date']) if sub['next_billing_date'] else None,
    }


# ─── Razorpay Integration ────────────────────────────────────────────────────

def _razorpay_request(method: str, path: str, data: dict = None) -> dict:
    """Make authenticated request to Razorpay API."""
    url = f"{RAZORPAY_API}{path}"
    auth = (RAZORPAY_KEY_ID, RAZORPAY_KEY_SECRET)
    with httpx.Client() as client:
        if method == "POST":
            r = client.post(url, json=data, auth=auth, timeout=30)
        else:
            r = client.get(url, auth=auth, timeout=30)
        r.raise_for_status()
        return r.json()


def create_razorpay_order(org_id: int, plan_id: int) -> Dict:
    """Create a Razorpay order for a plan purchase."""
    plan = get_plan(plan_id)
    if not plan:
        raise ValueError("Plan not found")

    receipt = f"callified_{org_id}_{plan_id}_{int(datetime.utcnow().timestamp())}"
    order = _razorpay_request("POST", "/orders", {
        "amount": plan['price_paise'],
        "currency": "INR",
        "receipt": receipt,
        "notes": {
            "org_id": str(org_id),
            "plan_id": str(plan_id),
            "plan_name": plan['name'],
        }
    })

    # Save payment record
    conn = get_conn()
    cursor = conn.cursor()
    cursor.execute("""
        INSERT INTO billing_payments (org_id, amount_paise, razorpay_order_id, description)
        VALUES (%s, %s, %s, %s)
    """, (org_id, plan['price_paise'], order['id'], f"Plan: {plan['name']}"))
    conn.close()

    return {
        "order_id": order['id'],
        "amount": plan['price_paise'],
        "currency": "INR",
        "key_id": RAZORPAY_KEY_ID,
        "plan_name": plan['name'],
    }


def verify_razorpay_payment(org_id: int, razorpay_order_id: str, razorpay_payment_id: str, razorpay_signature: str, plan_id: int) -> Dict:
    """Verify Razorpay payment signature and activate subscription."""
    # Verify signature
    message = f"{razorpay_order_id}|{razorpay_payment_id}"
    expected = hmac.new(
        RAZORPAY_KEY_SECRET.encode(),
        message.encode(),
        hashlib.sha256
    ).hexdigest()

    if expected != razorpay_signature:
        raise ValueError("Invalid payment signature")

    # Update payment record
    conn = get_conn()
    cursor = conn.cursor()

    # Prevent duplicate
    cursor.execute("SELECT id FROM billing_payments WHERE razorpay_payment_id = %s", (razorpay_payment_id,))
    if cursor.fetchone():
        conn.close()
        return {"status": "already_processed"}

    cursor.execute("""
        UPDATE billing_payments SET razorpay_payment_id = %s, razorpay_signature = %s, status = 'captured'
        WHERE razorpay_order_id = %s AND org_id = %s
    """, (razorpay_payment_id, razorpay_signature, razorpay_order_id, org_id))
    conn.close()

    # Create or renew subscription
    sub = get_org_subscription(org_id)
    if sub:
        # Renew existing
        plan = get_plan(plan_id)
        now = datetime.utcnow()
        if plan['billing_interval'] == 'monthly':
            new_end = now + timedelta(days=30)
        elif plan['billing_interval'] == 'quarterly':
            new_end = now + timedelta(days=90)
        else:
            new_end = now + timedelta(days=365)

        conn = get_conn()
        cursor = conn.cursor()
        cursor.execute("""
            UPDATE subscriptions SET plan_id = %s, status = 'active',
                   current_period_start = %s, current_period_end = %s,
                   next_billing_date = %s
            WHERE id = %s
        """, (plan_id, now, new_end, new_end.date(), sub['id']))
        conn.close()
        sub_id = sub['id']
    else:
        sub_id = create_subscription(org_id, plan_id)

    # Auto-create invoice for this payment.
    # Failure here is non-fatal — get_invoices_by_org will backfill on the
    # next billing page load — but we log at ERROR so it's never silent.
    try:
        from invoice_service import create_invoice
        conn = get_conn()
        cursor = conn.cursor()
        cursor.execute(
            "SELECT id, amount_paise FROM billing_payments WHERE razorpay_payment_id = %s",
            (razorpay_payment_id,)
        )
        pay_row = cursor.fetchone()
        conn.close()
        if pay_row:
            create_invoice(org_id, pay_row['id'], pay_row['amount_paise'])
        else:
            logger.error(
                f"[BILLING] Invoice skipped — payment row not found for "
                f"razorpay_payment_id={razorpay_payment_id}, org={org_id}. "
                f"Will be backfilled on next billing page load."
            )
    except Exception as e:
        logger.error(
            f"[BILLING] Invoice creation failed for org={org_id}, "
            f"payment={razorpay_payment_id}: {e}. "
            f"Will be backfilled on next billing page load."
        )

    logger.info(f"[BILLING] Payment verified: org={org_id}, plan={plan_id}, payment={razorpay_payment_id}")
    return {"status": "success", "subscription_id": sub_id}


def handle_razorpay_webhook(headers: dict, raw_body: bytes) -> Dict:
    """Handle Razorpay webhook events."""
    received_sig = headers.get("x-razorpay-signature", "")
    expected_sig = hmac.new(
        RAZORPAY_WEBHOOK_SECRET.encode(),
        raw_body,
        hashlib.sha256
    ).hexdigest()

    if expected_sig != received_sig:
        raise ValueError("Invalid webhook signature")

    import json
    event = json.loads(raw_body)
    event_type = event.get("event", "")

    if event_type == "payment.captured":
        payment = event.get("payload", {}).get("payment", {}).get("entity", {})
        logger.info(f"[BILLING WEBHOOK] Payment captured: {payment.get('id')}, amount={payment.get('amount')}")

    elif event_type == "payment.failed":
        payment = event.get("payload", {}).get("payment", {}).get("entity", {})
        razorpay_order_id = payment.get("order_id", "")
        if razorpay_order_id:
            conn = get_conn()
            cursor = conn.cursor()
            cursor.execute("UPDATE billing_payments SET status = 'failed' WHERE razorpay_order_id = %s", (razorpay_order_id,))
            conn.close()
        logger.warning(f"[BILLING WEBHOOK] Payment failed: {payment.get('id')}")

    return {"acknowledged": True}


# ─── Payment history ─────────────────────────────────────────────────────────

def get_payment_history(org_id: int) -> List[Dict]:
    conn = get_conn()
    cursor = conn.cursor()
    cursor.execute("""
        SELECT bp.*, bl.name as plan_name
        FROM billing_payments bp
        LEFT JOIN subscriptions s ON bp.subscription_id = s.id
        LEFT JOIN billing_plans bl ON s.plan_id = bl.id
        WHERE bp.org_id = %s ORDER BY bp.created_at DESC LIMIT 50
    """, (org_id,))
    rows = cursor.fetchall()
    conn.close()
    return rows

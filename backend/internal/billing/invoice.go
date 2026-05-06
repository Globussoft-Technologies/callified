package billing

import (
	"bytes"
	"fmt"
	"html/template"
)

// InvoiceData carries everything the template needs. Fields with no value
// render as blanks — they don't break the layout.
type InvoiceData struct {
	InvoiceNumber string
	Date          string
	Status        string // "Paid" | "Pending"
	OrgName       string
	OrgEmail      string
	PlanName      string
	PeriodStart   string
	PeriodEnd     string
	BillingCycle  string

	// Money. Pass total as paise; the helpers compute breakdown.
	Subtotal string // ₹ (taxable value, before GST)
	GST      string // ₹ (IGST 18%)
	GSTRate  string // "18%"
	Total    string // ₹

	PaymentID string
}

// NewInvoiceData computes the GST breakdown assuming the stored amount is
// inclusive of 18% IGST (typical Indian SaaS invoicing). If you switch to
// GST-exclusive pricing later, change the math here.
func NewInvoiceData(amountPaise int64) (subtotal, gst, total string) {
	totalRupees := float64(amountPaise) / 100.0
	subtotalRupees := totalRupees / 1.18
	gstRupees := totalRupees - subtotalRupees
	return fmt.Sprintf("%.2f", subtotalRupees),
		fmt.Sprintf("%.2f", gstRupees),
		fmt.Sprintf("%.2f", totalRupees)
}

const invoiceTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Invoice {{.InvoiceNumber}}</title>
<style>
  *{box-sizing:border-box}
  body{font-family:-apple-system,Segoe UI,Roboto,sans-serif;max-width:800px;margin:32px auto;padding:32px;color:#1f2937;background:#fff}
  .header{display:flex;justify-content:space-between;align-items:flex-start;border-bottom:2px solid #6366f1;padding-bottom:20px}
  h1{color:#6366f1;margin:0;font-size:28px}
  .tagline{color:#6b7280;font-size:13px;margin-top:4px}
  .company-info{font-size:11px;color:#6b7280;margin-top:8px;line-height:1.5}
  .meta{text-align:right;font-size:13px;color:#374151}
  .meta b{color:#111827}
  .badge{display:inline-block;padding:2px 10px;border-radius:12px;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:0.5px}
  .badge-paid{background:#d1fae5;color:#065f46}
  .badge-pending{background:#fef3c7;color:#92400e}
  .parties{display:flex;justify-content:space-between;margin:24px 0;gap:24px}
  .party{flex:1;font-size:13px;line-height:1.6}
  .party h3{font-size:11px;color:#6b7280;text-transform:uppercase;letter-spacing:1px;margin:0 0 6px 0}
  .party .name{font-weight:600;color:#111827;font-size:15px}
  table{width:100%;border-collapse:collapse;margin-top:16px}
  th{background:#f9fafb;padding:10px 12px;text-align:left;font-size:11px;color:#6b7280;text-transform:uppercase;letter-spacing:0.5px;border-bottom:1px solid #e5e7eb}
  th.right,td.right{text-align:right}
  td{padding:14px 12px;border-bottom:1px solid #f3f4f6;font-size:14px;vertical-align:top}
  .desc-sub{display:block;color:#6b7280;font-size:12px;margin-top:2px}
  .totals{margin-top:0;width:auto;float:right;min-width:280px}
  .totals td{border:none;padding:6px 12px}
  .totals .label{color:#6b7280}
  .totals .value{text-align:right;color:#111827}
  .totals .grand{border-top:2px solid #1f2937;padding-top:10px;margin-top:8px}
  .totals .grand td{font-size:18px;font-weight:700;color:#111827;padding-top:12px}
  .clearfix::after{content:"";display:table;clear:both}
  .footer{margin-top:48px;padding-top:20px;border-top:1px solid #e5e7eb;font-size:11px;color:#9ca3af;text-align:center;line-height:1.6}
</style>
</head>
<body>
<div class="header">
  <div>
    <h1>Callified AI</h1>
    <div class="tagline">AI-Powered Sales Dialer</div>
    <div class="company-info">
      Globussoft Technologies Pvt. Ltd.<br>
      <!--email_off-->support@callified.ai<!--/email_off-->
    </div>
  </div>
  <div class="meta">
    <div style="font-size:18px;font-weight:700;color:#111827;margin-bottom:6px">TAX INVOICE</div>
    <div><b>{{.InvoiceNumber}}</b></div>
    <div>Date: {{.Date}}</div>
    {{if .Status}}<div style="margin-top:8px"><span class="badge badge-{{if eq .Status "Paid"}}paid{{else}}pending{{end}}">{{.Status}}</span></div>{{end}}
  </div>
</div>

<div class="parties">
  <div class="party">
    <h3>Billed To</h3>
    <div class="name">{{.OrgName}}</div>
    {{if .OrgEmail}}<div>{{.OrgEmail}}</div>{{end}}
  </div>
  <div class="party" style="text-align:right">
    {{if .PeriodStart}}
    <h3>Billing Period</h3>
    <div>{{.PeriodStart}} — {{.PeriodEnd}}</div>
    {{if .BillingCycle}}<div style="color:#6b7280;font-size:12px;margin-top:2px">{{.BillingCycle}}</div>{{end}}
    {{end}}
  </div>
</div>

<table>
  <thead>
    <tr>
      <th>Description</th>
      <th class="right">Amount</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td>
        {{.PlanName}} Plan — Subscription
        {{if .PeriodStart}}<span class="desc-sub">{{.PeriodStart}} to {{.PeriodEnd}}</span>{{end}}
      </td>
      <td class="right">₹{{.Subtotal}}</td>
    </tr>
  </tbody>
</table>

<div class="clearfix">
  <table class="totals">
    <tr><td class="label">Subtotal</td><td class="value">₹{{.Subtotal}}</td></tr>
    <tr><td class="label">IGST ({{.GSTRate}})</td><td class="value">₹{{.GST}}</td></tr>
    <tr class="grand"><td>Total</td><td class="value">₹{{.Total}}</td></tr>
  </table>
</div>

<div class="footer">
  {{if .PaymentID}}Payment Reference: {{.PaymentID}}<br>{{end}}
  This is a computer-generated invoice and does not require a signature.<br>
  For any billing queries, contact <!--email_off-->support@callified.ai<!--/email_off-->.
</div>
</body>
</html>`

// GenerateInvoiceHTML renders an invoice as an HTML string.
func GenerateInvoiceHTML(d InvoiceData) string {
	if d.GSTRate == "" {
		d.GSTRate = "18%"
	}
	if d.Status == "" {
		d.Status = "Paid"
	}
	t, err := template.New("invoice").Parse(invoiceTmpl)
	if err != nil {
		return fmt.Sprintf("<p>Invoice %s — %s — ₹%s</p>", d.InvoiceNumber, d.PlanName, d.Total)
	}
	var buf bytes.Buffer
	_ = t.Execute(&buf, d)
	return buf.String()
}

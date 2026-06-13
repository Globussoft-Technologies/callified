package billing

import (
	"bytes"
	"fmt"

	"github.com/go-pdf/fpdf"
)

// GenerateInvoicePDF renders an invoice as a one-page A4 PDF.
//
// Why PDF, not HTML: the HTML version is rewritten in transit by Cloudflare's
// email-obfuscation pass (it ignores our `<!--email_off-->` markers when
// delivering the response through their proxy), so support@callified.ai
// rendered as "[email protected]" on the client. Cloudflare obfuscation
// only runs on `text/html`; serving `application/pdf` bypasses it cleanly.
//
// Layout intentionally mirrors the HTML invoice (same header colour, same
// "BILLED TO" / "BILLING PERIOD" split, same totals box, same footer)
// so the user sees the same document in both flows.
func GenerateInvoicePDF(d InvoiceData) []byte {
	if d.GSTRate == "" {
		d.GSTRate = "18%"
	}
	if d.Status == "" {
		d.Status = "Paid"
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(20, 18, 20)
	pdf.SetAutoPageBreak(true, 18)
	pdf.AddPage()

	// fpdf's default font (Helvetica) doesn't have the rupee glyph; use "Rs."
	// inline so the totals don't render as "?100.00". Plain Latin1 throughout.
	rupee := "Rs. "

	// ── Header ────────────────────────────────────────────────────────────────
	// Brand name in indigo (#6366f1)
	pdf.SetTextColor(99, 102, 241)
	pdf.SetFont("Helvetica", "B", 22)
	pdf.SetXY(20, 18)
	pdf.Cell(110, 9, "Callified AI")

	pdf.SetTextColor(107, 114, 128)
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetXY(20, 28)
	pdf.Cell(110, 5, "AI-Powered Sales Dialer")

	pdf.SetTextColor(75, 85, 99)
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetXY(20, 36)
	pdf.Cell(110, 4.5, "Globussoft Technologies Pvt. Ltd.")
	pdf.SetXY(20, 40.5)
	pdf.Cell(110, 4.5, "support@callified.ai")

	// Right-aligned meta block (TAX INVOICE label, number, date, status)
	pdf.SetTextColor(17, 24, 39)
	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetXY(120, 18)
	pdf.CellFormat(70, 8, "TAX INVOICE", "", 0, "R", false, 0, "")

	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetXY(120, 26.5)
	pdf.CellFormat(70, 5, d.InvoiceNumber, "", 0, "R", false, 0, "")

	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(75, 85, 99)
	pdf.SetXY(120, 32)
	pdf.CellFormat(70, 5, "Date: "+d.Date, "", 0, "R", false, 0, "")

	if d.Status != "" {
		drawStatusBadge(pdf, 165, 38, 25, 6, d.Status)
	}

	// Indigo divider under header
	pdf.SetDrawColor(99, 102, 241)
	pdf.SetLineWidth(0.6)
	pdf.Line(20, 50, 190, 50)

	// ── Billed To / Billing Period ────────────────────────────────────────────
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(107, 114, 128)
	pdf.SetXY(20, 58)
	pdf.Cell(85, 5, "BILLED TO")

	pdf.SetFont("Helvetica", "B", 13)
	pdf.SetTextColor(17, 24, 39)
	pdf.SetXY(20, 64)
	pdf.Cell(85, 6, d.OrgName)

	if d.OrgEmail != "" {
		pdf.SetFont("Helvetica", "", 9)
		pdf.SetTextColor(75, 85, 99)
		pdf.SetXY(20, 71)
		pdf.Cell(85, 4.5, d.OrgEmail)
	}

	if d.PeriodStart != "" {
		pdf.SetFont("Helvetica", "B", 9)
		pdf.SetTextColor(107, 114, 128)
		pdf.SetXY(105, 58)
		pdf.CellFormat(85, 5, "BILLING PERIOD", "", 0, "R", false, 0, "")

		pdf.SetFont("Helvetica", "", 10)
		pdf.SetTextColor(17, 24, 39)
		pdf.SetXY(105, 64)
		pdf.CellFormat(85, 6, d.PeriodStart+" — "+d.PeriodEnd, "", 0, "R", false, 0, "")

		if d.BillingCycle != "" {
			pdf.SetFont("Helvetica", "", 9)
			pdf.SetTextColor(107, 114, 128)
			pdf.SetXY(105, 70)
			pdf.CellFormat(85, 4.5, d.BillingCycle, "", 0, "R", false, 0, "")
		}
	}

	// ── Line-item table ───────────────────────────────────────────────────────
	tableY := 86.0
	// Header row (gray bg)
	pdf.SetFillColor(249, 250, 251)
	pdf.SetTextColor(107, 114, 128)
	pdf.SetFont("Helvetica", "B", 9)
	pdf.Rect(20, tableY, 170, 9, "F")
	pdf.SetXY(24, tableY+2)
	pdf.Cell(120, 5, "DESCRIPTION")
	pdf.SetXY(140, tableY+2)
	pdf.CellFormat(46, 5, "AMOUNT", "", 0, "R", false, 0, "")

	// Body row
	pdf.SetTextColor(17, 24, 39)
	pdf.SetFont("Helvetica", "", 10)
	descLabel := d.PlanName
	if descLabel == "" {
		descLabel = "Call Credits"
	}
	pdf.SetXY(24, tableY+12)
	pdf.Cell(120, 5, descLabel+" — Subscription")
	pdf.SetXY(140, tableY+12)
	pdf.CellFormat(46, 5, rupee+d.Subtotal, "", 0, "R", false, 0, "")

	if d.PeriodStart != "" {
		pdf.SetTextColor(107, 114, 128)
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetXY(24, tableY+18)
		pdf.Cell(120, 4, d.PeriodStart+" to "+d.PeriodEnd)
	}

	// ── Totals box (right-aligned) ────────────────────────────────────────────
	totalsY := tableY + 30
	pdf.SetDrawColor(229, 231, 235)
	pdf.SetLineWidth(0.2)

	pdf.SetTextColor(75, 85, 99)
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetXY(110, totalsY)
	pdf.CellFormat(50, 6, "Subtotal", "", 0, "R", false, 0, "")
	pdf.SetTextColor(17, 24, 39)
	pdf.SetXY(160, totalsY)
	pdf.CellFormat(30, 6, rupee+d.Subtotal, "", 0, "R", false, 0, "")

	pdf.SetTextColor(75, 85, 99)
	pdf.SetXY(110, totalsY+7)
	pdf.CellFormat(50, 6, fmt.Sprintf("IGST (%s)", d.GSTRate), "", 0, "R", false, 0, "")
	pdf.SetTextColor(17, 24, 39)
	pdf.SetXY(160, totalsY+7)
	pdf.CellFormat(30, 6, rupee+d.GST, "", 0, "R", false, 0, "")

	// Divider above grand total
	pdf.Line(110, totalsY+15, 190, totalsY+15)

	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetTextColor(17, 24, 39)
	pdf.SetXY(110, totalsY+17)
	pdf.CellFormat(50, 7, "Total", "", 0, "R", false, 0, "")
	pdf.SetXY(160, totalsY+17)
	pdf.CellFormat(30, 7, rupee+d.Total, "", 0, "R", false, 0, "")

	// ── Footer ────────────────────────────────────────────────────────────────
	footerY := totalsY + 35
	pdf.SetDrawColor(229, 231, 235)
	pdf.Line(20, footerY, 190, footerY)

	pdf.SetTextColor(107, 114, 128)
	pdf.SetFont("Helvetica", "", 9)
	if d.PaymentID != "" {
		pdf.SetXY(20, footerY+5)
		pdf.CellFormat(170, 5, "Payment Reference: "+d.PaymentID, "", 0, "C", false, 0, "")
	}
	pdf.SetXY(20, footerY+11)
	pdf.CellFormat(170, 5, "This is a computer-generated invoice and does not require a signature.", "", 0, "C", false, 0, "")
	pdf.SetXY(20, footerY+17)
	pdf.CellFormat(170, 5, "For any billing queries, contact support@callified.ai.", "", 0, "C", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		// Fall back to a one-line text PDF so the download isn't a 500.
		errPdf := fpdf.New("P", "mm", "A4", "")
		errPdf.AddPage()
		errPdf.SetFont("Helvetica", "", 11)
		errPdf.Cell(0, 10, fmt.Sprintf("Invoice %s — %s — Rs.%s", d.InvoiceNumber, d.PlanName, d.Total))
		buf.Reset()
		_ = errPdf.Output(&buf)
	}
	return buf.Bytes()
}

// drawStatusBadge renders a small rounded "Paid" / "Pending" pill matching
// the HTML invoice's badge styling. width/height are in mm.
func drawStatusBadge(pdf *fpdf.Fpdf, x, y, w, h float64, status string) {
	var fillR, fillG, fillB int
	var textR, textG, textB int
	switch status {
	case "Paid", "paid":
		fillR, fillG, fillB = 209, 250, 229 // emerald-100
		textR, textG, textB = 6, 95, 70     // emerald-800
	case "Pending", "pending":
		fillR, fillG, fillB = 254, 243, 199 // amber-100
		textR, textG, textB = 146, 64, 14   // amber-800
	default:
		fillR, fillG, fillB = 243, 244, 246 // gray-100
		textR, textG, textB = 55, 65, 81    // gray-700
	}
	pdf.SetFillColor(fillR, fillG, fillB)
	// rounded rect — fpdf has RoundedRect since v0.5
	pdf.RoundedRect(x, y, w, h, h/2, "1234", "F")
	pdf.SetTextColor(textR, textG, textB)
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetXY(x, y)
	pdf.CellFormat(w, h, status, "", 0, "C", false, 0, "")
}

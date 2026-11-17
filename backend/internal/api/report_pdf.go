package api

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-pdf/fpdf"
	"github.com/xuri/excelize/v2"
	"golang.org/x/image/font/gofont/goregular"
)

// Render the same workbook data in a paginated, readable PDF. Records are
// stacked rather than squeezed into tables with dozens of narrow columns.
func writeReportPDF(w http.ResponseWriter, r *http.Request, workbook *excelize.File, filename string) error {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(16, 16, 16)
	pdf.SetAutoPageBreak(true, 16)
	pdf.AddUTF8FontFromBytes("report", "", goregular.TTF)
	pdf.SetFont("report", "", 10)
	for _, sheet := range workbook.GetSheetList() {
		if err := r.Context().Err(); err != nil {
			return err
		}
		rows, err := workbook.GetRows(sheet)
		if err != nil {
			return err
		}
		pdf.AddPage()
		pdf.SetFontSize(16)
		pdf.MultiCell(0, 9, "Callified — "+sheet, "", "L", false)
		pdf.SetFontSize(10)
		if len(rows) < 2 {
			pdf.MultiCell(0, 6, "No records.", "", "L", false)
			continue
		}
		for i, row := range rows[1:] {
			if err := r.Context().Err(); err != nil {
				return err
			}
			pdf.Ln(4)
			pdf.SetTextColor(99, 102, 241)
			pdf.MultiCell(0, 6, fmt.Sprintf("Record %d", i+1), "", "L", false)
			pdf.SetTextColor(25, 30, 40)
			for col, value := range row {
				if value == "" {
					continue
				}
				label := fmt.Sprintf("Column %d", col+1)
				if col < len(rows[0]) {
					label = rows[0][col]
				}
				pdf.MultiCell(0, 5, label+": "+value, "", "L", false)
				cell, _ := excelize.CoordinatesToCellName(col+1, i+2)
				linked, target, err := workbook.GetCellHyperLink(sheet, cell)
				if err != nil {
					return err
				}
				if !linked && (strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://")) {
					target = value
				}
				if strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "http://") {
					pdf.SetTextColor(55, 70, 200)
					pdf.WriteLinkString(5, "Open link", target)
					pdf.Ln(6)
					pdf.SetTextColor(25, 30, 40)
				}
			}
		}
	}
	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.pdf"`, filename))
	_, err := w.Write(output.Bytes())
	return err
}

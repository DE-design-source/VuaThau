// Package xlsx: ghi bảng thành file Excel .xlsx tối giản (1 sheet, chữ + số, dòng đầu in đậm), không cần thư viện ngoài.
package xlsx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

func col(i int) string {
	if i >= 26 {
		return col(i/26-1) + string(rune('A'+i%26))
	}
	return string(rune('A' + i))
}

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func cell(r, k int, v any) string {
	ref, bold := fmt.Sprintf("%s%d", col(k), r), ""
	if r == 1 {
		bold = ` s="1"`
	}
	switch n := v.(type) {
	case int:
		return fmt.Sprintf(`<c r="%s"%s><v>%d</v></c>`, ref, bold, n)
	case int64:
		return fmt.Sprintf(`<c r="%s"%s><v>%d</v></c>`, ref, bold, n)
	case float64:
		return fmt.Sprintf(`<c r="%s"%s><v>%g</v></c>`, ref, bold, n)
	}
	return fmt.Sprintf(`<c r="%s" t="inlineStr"%s><is><t xml:space="preserve">%s</t></is></c>`, ref, bold, esc(fmt.Sprint(v)))
}

// Build: rows[0] là dòng tiêu đề. Giá trị int / int64 / float64 ghi thành số, còn lại thành chữ.
func Build(rows [][]any) ([]byte, error) {
	n := 1
	var sheet strings.Builder
	for i, row := range rows {
		n = max(n, len(row))
		fmt.Fprintf(&sheet, `<row r="%d">`, i+1)
		for k, v := range row {
			sheet.WriteString(cell(i+1, k, v))
		}
		sheet.WriteString(`</row>`)
	}
	const head = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`
	const ns = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"`
	const rel = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	const ct = "application/vnd.openxmlformats-officedocument.spreadsheetml"
	files := []struct{ name, body string }{
		{"[Content_Types].xml", head + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/xl/workbook.xml" ContentType="` + ct + `.sheet.main+xml"/>` +
			`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="` + ct + `.worksheet+xml"/>` +
			`<Override PartName="/xl/styles.xml" ContentType="` + ct + `.styles+xml"/></Types>`},
		{"_rels/.rels", head + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="` + rel + `/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", head + `<workbook ` + ns + ` xmlns:r="` + rel + `"><sheets><sheet name="Vua Thau" sheetId="1" r:id="rId1"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", head + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="` + rel + `/worksheet" Target="worksheets/sheet1.xml"/>` +
			`<Relationship Id="rId2" Type="` + rel + `/styles" Target="styles.xml"/></Relationships>`},
		{"xl/styles.xml", head + `<styleSheet ` + ns + `><fonts count="2"><font><sz val="11"/><name val="Calibri"/></font>` +
			`<font><b/><sz val="11"/><name val="Calibri"/></font></fonts><fills count="1"><fill><patternFill patternType="none"/></fill>` +
			`</fills><borders count="1"><border/></borders><cellStyleXfs count="1"><xf/></cellStyleXfs>` +
			`<cellXfs count="2"><xf/><xf fontId="1" applyFont="1"/></cellXfs></styleSheet>`},
		{"xl/worksheets/sheet1.xml", head + `<worksheet ` + ns + `><cols><col min="1" max="` + fmt.Sprint(n) +
			`" width="20" customWidth="1"/></cols><sheetData>` + sheet.String() + `</sheetData></worksheet>`},
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := z.Create(f.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(f.body)); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

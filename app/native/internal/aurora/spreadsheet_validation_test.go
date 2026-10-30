package aurora

import (
	"archive/zip"
	"bytes"
	"io"
	"sort"
	"strings"
	"testing"
)

const sheetNS = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
const sheetRelNS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

func sheetPackage(body string) map[string]string {
	return map[string]string{
		"[Content_Types].xml":        `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`,
		"_rels/.rels":                `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="root" Type="` + sheetRelNS + `/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<workbook xmlns="` + sheetNS + `" xmlns:r="` + sheetRelNS + `"><sheets><sheet name="Report" sheetId="1" r:id="one"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="one" Type="` + sheetRelNS + `/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   `<worksheet xmlns="` + sheetNS + `"><sheetData>` + body + `</sheetData></worksheet>`,
	}
}

func sheetArchive(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.WriteString(w, parts[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestSpreadsheetCSVRequiresCompleteRecords(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		ok         bool
	}{
		{"quoted comma", "name,value\n\"a,b\",2\n", true},
		{"quoted newline", "name,value\n\"a\nb\",2\n", true},
		{"escaped quote", "name,value\n\"a\"\"b\",2\n", true},
		{"semicolon with quoted commas", "name;value\n\"a,b,c\";2\n", true},
		{"tab and BOM", "\ufeffname\tvalue\r\na\t2\r\n", true},
		{"missing field among otherwise valid rows", "a,b\n1,2\n3,4\n5,6\n7\n", false},
		{"unclosed quote", "a,b\n\"unfinished,2\n", false},
		{"bare quote", "a,b\nun\"quoted,2\n", false},
		{"invalid UTF8", "a,b\n\xff,2\n", false},
		{"header only", "a,b\n", false},
		{"single column", "a\n1\n", false},
		{"over budget", "a,b\n" + strings.Repeat("1,2\n", (2<<20)/4), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := checkSheet(strings.NewReader(tc.data), int64(len(tc.data)), ".csv")
			if v.ok != tc.ok {
				t.Fatalf("accepted=%v want=%v (%s)", v.ok, tc.ok, v.why)
			}
		})
	}
	data := "a,b\n1,2\n"
	if v := checkSheet(strings.NewReader(data), int64(len(data)+100), ".csv"); v.ok {
		t.Fatal("short read accepted as complete CSV")
	}
}

func TestSpreadsheetXLSXRequiresLinkedParsedRows(t *testing.T) {
	valid := `<row r="1"><c r="A1"><v>1</v></c></row><row r="2"><c r="A2"><v>2</v></c></row>`
	for _, tc := range []struct {
		name   string
		mutate func(map[string]string)
		ok     bool
	}{
		{"valid package", func(p map[string]string) {}, true},
		{"truncated XML", func(p map[string]string) { p["xl/worksheets/sheet1.xml"] = "<worksheet>" + valid }, false},
		{"missing relationships", func(p map[string]string) { delete(p, "xl/_rels/workbook.xml.rels") }, false},
		{"comments are not rows", func(p map[string]string) {
			p["xl/worksheets/sheet1.xml"] = `<worksheet xmlns="` + sheetNS + `"><sheetData><!-- <row/><row/> --></sheetData></worksheet>`
		}, false},
		{"empty rows", func(p map[string]string) {
			p["xl/worksheets/sheet1.xml"] = `<worksheet xmlns="` + sheetNS + `"><sheetData><row r="1"/><row r="2"/></sheetData></worksheet>`
		}, false},
		{"same cell twice", func(p map[string]string) {
			p["xl/worksheets/sheet1.xml"] = strings.ReplaceAll(p["xl/worksheets/sheet1.xml"], `r="A2"`, `r="A1"`)
		}, false},
		{"external worksheet", func(p map[string]string) {
			p["xl/_rels/workbook.xml.rels"] = strings.Replace(p["xl/_rels/workbook.xml.rels"], `Target="worksheets/sheet1.xml"`, `Target="https://example.invalid/sheet.xml" TargetMode="External"`, 1)
		}, false},
		{"orphan sheet cannot prove workbook", func(p map[string]string) {
			p["xl/worksheets/sheet2.xml"] = p["xl/worksheets/sheet1.xml"]
			p["xl/worksheets/sheet1.xml"] = `<worksheet xmlns="` + sheetNS + `"><sheetData/></worksheet>`
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parts := sheetPackage(valid)
			tc.mutate(parts)
			raw := sheetArchive(t, parts)
			v := checkSheet(bytes.NewReader(raw), int64(len(raw)), ".xlsx")
			if v.ok != tc.ok {
				t.Fatalf("accepted=%v want=%v (%s)", v.ok, tc.ok, v.why)
			}
		})
	}
}

func TestSpreadsheetChecklistRevokesChangedArtifactProof(t *testing.T) {
	svc := quietSvc(t)
	bot := firstBot(svc)
	svc.newRequest(bot, "Tạo bảng tính xlsx", true)
	write := func(content string) {
		t.Helper()
		if err := svc.PC.WriteFile(bot, "out/report.xlsx", content); err != nil {
			t.Fatal(err)
		}
		svc.refreshChecklist(bot)
	}
	write("not a workbook")
	if len(svc.openItems(bot)) != 1 {
		t.Fatal("invalid workbook closed the checklist")
	}
	write(string(xlsx(t, 3)))
	if len(svc.openItems(bot)) != 0 {
		t.Fatal("valid workbook did not close the checklist")
	}
	proof, _, err := svc.PC.ReadFile(bot, "verify.txt")
	if err != nil || !strings.Contains(proof, "✓ bảng tính") {
		t.Fatal("verified file not recorded", proof, err)
	}
	// A new service has no in-memory verifyTxt, but the old report is on disk.
	svc.pcDo(bot, func(st *pcState) { *st = pcState{} })
	svc.newRequest(bot, "Tạo bảng tính xlsx", true)
	write("workbook changed after verification")
	if len(svc.openItems(bot)) != 1 {
		t.Fatal("changed workbook kept stale completion")
	}
	proof, _, err = svc.PC.ReadFile(bot, "verify.txt")
	if err != nil || strings.Contains(proof, "✓ bảng tính") || !strings.Contains(proof, "✗ bảng tính") {
		t.Fatal("persisted verification did not revoke changed artifact", proof, err)
	}
}

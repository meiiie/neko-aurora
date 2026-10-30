package documents

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const officeTestSpreadsheetNS = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
const officeTestPresentationNS = "http://schemas.openxmlformats.org/presentationml/2006/main"
const officeTestDrawingNS = "http://schemas.openxmlformats.org/drawingml/2006/main"
const officeTestRelNS = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
const officeTestSheetType = "application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"
const officeTestWorkbookType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"
const officeTestSlideType = "application/vnd.openxmlformats-officedocument.presentationml.slide+xml"
const officeTestPresentationType = "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"

func officeTestArchive(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	names := make([]string, 0, len(parts))
	for name := range parts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		stream, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = stream.Write([]byte(parts[name])); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func officeTestTypes(overrides map[string]string) string {
	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}
	sort.Strings(names)
	text := `<Types xmlns="` + contentTypesNamespace + `"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/>`
	for _, name := range names {
		text += `<Override PartName="/` + name + `" ContentType="` + overrides[name] + `"/>`
	}
	return text + `</Types>`
}

func officeTestRootRelationships(main string) string {
	return `<Relationships xmlns="` + relationshipsNamespace + `"><Relationship Id="root" Type="` + mainDocumentRelationship + `" Target="` + main + `"/></Relationships>`
}
func officeTestWorksheet(body string) string {
	return `<worksheet xmlns="` + officeTestSpreadsheetNS + `"><sheetData>` + body + `</sheetData></worksheet>`
}
func officeTestSlide(text string) string {
	return `<p:sld xmlns:p="` + officeTestPresentationNS + `" xmlns:a="` + officeTestDrawingNS + `"><p:cSld><p:spTree><p:sp><p:nvSpPr><p:cNvPr id="2" name="Text"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>` + text + `</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`
}

func officeTestXLSX() map[string]string {
	return map[string]string{
		"[Content_Types].xml":        officeTestTypes(map[string]string{"xl/workbook.xml": officeTestWorkbookType, "xl/worksheets/sheet1.xml": officeTestSheetType, "xl/worksheets/sheet2.xml": officeTestSheetType, "xl/worksheets/sheet10.xml": officeTestSheetType, "xl/sharedStrings.xml": "application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"}),
		"_rels/.rels":                officeTestRootRelationships("xl/workbook.xml"),
		"xl/workbook.xml":            `<workbook xmlns="` + officeTestSpreadsheetNS + `" xmlns:r="` + officeTestRelNS + `"><sheets><sheet name="Ten" sheetId="10" r:id="ten"/><sheet name="Two" sheetId="2" r:id="two"/><sheet name="Báo cáo 猫" sheetId="1" r:id="one"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="` + relationshipsNamespace + `"><Relationship Id="ten" Type="` + officeTestRelNS + `/worksheet" Target="worksheets/sheet10.xml"/><Relationship Id="two" Type="` + officeTestRelNS + `/worksheet" Target="worksheets/sheet2.xml"/><Relationship Id="one" Type="` + officeTestRelNS + `/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="strings" Type="` + officeTestRelNS + `/sharedStrings" Target="sharedStrings.xml"/></Relationships>`,
		"xl/sharedStrings.xml":       `<sst xmlns="` + officeTestSpreadsheetNS + `" count="1" uniqueCount="1"><si><r><t>Xin </t></r><r><t>chào &amp; 猫</t></r></si></sst>`,
		"xl/worksheets/sheet1.xml":   officeTestWorksheet(`<row r="2"><c r="B2" t="b"><v>1</v></c></row><row r="1"><c r="B1" t="inlineStr"><is><t>literal &lt;script&gt; and ` + "`ticks`" + `</t></is></c><c r="A1" t="s"><v>0</v></c><c r="C1"><f>SUM(A2:A3)</f><v>12</v></c></row>`),
		"xl/worksheets/sheet2.xml":   officeTestWorksheet(`<row r="1"><c r="A1"><v>2</v></c></row>`),
		"xl/worksheets/sheet10.xml":  officeTestWorksheet(`<row r="1"><c r="A1" t="str"><v>ten</v></c></row>`),
	}
}

func officeTestPPTX() map[string]string {
	return map[string]string{
		"[Content_Types].xml":             officeTestTypes(map[string]string{"ppt/presentation.xml": officeTestPresentationType, "ppt/slides/slide1.xml": officeTestSlideType, "ppt/slides/slide2.xml": officeTestSlideType, "ppt/slides/slide10.xml": officeTestSlideType}),
		"_rels/.rels":                     officeTestRootRelationships("ppt/presentation.xml"),
		"ppt/presentation.xml":            `<p:presentation xmlns:p="` + officeTestPresentationNS + `" xmlns:r="` + officeTestRelNS + `"><p:sldIdLst><p:sldId id="300" r:id="ten"/><p:sldId id="257" r:id="two"/><p:sldId id="256" r:id="one"/></p:sldIdLst></p:presentation>`,
		"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="` + relationshipsNamespace + `"><Relationship Id="ten" Type="` + officeTestRelNS + `/slide" Target="slides/slide10.xml"/><Relationship Id="two" Type="` + officeTestRelNS + `/slide" Target="slides/slide2.xml"/><Relationship Id="one" Type="` + officeTestRelNS + `/slide" Target="slides/slide1.xml"/></Relationships>`,
		"ppt/slides/slide1.xml":           officeTestSlide("Xin chào &amp; 猫"),
		"ppt/slides/slide2.xml":           officeTestSlide("second"),
		"ppt/slides/slide10.xml":          officeTestSlide("tenth"),
	}
}

func officeTestReject(t *testing.T, parts map[string]string, extract func([]byte) (Result, error), kind ErrorKind) {
	t.Helper()
	result, err := extract(officeTestArchive(t, parts))
	var rejected *Error
	if !errors.As(err, &rejected) || rejected.Kind != kind || result != (Result{}) {
		t.Fatalf("want typed %q with no partial output, got bytes=%d error=%v", kind, result.TextBytes, err)
	}
}

func officeTestDigest(t *testing.T, result Result) {
	t.Helper()
	sum := sha256.Sum256([]byte(result.Text))
	if result.TextBytes != len(result.Text) || result.TextSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("full UTF-8 text digest or byte count differs")
	}
}

func TestOfficeXLSXNumericOrderUnicodeSharedStringsAndCachedFormula(t *testing.T) {
	result, err := ExtractXLSX(officeTestArchive(t, officeTestXLSX()))
	if err != nil {
		t.Fatal(err)
	}
	want := "Sheet 1: Báo cáo 猫 (id 1; xl/worksheets/sheet1.xml)\nA1: Xin chào & 猫\nB1: literal <script> and `ticks`\nC1 [cached formula]: 12\nB2: true\n\nSheet 2: Two (id 2; xl/worksheets/sheet2.xml)\nA1: 2\n\nSheet 10: Ten (id 10; xl/worksheets/sheet10.xml)\nA1: ten"
	if result.Text != want {
		t.Fatalf("unexpected numeric-order extraction: %q", result.Text)
	}
	officeTestDigest(t, result)
	if strings.Contains(result.Text, "SUM(") {
		t.Fatal("formula source was evaluated or exported")
	}
}

func TestOfficeXLSXSummaryUsesCellRowsAndRejectsPartialProof(t *testing.T) {
	parts := officeTestXLSX()
	summary, err := InspectXLSX(officeTestArchive(t, parts))
	if err != nil || summary != (XLSXSummary{Sheets: 3, MaxPopulatedRows: 2, PopulatedCells: 6, FormulaCells: 1}) {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	parts["xl/worksheets/sheet2.xml"] = officeTestWorksheet(`<row r="1"><c r="A1" t="inlineStr"><is><t>first&#10;second&#10;third</t></is></c></row>`)
	summary, err = InspectXLSX(officeTestArchive(t, parts))
	if err != nil || summary.MaxPopulatedRows != 2 {
		t.Fatal("text line breaks inflated row count", summary, err)
	}
	parts["xl/worksheets/sheet10.xml"] = `<worksheet>`
	summary, err = InspectXLSX(officeTestArchive(t, parts))
	if err == nil || summary != (XLSXSummary{}) {
		t.Fatal("invalid later sheet returned partial proof", summary, err)
	}
	parts = officeTestXLSX()
	parts["xl/worksheets/sheet1.xml"] = officeTestWorksheet(`<row r="1"><c r="A1"><f>1+1</f></c></row><row r="2"/><row r="3"/>`)
	summary, err = InspectXLSX(officeTestArchive(t, parts))
	if err != nil || summary.MaxPopulatedRows != 1 || summary.UncachedFormulaCells != 1 {
		t.Fatal("blank rows or absent cached formula misreported", summary, err)
	}
}

func TestOfficePPTXNumericSlideOrderAndProvenance(t *testing.T) {
	result, err := ExtractPPTX(officeTestArchive(t, officeTestPPTX()))
	if err != nil {
		t.Fatal(err)
	}
	want := "Slide 1 (ppt/slides/slide1.xml)\nXin chào & 猫\n\nSlide 2 (ppt/slides/slide2.xml)\nsecond\n\nSlide 10 (ppt/slides/slide10.xml)\ntenth"
	if result.Text != want {
		t.Fatalf("unexpected slide extraction: %q", result.Text)
	}
	officeTestDigest(t, result)
}

func TestOfficeRejectsNonPackagesWrongTypeAndNamespace(t *testing.T) {
	for _, extract := range []func([]byte) (Result, error){ExtractXLSX, ExtractPPTX} {
		officeTestReject(t, map[string]string{"notes.xml": "<notes>not Office</notes>"}, extract, InvalidOffice)
	}
	x := officeTestXLSX()
	x["[Content_Types].xml"] = strings.ReplaceAll(x["[Content_Types].xml"], officeTestWorkbookType, "application/xml")
	officeTestReject(t, x, ExtractXLSX, InvalidOffice)
	p := officeTestPPTX()
	p["ppt/presentation.xml"] = strings.ReplaceAll(p["ppt/presentation.xml"], officeTestPresentationNS, "urn:fake")
	officeTestReject(t, p, ExtractPPTX, InvalidOffice)
	x = officeTestXLSX()
	x["xl/worksheets/sheet1.xml"] = strings.ReplaceAll(x["xl/worksheets/sheet1.xml"], officeTestSpreadsheetNS, "urn:fake")
	officeTestReject(t, x, ExtractXLSX, InvalidOffice)
	p = officeTestPPTX()
	p["[Content_Types].xml"] = strings.ReplaceAll(p["[Content_Types].xml"], officeTestSlideType, "application/xml")
	officeTestReject(t, p, ExtractPPTX, InvalidOffice)
}

func TestOfficeRejectsExternalReferencesMacrosAndEmbeddedCode(t *testing.T) {
	for _, name := range []string{"xl/vbaProject.bin", "xl/externalLinks/externalLink1.xml", "xl/embeddings/object.bin", "xl/script.js"} {
		x := officeTestXLSX()
		x[name] = "unsafe"
		officeTestReject(t, x, ExtractXLSX, UnsafeOffice)
	}
	x := officeTestXLSX()
	x["[Content_Types].xml"] = strings.ReplaceAll(x["[Content_Types].xml"], officeTestWorkbookType, "application/vnd.ms-excel.sheet.macroEnabled.main+xml")
	officeTestReject(t, x, ExtractXLSX, UnsafeOffice)
	p := officeTestPPTX()
	p["ppt/_rels/presentation.xml.rels"] = strings.ReplaceAll(p["ppt/_rels/presentation.xml.rels"], `</Relationships>`, `<Relationship Id="external" Type="`+officeTestRelNS+`/hyperlink" Target="https://example.invalid" TargetMode="External"/></Relationships>`)
	officeTestReject(t, p, ExtractPPTX, UnsafeOffice)
	x = officeTestXLSX()
	x["xl/worksheets/sheet1.xml"] = officeTestWorksheet(`<row r="1"><c r="A1"><f>WEBSERVICE("https://example.invalid")</f><v>3</v></c></row>`)
	officeTestReject(t, x, ExtractXLSX, UnsafeOffice)
}

func TestOfficeRejectsDTDTraversalDepthAndTextBudgets(t *testing.T) {
	x := officeTestXLSX()
	x["xl/unused.xml"] = `<!DOCTYPE doc [<!ENTITY e SYSTEM "file:///outside">]><doc/>`
	officeTestReject(t, x, ExtractXLSX, UnsafeOffice)
	p := officeTestPPTX()
	p["../outside.xml"] = "<doc/>"
	officeTestReject(t, p, ExtractPPTX, UnsafeOffice)
	x = officeTestXLSX()
	x["xl/unused.xml"] = "<doc>" + strings.Repeat("<nested>", 64) + strings.Repeat("</nested>", 64) + "</doc>"
	officeTestReject(t, x, ExtractXLSX, LimitExceeded)
	p = officeTestPPTX()
	p["ppt/slides/slide1.xml"] = officeTestSlide(strings.Repeat("猫", (512<<10)/3+1))
	officeTestReject(t, p, ExtractPPTX, LimitExceeded)
	x = officeTestXLSX()
	x["xl/worksheets/sheet1.xml"] = officeTestWorksheet(`<row r="1"><c r="A1" t="inlineStr"><is><t>` + strings.Repeat("x", 512<<10) + `</t></is></c></row>`)
	officeTestReject(t, x, ExtractXLSX, LimitExceeded)
}

func TestOfficeRejectsInvalidCellReferencesAndSharedStringIndex(t *testing.T) {
	for _, cell := range []string{`<c r="XFE1"><v>1</v></c>`, `<c r="A1048577"><v>1</v></c>`, `<c r="A1" t="s"><v>42</v></c>`, `<c r="A1"><v>NaN</v></c>`, `<c r="A1"><v>1</v></c><c r="A1"><v>2</v></c>`} {
		x := officeTestXLSX()
		x["xl/worksheets/sheet1.xml"] = officeTestWorksheet(`<row r="1">` + cell + `</row>`)
		officeTestReject(t, x, ExtractXLSX, InvalidOffice)
	}
}

func TestOfficeRejectsCorruptZIPCRCAndClaimedBomb(t *testing.T) {
	raw := officeTestArchive(t, officeTestPPTX())
	raw = mutateCentral(t, raw, "ppt/slides/slide1.xml", 16, 0)
	result, err := ExtractPPTX(raw)
	var rejected *Error
	if !errors.As(err, &rejected) || rejected.Kind != CorruptOffice || result != (Result{}) {
		t.Fatal("CRC corruption must fail closed")
	}
	raw = officeTestArchive(t, officeTestXLSX())
	raw = mutateCentral(t, raw, "xl/worksheets/sheet1.xml", 24, 8<<20+1)
	result, err = ExtractXLSX(raw)
	if !errors.As(err, &rejected) || rejected.Kind != LimitExceeded || result != (Result{}) {
		t.Fatal("claimed zip bomb must fail closed")
	}
}

func TestOfficeXLSXInlinePhoneticsAndSharedFormulaPresence(t *testing.T) {
	x := officeTestXLSX()
	x["xl/worksheets/sheet1.xml"] = officeTestWorksheet("<row r=\"1\"><c r=\"A1\" t=\"inlineStr\"><is><t>base</t><rPh sb=\"0\" eb=\"4\"><t>pronunciation</t></rPh></is></c><c r=\"B1\"><f t=\"shared\" si=\"0\"/><v>42</v></c><c r=\"C1\"><f t=\"shared\" si=\"0\"/></c></row>")
	result, err := ExtractXLSX(officeTestArchive(t, x))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Text, "\nA1: base\nB1 [cached formula]: 42\nC1 [cached formula]: [no cached value]") || strings.Contains(result.Text, "pronunciation") {
		t.Fatalf("inline phonetics or shared formula presence handled incorrectly: %q", result.Text)
	}
}

func TestOfficeRejectsNumericPartAliasesAndTextOutsideSlideTree(t *testing.T) {
	t.Run("sheet_alias", func(t *testing.T) {
		x := officeTestXLSX()
		x["xl/worksheets/sheet01.xml"] = x["xl/worksheets/sheet1.xml"]
		delete(x, "xl/worksheets/sheet1.xml")
		for name, value := range x {
			x[name] = strings.ReplaceAll(value, "sheet1.xml", "sheet01.xml")
		}
		officeTestReject(t, x, ExtractXLSX, InvalidOffice)
	})
	t.Run("outside_shape_tree", func(t *testing.T) {
		p := officeTestPPTX()
		p["ppt/slides/slide1.xml"] = strings.Replace(p["ppt/slides/slide1.xml"], "<p:cSld>", "<a:p><a:r><a:t>not shape text</a:t></a:r></a:p><p:cSld>", 1)
		officeTestReject(t, p, ExtractPPTX, InvalidOffice)
	})
}

func TestOfficeRejectsForeignSharedStringText(t *testing.T) {
	x := officeTestXLSX()
	x["xl/sharedStrings.xml"] = "<sst xmlns=\"" + officeTestSpreadsheetNS + "\"><si><fake:t xmlns:fake=\"urn:fake\">shadow text</fake:t></si></sst>"
	officeTestReject(t, x, ExtractXLSX, InvalidOffice)
}

func TestOfficePPTXBreaksParagraphsAndStrictNamespaces(t *testing.T) {
	p := officeTestPPTX()
	p["ppt/slides/slide1.xml"] = strings.Replace(p["ppt/slides/slide1.xml"], "<a:p><a:r><a:t>Xin chào &amp; 猫</a:t></a:r></a:p>", "<a:p><a:r><a:t>first</a:t></a:r><a:tab/><a:r><a:t>tab</a:t></a:r><a:br/><a:r><a:t>break</a:t></a:r></a:p><a:p/><a:p><a:r><a:t>next</a:t></a:r></a:p>", 1)
	result, err := ExtractPPTX(officeTestArchive(t, p))
	if err != nil || !strings.Contains(result.Text, "\nfirst\ttab\nbreak\n\nnext\n\nSlide 2") {
		t.Fatalf("break/paragraph extraction differs: %q %v", result.Text, err)
	}
	for name, value := range p {
		value = strings.ReplaceAll(value, officeTestPresentationNS, "http://purl.oclc.org/ooxml/presentationml/main")
		value = strings.ReplaceAll(value, officeTestDrawingNS, "http://purl.oclc.org/ooxml/drawingml/main")
		value = strings.ReplaceAll(value, officeTestRelNS, "http://purl.oclc.org/ooxml/officeDocument/relationships")
		p[name] = value
	}
	strict, err := ExtractPPTX(officeTestArchive(t, p))
	if err != nil || strict != result {
		t.Fatalf("strict namespace changed accepted text: %v", err)
	}
	x := officeTestXLSX()
	for name, value := range x {
		value = strings.ReplaceAll(value, officeTestSpreadsheetNS, "http://purl.oclc.org/ooxml/spreadsheetml/main")
		value = strings.ReplaceAll(value, officeTestRelNS, "http://purl.oclc.org/ooxml/officeDocument/relationships")
		x[name] = value
	}
	if _, err := ExtractXLSX(officeTestArchive(t, x)); err != nil {
		t.Fatalf("strict spreadsheet rejected: %v", err)
	}

	p = officeTestPPTX()
	table := "<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id=\"3\" name=\"Table\"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr><p:xfrm/><a:graphic><a:graphicData uri=\"http://schemas.openxmlformats.org/drawingml/2006/table\"><a:tbl><a:tblPr/><a:tblGrid><a:gridCol w=\"100\"/><a:gridCol w=\"100\"/></a:tblGrid><a:tr h=\"100\"><a:tc><a:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>cell one 猫</a:t></a:r></a:p></a:txBody><a:tcPr/></a:tc><a:tc><a:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t>cell two</a:t></a:r></a:p></a:txBody><a:tcPr/></a:tc></a:tr></a:tbl></a:graphicData></a:graphic></p:graphicFrame>"
	p["ppt/slides/slide1.xml"] = strings.Replace(p["ppt/slides/slide1.xml"], "</p:spTree>", table+"</p:spTree>", 1)
	tableResult, err := ExtractPPTX(officeTestArchive(t, p))
	if err != nil || !strings.Contains(tableResult.Text, "Xin chào & 猫\ncell one 猫\ncell two") {
		t.Fatalf("DrawingML table cell text differs: %v", err)
	}

}

func TestOfficeRejectsUnrecognizedPayloadsAndActiveTypes(t *testing.T) {
	for _, name := range []string{"xl/tool.py", "ppt/tool.wsf", "xl/unknown.dat"} {
		x := officeTestXLSX()
		x[name] = "payload must not be treated as inert media"
		officeTestReject(t, x, ExtractXLSX, UnsafeOffice)
	}
	x := officeTestXLSX()
	x["xl/unused.xml"] = "<data/>"
	x["[Content_Types].xml"] = strings.Replace(x["[Content_Types].xml"], "</Types>", "<Override PartName=\"/xl/unused.xml\" ContentType=\"application/x-vbscript\"/></Types>", 1)
	officeTestReject(t, x, ExtractXLSX, UnsafeOffice)
}

func TestOfficeMissingRelationshipsAndEmbeddedXML(t *testing.T) {
	for _, missing := range []string{"[Content_Types].xml", "_rels/.rels", "xl/_rels/workbook.xml.rels"} {
		x := officeTestXLSX()
		delete(x, missing)
		officeTestReject(t, x, ExtractXLSX, InvalidOffice)
	}
	for _, tag := range []string{"script", "oleObj", "externalData", "ddeLink", "controls"} {
		p := officeTestPPTX()
		p["ppt/unused.xml"] = "<doc><" + tag + "/></doc>"
		officeTestReject(t, p, ExtractPPTX, UnsafeOffice)
	}
	x := officeTestXLSX()
	x["xl/worksheets/sheet1.xml"] = officeTestWorksheet("<row r=\"1\"><c r=\"A1\"><f>HYPERLINK (A2)</f><v>3</v></c></row>")
	officeTestReject(t, x, ExtractXLSX, UnsafeOffice)
}

func TestOfficeArchiveCountAndActualXMLBudget(t *testing.T) {
	x := officeTestXLSX()
	for i := 0; i < MaxArchiveEntries; i++ {
		x["xl/media/"+strconv.Itoa(i)+".png"] = "inert"
	}
	officeTestReject(t, x, ExtractXLSX, LimitExceeded)
	x = officeTestXLSX()
	x["xl/unused.xml"] = "<doc>" + strings.Repeat("x", MaxXMLBytes) + "</doc>"
	officeTestReject(t, x, ExtractXLSX, LimitExceeded)
	raw := make([]byte, MaxOfficePackageBytes+1)
	for _, extract := range []func([]byte) (Result, error){ExtractXLSX, ExtractPPTX} {
		result, err := extract(raw)
		var rejected *Error
		if !errors.As(err, &rejected) || rejected.Kind != LimitExceeded || result != (Result{}) {
			t.Fatal("oversized raw input must reject without output")
		}
	}
}

func TestOfficeArchiveDuplicateSymlinkAndEncryption(t *testing.T) {
	for _, kind := range []string{"duplicate", "symlink", "encrypted"} {
		t.Run(kind, func(t *testing.T) {
			x := officeTestXLSX()
			var out bytes.Buffer
			writer := zip.NewWriter(&out)
			for name, data := range x {
				header := &zip.FileHeader{Name: name, Method: zip.Deflate}
				if kind == "symlink" && name == "xl/worksheets/sheet1.xml" {
					header.SetMode(os.ModeSymlink | 0600)
				}
				stream, err := writer.CreateHeader(header)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = stream.Write([]byte(data)); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "duplicate" {
				stream, err := writer.Create("XL/WORKBOOK.XML")
				if err != nil {
					t.Fatal(err)
				}
				if _, err = stream.Write([]byte(x["xl/workbook.xml"])); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			raw := out.Bytes()
			if kind == "encrypted" {
				raw = append([]byte(nil), raw...)
				offset := bytes.Index(raw, []byte("PK\x01\x02"))
				if offset < 0 {
					t.Fatal("missing central header")
				}
				binary.LittleEndian.PutUint16(raw[offset+8:offset+10], binary.LittleEndian.Uint16(raw[offset+8:offset+10])|1)
			}
			result, err := ExtractXLSX(raw)
			var rejected *Error
			if !errors.As(err, &rejected) || rejected.Kind != UnsafeOffice || result != (Result{}) {
				t.Fatalf("%s archive was accepted: %v", kind, err)
			}
		})
	}
}

func TestOfficeCellAndSharedStringCountBudgets(t *testing.T) {
	x := officeTestXLSX()
	var cells strings.Builder
	cells.WriteString("<row>")
	for i := 1; i <= MaxOfficeValues+1; i++ {
		cells.WriteString("<c r=\"A" + strconv.Itoa(i) + "\"><v>1</v></c>")
	}
	cells.WriteString("</row>")
	x["xl/worksheets/sheet1.xml"] = officeTestWorksheet(cells.String())
	officeTestReject(t, x, ExtractXLSX, LimitExceeded)
	x = officeTestXLSX()
	x["xl/sharedStrings.xml"] = "<sst xmlns=\"" + officeTestSpreadsheetNS + "\">" + strings.Repeat("<si><t/></si>", MaxOfficeValues+1) + "</sst>"
	officeTestReject(t, x, ExtractXLSX, LimitExceeded)
	x = officeTestXLSX()
	x["xl/sharedStrings.xml"] = "<sst xmlns=\"" + officeTestSpreadsheetNS + "\"><si><t>" + strings.Repeat("x", MaxTextBytes/2+1) + "</t></si><si><t>" + strings.Repeat("y", MaxTextBytes/2) + "</t></si></sst>"
	officeTestReject(t, x, ExtractXLSX, LimitExceeded)
}

func TestOfficePartBudgetAndInertMedia(t *testing.T) {
	p := officeTestPPTX()
	var ids, relations strings.Builder
	types := map[string]string{"ppt/presentation.xml": officeTestPresentationType}
	for i := 1; i <= MaxOfficeParts+1; i++ {
		part := "ppt/slides/slide" + strconv.Itoa(i) + ".xml"
		id := "s" + strconv.Itoa(i)
		types[part] = officeTestSlideType
		p[part] = officeTestSlide("text")
		ids.WriteString("<p:sldId id=\"" + strconv.Itoa(i+255) + "\" r:id=\"" + id + "\"/>")
		relations.WriteString("<Relationship Id=\"" + id + "\" Type=\"" + officeTestRelNS + "/slide\" Target=\"slides/slide" + strconv.Itoa(i) + ".xml\"/>")
	}
	p["[Content_Types].xml"] = officeTestTypes(types)
	p["ppt/presentation.xml"] = "<p:presentation xmlns:p=\"" + officeTestPresentationNS + "\" xmlns:r=\"" + officeTestRelNS + "\"><p:sldIdLst>" + ids.String() + "</p:sldIdLst></p:presentation>"
	p["ppt/_rels/presentation.xml.rels"] = "<Relationships xmlns=\"" + relationshipsNamespace + "\">" + relations.String() + "</Relationships>"
	officeTestReject(t, p, ExtractPPTX, LimitExceeded)
	x := officeTestXLSX()
	x["xl/media/image1.png"] = "\x89PNG\r\n\x1a\ninert bytes are never decoded"
	got, err := ExtractXLSX(officeTestArchive(t, x))
	if err != nil {
		t.Fatal(err)
	}
	base, err := ExtractXLSX(officeTestArchive(t, officeTestXLSX()))
	if err != nil || got != base {
		t.Fatal("inert media changed text")
	}
}

func TestOfficeFormulaWithoutCachedValue(t *testing.T) {
	for _, kind := range []string{"b", "d", "s", "str", "e", "n"} {
		t.Run(kind, func(t *testing.T) {
			x := officeTestXLSX()
			x["xl/worksheets/sheet1.xml"] = officeTestWorksheet("<row r=\"1\"><c r=\"A1\" t=\"" + kind + "\"><f>1=1</f></c></row>")
			result, err := ExtractXLSX(officeTestArchive(t, x))
			if err != nil || !strings.Contains(result.Text, "A1 [cached formula]: [no cached value]") {
				t.Fatalf("absent cache differs for type %s: %v", kind, err)
			}
		})
	}
}

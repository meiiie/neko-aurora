package documents

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	InvalidOffice         ErrorKind = "invalid_office"
	UnsafeOffice          ErrorKind = "unsafe_office"
	CorruptOffice         ErrorKind = "corrupt_office"
	MaxOfficePackageBytes           = MaxDOCXBytes
	MaxOfficeValues                 = 65536
	MaxOfficeParts                  = 128

	officeSpreadsheetNS        = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	officeStrictSpreadsheetNS  = "http://purl.oclc.org/ooxml/spreadsheetml/main"
	officePresentationNS       = "http://schemas.openxmlformats.org/presentationml/2006/main"
	officeStrictPresentationNS = "http://purl.oclc.org/ooxml/presentationml/main"
	officeDrawingNS            = "http://schemas.openxmlformats.org/drawingml/2006/main"
	officeStrictDrawingNS      = "http://purl.oclc.org/ooxml/drawingml/main"
	officeRelNS                = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	officeStrictRelNS          = "http://purl.oclc.org/ooxml/officeDocument/relationships"
	officeWorkbookType         = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"
	officeWorksheetType        = "application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"
	officeStringsType          = "application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"
	officePresentationType     = "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"
	officeSlideType            = "application/vnd.openxmlformats-officedocument.presentationml.slide+xml"
)

type officeRelation struct{ kind, target string }
type officePackage struct {
	parts map[string][]byte
	types map[string]string
	rels  map[string]map[string]officeRelation
}
type officeItem struct {
	number, id          int
	name, target, state string
}
type officeText struct{ builder strings.Builder }

func (text *officeText) add(value string) error {
	if len(value) > MaxTextBytes-text.builder.Len() {
		return reject(LimitExceeded, "office_text_bytes")
	}
	text.builder.WriteString(value)
	return nil
}
func officeResult(text string) Result {
	sum := sha256.Sum256([]byte(text))
	return Result{Text: text, TextSHA256: hex.EncodeToString(sum[:]), TextBytes: len(text)}
}
func officeMappedError(err error) error {
	if parsed, ok := err.(*Error); ok && parsed.Kind == UnsafeDOCX {
		return reject(UnsafeOffice, parsed.Reason)
	}
	return err
}
func officeDangerousPart(name string) bool {
	if dangerousPart(name) {
		return true
	}
	lower := strings.ToLower(name)
	if strings.Contains(lower, "externallinks") {
		return true
	}
	switch path.Ext(lower) {
	case ".xlsm", ".xltm", ".xlam", ".pptm", ".potm", ".ppam", ".svg", ".pdf", ".rtf":
		return true
	}
	return false
}
func officeInertPart(name string) bool {
	if isXML(name) {
		return true
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tif", ".tiff", ".emf", ".wmf", ".wav", ".mp3", ".mp4", ".m4a", ".wma", ".wmv", ".mpeg", ".mpg", ".mov", ".avi":
		return true
	}
	return false
}
func officeDangerousType(value string) bool {
	lower := strings.ToLower(value)
	if unsafeContentType(value) {
		return true
	}
	for _, marker := range []string{"macro", "externallink", "vbscript", "ecmascript", "powershell", "python", "xslt"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func officeArchive(raw []byte) (*officePackage, error) {
	if len(raw) > MaxOfficePackageBytes {
		return nil, reject(LimitExceeded, "raw_bytes")
	}
	if len(raw) < 4 || !bytes.Equal(raw[:4], []byte("PK\x03\x04")) {
		return nil, reject(InvalidOffice, "zip_signature")
	}
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, reject(CorruptOffice, "zip_directory")
	}
	if len(archive.File) > MaxArchiveEntries {
		return nil, reject(LimitExceeded, "archive_entries")
	}
	seen := map[string]bool{}
	var claimed uint64
	for _, file := range archive.File {
		name := strings.TrimSuffix(file.Name, "/")
		mode := file.Mode().Type()
		if !safePartName(name) || officeDangerousPart(name) || !file.FileInfo().IsDir() && !officeInertPart(name) || mode != 0 && mode != os.ModeDir {
			return nil, reject(UnsafeOffice, "archive_entry")
		}
		key := strings.ToLower(name)
		if seen[key] {
			return nil, reject(UnsafeOffice, "duplicate_entry")
		}
		seen[key] = true
		if file.Flags&0x41 != 0 || file.Method != zip.Store && file.Method != zip.Deflate {
			return nil, reject(UnsafeOffice, "archive_encoding")
		}
		if file.UncompressedSize64 > MaxUncompressedBytes || claimed > MaxUncompressedBytes-file.UncompressedSize64 {
			return nil, reject(LimitExceeded, "claimed_uncompressed_bytes")
		}
		claimed += file.UncompressedSize64
		if isXML(name) && file.UncompressedSize64 > MaxXMLBytes {
			return nil, reject(LimitExceeded, "xml_bytes")
		}
		if file.FileInfo().IsDir() && file.UncompressedSize64 != 0 {
			return nil, reject(UnsafeOffice, "directory_payload")
		}
	}
	pkg := &officePackage{parts: map[string][]byte{}, types: map[string]string{}, rels: map[string]map[string]officeRelation{}}
	actual := 0
	for _, file := range archive.File {
		limit := MaxUncompressedBytes - actual
		if isXML(file.Name) && limit > MaxXMLBytes {
			limit = MaxXMLBytes
		}
		stream, err := file.Open()
		if err != nil {
			return nil, reject(CorruptOffice, "zip_entry_open")
		}
		data, readErr := io.ReadAll(io.LimitReader(stream, int64(limit)+1))
		closeErr := stream.Close()
		if len(data) > limit {
			return nil, reject(LimitExceeded, "actual_uncompressed_bytes")
		}
		if readErr != nil || closeErr != nil || uint64(len(data)) != file.UncompressedSize64 {
			return nil, reject(CorruptOffice, "zip_entry_integrity")
		}
		actual += len(data)
		if file.FileInfo().IsDir() {
			continue
		}
		if isXML(file.Name) {
			if _, err := validateXML(data); err != nil {
				return nil, officeMappedError(err)
			}
			if err := officeXMLSafety(data); err != nil {
				return nil, err
			}
		}
		pkg.parts[file.Name] = data
	}
	if err := pkg.readTypes(); err != nil {
		return nil, err
	}
	for name, data := range pkg.parts {
		if strings.HasSuffix(strings.ToLower(name), ".rels") {
			relations, err := pkg.readRelations(name, data)
			if err != nil {
				return nil, err
			}
			pkg.rels[name] = relations
		}
	}
	return pkg, nil
}

func officeXMLSafety(data []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return reject(InvalidXML, "office_xml_syntax")
		}
		element, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch strings.ToLower(element.Name.Local) {
		case "script", "oleobj", "oleobject", "externallink", "externalreferences", "externalreference", "externalbook", "externaldata", "ddelink", "olelink", "control", "controls", "activex":
			return reject(UnsafeOffice, "active_xml_content")
		}
		for _, attribute := range element.Attr {
			if (attribute.Name.Space == officeRelNS || attribute.Name.Space == officeStrictRelNS) && attribute.Name.Local == "link" && attribute.Value != "" {
				return reject(UnsafeOffice, "external_xml_reference")
			}
		}
	}
}

func (pkg *officePackage) readTypes() error {
	data, ok := pkg.parts["[Content_Types].xml"]
	if !ok {
		return reject(InvalidOffice, "content_types_missing")
	}
	root, _ := validateXML(data)
	if root != (xml.Name{Space: contentTypesNamespace, Local: "Types"}) {
		return reject(InvalidOffice, "content_types_namespace")
	}
	defaults := map[string]string{}
	depth := 0
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return reject(InvalidXML, "content_types_syntax")
		}
		switch element := token.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				continue
			}
			value := attrValue(element, "ContentType")
			if officeDangerousType(value) {
				return reject(UnsafeOffice, "active_content_type")
			}
			if depth != 2 || element.Name.Space != contentTypesNamespace || value == "" {
				return reject(InvalidOffice, "content_type_entry")
			}
			switch element.Name.Local {
			case "Override":
				name := attrValue(element, "PartName")
				if !strings.HasPrefix(name, "/") || !safePartName(strings.TrimPrefix(name, "/")) {
					return reject(UnsafeOffice, "content_type_part")
				}
				name = strings.TrimPrefix(name, "/")
				if _, ok := pkg.parts[name]; !ok {
					return reject(InvalidOffice, "content_type_missing_part")
				}
				if _, ok := pkg.types[name]; ok {
					return reject(InvalidOffice, "duplicate_content_type")
				}
				pkg.types[name] = value
			case "Default":
				extension := strings.ToLower(attrValue(element, "Extension"))
				if extension == "" || strings.ContainsAny(extension, "./\\:%?#") {
					return reject(InvalidOffice, "content_type_extension")
				}
				if _, ok := defaults[extension]; ok {
					return reject(InvalidOffice, "duplicate_content_type")
				}
				defaults[extension] = value
			default:
				return reject(InvalidOffice, "content_type_element")
			}
		case xml.EndElement:
			depth--
		}
	}
	for name := range pkg.parts {
		if pkg.types[name] == "" {
			pkg.types[name] = defaults[strings.TrimPrefix(strings.ToLower(path.Ext(name)), ".")]
		}
	}
	return nil
}

func (pkg *officePackage) readRelations(name string, data []byte) (map[string]officeRelation, error) {
	root, _ := validateXML(data)
	if root != (xml.Name{Space: relationshipsNamespace, Local: "Relationships"}) {
		return nil, reject(InvalidOffice, "relationships_namespace")
	}
	base := ""
	if name != "_rels/.rels" {
		directory := path.Dir(name)
		if path.Base(directory) != "_rels" {
			return nil, reject(UnsafeOffice, "relationship_part_path")
		}
		base = path.Dir(directory)
		source := path.Join(base, strings.TrimSuffix(path.Base(name), ".rels"))
		if _, ok := pkg.parts[source]; !ok {
			return nil, reject(UnsafeOffice, "relationship_missing_source")
		}
	}
	relations := map[string]officeRelation{}
	depth := 0
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, reject(InvalidXML, "relationships_syntax")
		}
		switch element := token.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				continue
			}
			if depth != 2 || element.Name.Space != relationshipsNamespace || element.Name.Local != "Relationship" {
				return nil, reject(InvalidOffice, "relationship_element")
			}
			id, kind, target := attrValue(element, "Id"), attrValue(element, "Type"), attrValue(element, "Target")
			if id == "" || len(id) > 128 || kind == "" || target == "" {
				return nil, reject(InvalidOffice, "relationship_attributes")
			}
			if _, ok := relations[id]; ok {
				return nil, reject(InvalidOffice, "duplicate_relationship")
			}
			mode := attrValue(element, "TargetMode")
			if mode != "" && mode != "Internal" || strings.HasPrefix(target, "/") || strings.ContainsAny(target, "\\:%?#") || officeDangerousType(kind) {
				return nil, reject(UnsafeOffice, "external_relationship")
			}
			resolved := path.Join(base, target)
			if !safePartName(resolved) || officeDangerousPart(resolved) {
				return nil, reject(UnsafeOffice, "relationship_target")
			}
			if _, ok := pkg.parts[resolved]; !ok {
				return nil, reject(InvalidOffice, "relationship_missing_target")
			}
			relations[id] = officeRelation{kind: kind, target: resolved}
		case xml.EndElement:
			depth--
		}
	}
	return relations, nil
}

func officeRelationKind(kind, suffix string) bool {
	return kind == officeRelNS+"/"+suffix || kind == officeStrictRelNS+"/"+suffix
}
func officeRelationID(element xml.StartElement) string {
	for _, attribute := range element.Attr {
		if attribute.Name.Local == "id" && (attribute.Name.Space == officeRelNS || attribute.Name.Space == officeStrictRelNS) {
			return attribute.Value
		}
	}
	return ""
}
func (pkg *officePackage) main(part, contentType string) error {
	if pkg.types[part] != contentType {
		return reject(InvalidOffice, "main_content_type")
	}
	count := 0
	for _, relation := range pkg.rels["_rels/.rels"] {
		if officeRelationKind(relation.kind, "officeDocument") {
			count++
			if relation.target != part {
				return reject(InvalidOffice, "main_document_part")
			}
		}
	}
	if count != 1 {
		return reject(InvalidOffice, "main_document_relationship")
	}
	return nil
}
func officeNumber(value string, maximum int) (int, bool) {
	if value == "" || len(value) > 10 {
		return 0, false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	number, err := strconv.Atoi(value)
	return number, err == nil && number > 0 && number <= maximum
}
func officePartNumber(name, prefix, suffix string) (int, bool) {
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return 0, false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
	number, ok := officeNumber(value, 1048576)
	return number, ok && strconv.Itoa(number) == value
}

// XLSXSummary describes parsed cells; formulas are never evaluated.
type XLSXSummary struct {
	Sheets, MaxPopulatedRows, PopulatedCells int
	FormulaCells, UncachedFormulaCells       int
}

// InspectXLSX uses the same complete package validation and limits as text extraction.
// A rejected package returns no partial summary.
func InspectXLSX(raw []byte) (XLSXSummary, error) {
	_, summary, err := inspectXLSX(raw)
	return summary, err
}

func ExtractXLSX(raw []byte) (Result, error) {
	result, _, err := inspectXLSX(raw)
	return result, err
}

func inspectXLSX(raw []byte) (Result, XLSXSummary, error) {
	pkg, err := officeArchive(raw)
	if err != nil {
		return Result{}, XLSXSummary{}, err
	}
	if err = pkg.main("xl/workbook.xml", officeWorkbookType); err != nil {
		return Result{}, XLSXSummary{}, err
	}
	items, namespace, err := pkg.workbook()
	if err != nil {
		return Result{}, XLSXSummary{}, err
	}
	var shared []string
	sharedTarget := ""
	for _, relation := range pkg.rels["xl/_rels/workbook.xml.rels"] {
		if officeRelationKind(relation.kind, "sharedStrings") {
			if sharedTarget != "" {
				return Result{}, XLSXSummary{}, reject(InvalidOffice, "duplicate_shared_strings")
			}
			sharedTarget = relation.target
		}
	}
	if sharedTarget != "" {
		if pkg.types[sharedTarget] != officeStringsType {
			return Result{}, XLSXSummary{}, reject(InvalidOffice, "shared_strings_content_type")
		}
		shared, err = officeStrings(pkg.parts[sharedTarget], namespace)
		if err != nil {
			return Result{}, XLSXSummary{}, err
		}
	}
	var output officeText
	summary := XLSXSummary{Sheets: len(items)}
	for index, item := range items {
		if index != 0 {
			if err = output.add("\n\n"); err != nil {
				return Result{}, XLSXSummary{}, err
			}
		}
		state := ""
		if item.state != "" && item.state != "visible" {
			state = "; " + item.state
		}
		if err = output.add(fmt.Sprintf("Sheet %d: %s (id %d%s; %s)", item.number, item.name, item.id, state, item.target)); err != nil {
			return Result{}, XLSXSummary{}, err
		}
		cells, err := officeCells(pkg.parts[item.target], namespace, shared)
		if err != nil {
			return Result{}, XLSXSummary{}, err
		}
		rows := map[int]bool{}
		for _, cell := range cells {
			if strings.TrimSpace(cell.text) != "" {
				rows[cell.row] = true
				summary.PopulatedCells++
			}
			if cell.formulaCount != 0 {
				summary.FormulaCells++
				if cell.valueCount == 0 {
					summary.UncachedFormulaCells++
				}
			}
			label := cell.reference
			if cell.formulaCount != 0 {
				label += " [cached formula]"
			}
			if err = output.add("\n" + label + ": " + cell.text); err != nil {
				return Result{}, XLSXSummary{}, err
			}
		}
		summary.MaxPopulatedRows = max(summary.MaxPopulatedRows, len(rows))
	}
	return officeResult(output.builder.String()), summary, nil
}

func (pkg *officePackage) workbook() ([]officeItem, string, error) {
	data := pkg.parts["xl/workbook.xml"]
	root, _ := validateXML(data)
	if root.Local != "workbook" || (root.Space != officeSpreadsheetNS && root.Space != officeStrictSpreadsheetNS) {
		return nil, "", reject(InvalidOffice, "workbook_namespace")
	}
	relations := pkg.rels["xl/_rels/workbook.xml.rels"]
	var items []officeItem
	seenID, seenPart, seenName := map[int]bool{}, map[string]bool{}, map[string]bool{}
	depth, groups := 0, 0
	inSheets := false
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", reject(InvalidXML, "workbook_syntax")
		}
		switch element := token.(type) {
		case xml.StartElement:
			depth++
			if element.Name.Local == "sheets" && depth == 2 {
				if element.Name.Space != root.Space {
					return nil, "", reject(InvalidOffice, "sheets_namespace")
				}
				groups++
				inSheets = true
			}
			if element.Name.Local != "sheet" {
				continue
			}
			if !inSheets || depth != 3 || element.Name.Space != root.Space {
				return nil, "", reject(InvalidOffice, "sheet_position")
			}
			id, valid := officeNumber(attrValue(element, "sheetId"), 2147483647)
			name := attrValue(element, "name")
			state := attrValue(element, "state")
			if !valid || seenID[id] || name == "" || len([]rune(name)) > 31 || strings.ContainsAny(name, ":\\/?*[]") || seenName[strings.ToLower(name)] || state != "" && state != "visible" && state != "hidden" && state != "veryHidden" {
				return nil, "", reject(InvalidOffice, "sheet_metadata")
			}
			for _, r := range name {
				if unicode.IsControl(r) {
					return nil, "", reject(UnsafeOffice, "sheet_name_control")
				}
			}
			relation, ok := relations[officeRelationID(element)]
			number, numberOK := officePartNumber(relation.target, "xl/worksheets/sheet", ".xml")
			if !ok || !officeRelationKind(relation.kind, "worksheet") || !numberOK || seenPart[relation.target] || pkg.types[relation.target] != officeWorksheetType {
				return nil, "", reject(InvalidOffice, "worksheet_part")
			}
			seenID[id], seenPart[relation.target], seenName[strings.ToLower(name)] = true, true, true
			items = append(items, officeItem{number: number, id: id, name: name, target: relation.target, state: state})
			if len(items) > MaxOfficeParts {
				return nil, "", reject(LimitExceeded, "office_parts")
			}
		case xml.EndElement:
			if element.Name.Local == "sheets" && depth == 2 {
				inSheets = false
			}
			depth--
		}
	}
	if groups != 1 || len(items) == 0 {
		return nil, "", reject(InvalidOffice, "workbook_sheets")
	}
	sort.Slice(items, func(i, j int) bool { return items[i].number < items[j].number })
	return items, root.Space, nil
}

func officeStrings(data []byte, namespace string) ([]string, error) {
	root, _ := validateXML(data)
	if root != (xml.Name{Space: namespace, Local: "sst"}) {
		return nil, reject(InvalidOffice, "shared_strings_namespace")
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var result []string
	var current officeText
	var stack []xml.Name
	inItem := false
	phonetic := 0
	total := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, reject(InvalidXML, "shared_strings_syntax")
		}
		switch element := token.(type) {
		case xml.StartElement:
			stack = append(stack, element.Name)
			switch element.Name.Local {
			case "si", "t", "r", "rPh":
				if element.Name.Space != namespace {
					return nil, reject(InvalidOffice, "shared_string_namespace")
				}
			}
			if element.Name.Space == namespace && element.Name.Local == "si" {
				if len(stack) != 2 || inItem {
					return nil, reject(InvalidOffice, "shared_string_position")
				}
				inItem = true
				current = officeText{}
			}
			if inItem && element.Name.Space == namespace && element.Name.Local == "rPh" {
				phonetic++
			}
		case xml.CharData:
			if inItem && phonetic == 0 && len(stack) > 0 && stack[len(stack)-1] == (xml.Name{Space: namespace, Local: "t"}) {
				if err := current.add(string(element)); err != nil {
					return nil, err
				}
			}
		case xml.EndElement:
			if inItem && element.Name.Space == namespace && element.Name.Local == "rPh" {
				phonetic--
			}
			if element.Name.Space == namespace && element.Name.Local == "si" {
				value := current.builder.String()
				if len(value) > MaxTextBytes-total || len(result) >= MaxOfficeValues {
					return nil, reject(LimitExceeded, "shared_string_budget")
				}
				total += len(value)
				result = append(result, value)
				inItem = false
			}
			stack = stack[:len(stack)-1]
		}
	}
	return result, nil
}

type officeCell struct {
	reference, kind, formula, text string
	row, column                    int
	value, inline                  strings.Builder
	valueCount, formulaCount       int
}

func officeCellAddress(reference string) (int, int, bool) {
	if len(reference) < 2 || len(reference) > 10 {
		return 0, 0, false
	}
	column, index := 0, 0
	for index < len(reference) && reference[index] >= 'A' && reference[index] <= 'Z' {
		column = column*26 + int(reference[index]-'A'+1)
		index++
	}
	if index == 0 || index > 3 || column > 16384 {
		return 0, 0, false
	}
	row, ok := officeNumber(reference[index:], 1048576)
	return row, column, ok
}
func officeUnsafeFormula(value string) bool {
	upper := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToUpper(r)
	}, value)
	for _, marker := range []string{"[", "]", "|", "HTTP:", "HTTPS:", "FILE:", "FTP:", "WEBSERVICE(", "HYPERLINK(", "RTD(", "DDE(", "CUBEVALUE(", "CUBEMEMBER(", "EXEC(", "CALL(", "REGISTER(", "REGISTER.ID(", "EVALUATE(", "RUN("} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}
func officeCells(data []byte, namespace string, shared []string) ([]officeCell, error) {
	root, _ := validateXML(data)
	if root != (xml.Name{Space: namespace, Local: "worksheet"}) {
		return nil, reject(InvalidOffice, "worksheet_namespace")
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var stack []xml.Name
	var cells []officeCell
	seen := map[string]bool{}
	rowNumber := 0
	groups := 0
	var current *officeCell
	var formula strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, reject(InvalidXML, "worksheet_syntax")
		}
		switch element := token.(type) {
		case xml.StartElement:
			stack = append(stack, element.Name)
			switch element.Name.Local {
			case "sheetData", "row", "c", "v", "f", "is", "t":
				if element.Name.Space != namespace {
					return nil, reject(InvalidOffice, "cell_namespace")
				}
			}
			if element.Name.Space != namespace {
				continue
			}
			if element.Name.Local == "sheetData" {
				if len(stack) != 2 {
					return nil, reject(InvalidOffice, "sheet_data_position")
				}
				groups++
			}
			if element.Name.Local == "row" {
				if len(stack) != 3 || stack[1].Local != "sheetData" {
					return nil, reject(InvalidOffice, "row_position")
				}
				rowNumber = 0
				if number := attrValue(element, "r"); number != "" {
					var ok bool
					rowNumber, ok = officeNumber(number, 1048576)
					if !ok {
						return nil, reject(InvalidOffice, "row_reference")
					}
				}
			}
			if element.Name.Local == "c" {
				if current != nil || len(stack) != 4 || stack[2].Local != "row" {
					return nil, reject(InvalidOffice, "cell_position")
				}
				reference := attrValue(element, "r")
				row, column, ok := officeCellAddress(reference)
				if !ok || seen[reference] || rowNumber != 0 && row != rowNumber {
					return nil, reject(InvalidOffice, "cell_reference")
				}
				seen[reference] = true
				if len(seen) > MaxOfficeValues {
					return nil, reject(LimitExceeded, "cell_count")
				}
				current = &officeCell{reference: reference, kind: attrValue(element, "t"), row: row, column: column}
				formula = strings.Builder{}
			}
			if current != nil && element.Name.Local == "v" {
				if len(stack) != 5 {
					return nil, reject(InvalidOffice, "cell_value_position")
				}
				current.valueCount++
			}
			if current != nil && element.Name.Local == "f" {
				if len(stack) != 5 {
					return nil, reject(InvalidOffice, "formula_position")
				}
				current.formulaCount++
			}
		case xml.CharData:
			if current == nil || len(stack) == 0 {
				continue
			}
			name := stack[len(stack)-1]
			if name.Space != namespace {
				continue
			}
			var destination *strings.Builder
			if name.Local == "v" {
				destination = &current.value
			}
			if name.Local == "f" {
				destination = &formula
			}
			if name.Local == "t" && current.kind == "inlineStr" {
				inInline, phonetic := false, false
				for _, ancestor := range stack {
					if ancestor.Space == namespace && ancestor.Local == "is" {
						inInline = true
					}
					if ancestor.Space == namespace && ancestor.Local == "rPh" {
						phonetic = true
					}
				}
				if inInline && !phonetic {
					destination = &current.inline
				}
			}
			if destination != nil {
				if len(element) > MaxTextBytes-destination.Len() {
					return nil, reject(LimitExceeded, "cell_text_bytes")
				}
				destination.Write(element)
			}
		case xml.EndElement:
			if current != nil && element.Name.Space == namespace && element.Name.Local == "c" {
				current.formula = formula.String()
				if current.valueCount > 1 || current.formulaCount > 1 {
					return nil, reject(InvalidOffice, "duplicate_cell_value")
				}
				if officeUnsafeFormula(current.formula) {
					return nil, reject(UnsafeOffice, "external_formula_reference")
				}
				if err := officeCellValue(current, shared); err != nil {
					return nil, err
				}
				if current.text != "" || current.formulaCount != 0 {
					cells = append(cells, *current)
				}
				current = nil
			}
			stack = stack[:len(stack)-1]
		}
	}
	if groups != 1 {
		return nil, reject(InvalidOffice, "sheet_data_missing")
	}
	sort.Slice(cells, func(i, j int) bool {
		if cells[i].row != cells[j].row {
			return cells[i].row < cells[j].row
		}
		return cells[i].column < cells[j].column
	})
	return cells, nil
}
func officeCellValue(cell *officeCell, shared []string) error {
	if cell.formulaCount != 0 && cell.valueCount == 0 {
		cell.text = "[no cached value]"
		return nil
	}
	value := cell.value.String()
	switch cell.kind {
	case "s":
		trimmed := strings.TrimSpace(value)
		index, err := strconv.Atoi(trimmed)
		if err != nil || index < 0 || index >= len(shared) {
			return reject(InvalidOffice, "shared_string_index")
		}
		cell.text = shared[index]
	case "inlineStr":
		if cell.valueCount != 0 {
			return reject(InvalidOffice, "inline_string_value")
		}
		cell.text = cell.inline.String()
	case "str", "e":
		cell.text = value
	case "b":
		switch strings.TrimSpace(value) {
		case "0", "false":
			cell.text = "false"
		case "1", "true":
			cell.text = "true"
		default:
			return reject(InvalidOffice, "boolean_cell_value")
		}
	case "d":
		value = strings.TrimSpace(value)
		_, first := time.Parse(time.RFC3339Nano, value)
		_, second := time.Parse(time.DateOnly, value)
		if first != nil && second != nil {
			return reject(InvalidOffice, "date_cell_value")
		}
		cell.text = value
	case "", "n":
		value = strings.TrimSpace(value)
		if value != "" {
			number, err := strconv.ParseFloat(value, 64)
			if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
				return reject(InvalidOffice, "numeric_cell_value")
			}
			cell.text = value
		}
	default:
		return reject(InvalidOffice, "cell_type")
	}

	return nil
}

func ExtractPPTX(raw []byte) (Result, error) {
	pkg, err := officeArchive(raw)
	if err != nil {
		return Result{}, err
	}
	if err = pkg.main("ppt/presentation.xml", officePresentationType); err != nil {
		return Result{}, err
	}
	items, namespace, err := pkg.presentation()
	if err != nil {
		return Result{}, err
	}
	var output officeText
	for index, item := range items {
		if index != 0 {
			if err = output.add("\n\n"); err != nil {
				return Result{}, err
			}
		}
		text, err := officeSlideText(pkg.parts[item.target], namespace)
		if err != nil {
			return Result{}, err
		}
		if err = output.add(fmt.Sprintf("Slide %d (%s)", item.number, item.target)); err != nil {
			return Result{}, err
		}
		if text != "" {
			if err = output.add("\n" + text); err != nil {
				return Result{}, err
			}
		}
	}
	return officeResult(output.builder.String()), nil
}
func (pkg *officePackage) presentation() ([]officeItem, string, error) {
	data := pkg.parts["ppt/presentation.xml"]
	root, _ := validateXML(data)
	if root.Local != "presentation" || (root.Space != officePresentationNS && root.Space != officeStrictPresentationNS) {
		return nil, "", reject(InvalidOffice, "presentation_namespace")
	}
	relations := pkg.rels["ppt/_rels/presentation.xml.rels"]
	depth, groups := 0, 0
	inSlides := false
	var items []officeItem
	seenID := map[int]bool{}
	seenPart := map[string]bool{}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", reject(InvalidXML, "presentation_syntax")
		}
		switch element := token.(type) {
		case xml.StartElement:
			depth++
			if element.Name.Local == "sldIdLst" && depth == 2 {
				if element.Name.Space != root.Space {
					return nil, "", reject(InvalidOffice, "slides_namespace")
				}
				groups++
				inSlides = true
			}
			if element.Name.Local != "sldId" {
				continue
			}
			if !inSlides || depth != 3 || element.Name.Space != root.Space {
				return nil, "", reject(InvalidOffice, "slide_position")
			}
			id, ok := officeNumber(attrValue(element, "id"), 2147483647)
			relation, exists := relations[officeRelationID(element)]
			number, numberOK := officePartNumber(relation.target, "ppt/slides/slide", ".xml")
			if !ok || id < 256 || seenID[id] || !exists || !officeRelationKind(relation.kind, "slide") || !numberOK || seenPart[relation.target] || pkg.types[relation.target] != officeSlideType {
				return nil, "", reject(InvalidOffice, "slide_part")
			}
			seenID[id], seenPart[relation.target] = true, true
			items = append(items, officeItem{number: number, id: id, target: relation.target})
			if len(items) > MaxOfficeParts {
				return nil, "", reject(LimitExceeded, "office_parts")
			}
		case xml.EndElement:
			if element.Name.Local == "sldIdLst" && depth == 2 {
				inSlides = false
			}
			depth--
		}
	}
	if groups != 1 || len(items) == 0 {
		return nil, "", reject(InvalidOffice, "presentation_slides")
	}
	sort.Slice(items, func(i, j int) bool { return items[i].number < items[j].number })
	return items, root.Space, nil
}
func officeSlideText(data []byte, namespace string) (string, error) {
	root, _ := validateXML(data)
	if root != (xml.Name{Space: namespace, Local: "sld"}) {
		return "", reject(InvalidOffice, "slide_namespace")
	}
	drawing := officeDrawingNS
	if namespace == officeStrictPresentationNS {
		drawing = officeStrictDrawingNS
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var output officeText
	var stack []xml.Name
	pending := 0
	common, trees := 0, 0
	appendValue := func(value string) error {
		if value == "" {
			return nil
		}
		if pending != 0 {
			if err := output.add(strings.Repeat("\n", pending)); err != nil {
				return err
			}
			pending = 0
		}
		return output.add(value)
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", reject(InvalidXML, "slide_syntax")
		}
		switch element := token.(type) {
		case xml.StartElement:
			stack = append(stack, element.Name)
			if element.Name.Local == "cSld" {
				if element.Name.Space != namespace || len(stack) != 2 {
					return "", reject(InvalidOffice, "slide_common_position")
				}
				common++
			}
			if element.Name.Local == "spTree" {
				if element.Name.Space != namespace || len(stack) != 3 || stack[1].Local != "cSld" {
					return "", reject(InvalidOffice, "slide_tree_position")
				}
				trees++
			}
			switch element.Name.Local {
			case "t", "p", "br", "tab":
				if element.Name.Space != drawing {
					return "", reject(InvalidOffice, "slide_text_namespace")
				}
				if len(stack) < 4 || stack[1] != (xml.Name{Space: namespace, Local: "cSld"}) || stack[2] != (xml.Name{Space: namespace, Local: "spTree"}) {
					return "", reject(InvalidOffice, "slide_text_position")
				}
			}
			if element.Name.Space == drawing {
				switch element.Name.Local {
				case "br":
					if err := appendValue("\n"); err != nil {
						return "", err
					}
				case "tab":
					if err := appendValue("\t"); err != nil {
						return "", err
					}
				}
			}
		case xml.CharData:
			if len(stack) > 0 && stack[len(stack)-1] == (xml.Name{Space: drawing, Local: "t"}) {
				if err := appendValue(string(element)); err != nil {
					return "", err
				}
			}
		case xml.EndElement:
			if element.Name.Space == drawing && element.Name.Local == "p" {
				pending++
				if pending > MaxTextBytes {
					return "", reject(LimitExceeded, "slide_structural_text")
				}
			}
			stack = stack[:len(stack)-1]
		}
	}
	if common != 1 || trees != 1 {
		return "", reject(InvalidOffice, "slide_required_parts")
	}
	return output.builder.String(), nil
}

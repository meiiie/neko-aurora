package documents

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxDOCXBytes         = 1 << 20
	MaxArchiveEntries    = 256
	MaxUncompressedBytes = 8 << 20
	MaxXMLBytes          = 2 << 20
	MaxTextBytes         = 512 << 10
	MaxXMLDepth          = 64

	wordNamespace                  = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	strictWordNamespace            = "http://purl.oclc.org/ooxml/wordprocessingml/main"
	contentTypesNamespace          = "http://schemas.openxmlformats.org/package/2006/content-types"
	relationshipsNamespace         = "http://schemas.openxmlformats.org/package/2006/relationships"
	mainDocumentContentType        = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	mainDocumentRelationship       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	strictMainDocumentRelationship = "http://purl.oclc.org/ooxml/officeDocument/relationships/officeDocument"
)

type Result struct {
	Text       string
	TextSHA256 string
	TextBytes  int
}

type ErrorKind string

const (
	InvalidDOCX   ErrorKind = "invalid_docx"
	UnsafeDOCX    ErrorKind = "unsafe_docx"
	LimitExceeded ErrorKind = "limit_exceeded"
	CorruptDOCX   ErrorKind = "corrupt_docx"
	InvalidXML    ErrorKind = "invalid_xml"
)

type Error struct {
	Kind   ErrorKind
	Reason string
}

func (e *Error) Error() string {
	return "documents: " + string(e.Kind) + " (" + e.Reason + ")"
}

func ExtractDOCX(raw []byte) (Result, error) {
	if len(raw) > MaxDOCXBytes {
		return Result{}, reject(LimitExceeded, "raw_bytes")
	}
	if len(raw) < 4 || !bytes.Equal(raw[:4], []byte("PK\x03\x04")) {
		return Result{}, reject(InvalidDOCX, "zip_signature")
	}
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return Result{}, reject(CorruptDOCX, "zip_directory")
	}
	if len(archive.File) > MaxArchiveEntries {
		return Result{}, reject(LimitExceeded, "archive_entries")
	}
	parts := make(map[string][]byte, len(archive.File))
	names := make(map[string]bool, len(archive.File))
	var claimed uint64
	for _, file := range archive.File {
		name := strings.TrimSuffix(file.Name, "/")
		typeBits := file.Mode().Type()
		if !safePartName(name) || dangerousPart(name) || typeBits != 0 && typeBits != os.ModeDir {
			return Result{}, reject(UnsafeDOCX, "archive_entry")
		}
		key := strings.ToLower(name)
		if names[key] {
			return Result{}, reject(UnsafeDOCX, "duplicate_entry")
		}
		names[key] = true
		if file.Flags&0x41 != 0 || file.Method != zip.Store && file.Method != zip.Deflate {
			return Result{}, reject(UnsafeDOCX, "archive_encoding")
		}
		if file.UncompressedSize64 > MaxUncompressedBytes || claimed > MaxUncompressedBytes-file.UncompressedSize64 {
			return Result{}, reject(LimitExceeded, "claimed_uncompressed_bytes")
		}
		claimed += file.UncompressedSize64
		if isXML(name) && file.UncompressedSize64 > MaxXMLBytes {
			return Result{}, reject(LimitExceeded, "xml_bytes")
		}
		if file.FileInfo().IsDir() && file.UncompressedSize64 != 0 {
			return Result{}, reject(UnsafeDOCX, "directory_payload")
		}
	}
	actual := 0
	for _, file := range archive.File {
		limit := MaxUncompressedBytes - actual
		if isXML(file.Name) && limit > MaxXMLBytes {
			limit = MaxXMLBytes
		}
		stream, err := file.Open()
		if err != nil {
			return Result{}, reject(CorruptDOCX, "zip_entry_open")
		}
		data, readErr := io.ReadAll(io.LimitReader(stream, int64(limit)+1))
		closeErr := stream.Close()
		if len(data) > limit {
			return Result{}, reject(LimitExceeded, "actual_uncompressed_bytes")
		}
		if readErr != nil || closeErr != nil || uint64(len(data)) != file.UncompressedSize64 {
			return Result{}, reject(CorruptDOCX, "zip_entry_integrity")
		}
		actual += len(data)
		if file.FileInfo().IsDir() {
			continue
		}
		if isXML(file.Name) {
			if _, err := validateXML(data); err != nil {
				return Result{}, err
			}
		}
		parts[file.Name] = data
	}
	contentTypes, hasTypes := parts["[Content_Types].xml"]
	rootRelationships, hasRelationships := parts["_rels/.rels"]
	document, hasDocument := parts["word/document.xml"]
	if !hasTypes || !hasRelationships || !hasDocument {
		return Result{}, reject(InvalidDOCX, "required_parts")
	}
	if err := validateContentTypes(contentTypes, parts); err != nil {
		return Result{}, err
	}
	if err := validateRelationships("_rels/.rels", rootRelationships, parts, true); err != nil {
		return Result{}, err
	}
	for name, data := range parts {
		if name != "_rels/.rels" && strings.HasSuffix(strings.ToLower(name), ".rels") {
			if err := validateRelationships(name, data, parts, false); err != nil {
				return Result{}, err
			}
		}
	}
	text, err := extractDocument(document)
	if err != nil {
		return Result{}, err
	}
	sum := sha256.Sum256([]byte(text))
	return Result{Text: text, TextSHA256: hex.EncodeToString(sum[:]), TextBytes: len(text)}, nil
}

func reject(kind ErrorKind, reason string) error {
	return &Error{Kind: kind, Reason: reason}
}

func safePartName(name string) bool {
	if name == "" || len(name) > 1024 || !utf8.ValidString(name) || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:%?#<>\"|*") || path.Clean(name) != name {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") {
			return false
		}
		base := strings.ToUpper(strings.SplitN(segment, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
			return false
		}
	}
	return true
}

func dangerousPart(name string) bool {
	lower := strings.ToLower(name)
	for _, marker := range []string{"vbaproject", "macros"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	for _, segment := range strings.Split(lower, "/") {
		if segment == "activex" || segment == "embeddings" {
			return true
		}
	}
	switch path.Ext(lower) {
	case ".bin", ".exe", ".dll", ".com", ".bat", ".cmd", ".ps1", ".js", ".vbs", ".hta", ".ole", ".docm", ".dotm", ".html", ".htm", ".mhtml":
		return true
	}
	return false
}

func isXML(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".xml") || strings.HasSuffix(lower, ".rels")
}

func validateXML(data []byte) (xml.Name, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	depth, roots := 0, 0
	declaration := false
	var root xml.Name
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			if depth != 0 || roots != 1 {
				return root, reject(InvalidXML, "xml_root")
			}
			return root, nil
		}
		if err != nil {
			return root, reject(InvalidXML, "xml_syntax")
		}
		switch token := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				root = token.Name
				if roots != 1 {
					return root, reject(InvalidXML, "multiple_xml_roots")
				}
			}
			depth++
			if depth > MaxXMLDepth {
				return root, reject(LimitExceeded, "xml_depth")
			}
			seen := make(map[xml.Name]bool, len(token.Attr))
			for _, attr := range token.Attr {
				if seen[attr.Name] {
					return root, reject(InvalidXML, "duplicate_xml_attribute")
				}
				seen[attr.Name] = true
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && len(bytes.TrimSpace(token)) != 0 {
				return root, reject(InvalidXML, "outside_xml_root")
			}
		case xml.Directive:
			return root, reject(UnsafeDOCX, "xml_directive")
		case xml.ProcInst:
			if token.Target != "xml" || declaration || roots != 0 {
				return root, reject(UnsafeDOCX, "xml_processing_instruction")
			}
			declaration = true
		}
	}
}

func attrValue(element xml.StartElement, name string) string {
	for _, attr := range element.Attr {
		if attr.Name.Space == "" && attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func unsafeContentType(contentType string) bool {
	lower := strings.ToLower(contentType)
	for _, marker := range []string{"macroenabled", "vbaproject", "activex", "oleobject", "officedocument.package", "javascript", "text/html"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func validateContentTypes(data []byte, parts map[string][]byte) error {
	root, _ := validateXML(data)
	if root.Space != contentTypesNamespace || root.Local != "Types" {
		return reject(InvalidDOCX, "content_types_namespace")
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	seen := map[string]bool{}
	mainType := false
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return reject(InvalidXML, "content_types_syntax")
		}
		switch token := token.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				continue
			}
			contentType := attrValue(token, "ContentType")
			if unsafeContentType(contentType) {
				return reject(UnsafeDOCX, "active_content_type")
			}
			if depth != 2 || token.Name.Space != contentTypesNamespace || contentType == "" {
				return reject(InvalidDOCX, "content_type_entry")
			}
			var key string
			switch token.Name.Local {
			case "Override":
				part := attrValue(token, "PartName")
				if !strings.HasPrefix(part, "/") || !safePartName(strings.TrimPrefix(part, "/")) {
					return reject(UnsafeDOCX, "content_type_part")
				}
				if _, exists := parts[strings.TrimPrefix(part, "/")]; !exists {
					return reject(InvalidDOCX, "content_type_missing_part")
				}
				key = "part:" + strings.ToLower(part)
				if part == "/word/document.xml" {
					mainType = contentType == mainDocumentContentType
				}
			case "Default":
				extension := attrValue(token, "Extension")
				if extension == "" || strings.ContainsAny(extension, ".:/\\%?#") {
					return reject(InvalidDOCX, "content_type_extension")
				}
				key = "extension:" + strings.ToLower(extension)
			default:
				return reject(InvalidDOCX, "content_type_element")
			}
			if seen[key] {
				return reject(InvalidDOCX, "duplicate_content_type")
			}
			seen[key] = true
		case xml.EndElement:
			depth--
		}
	}
	if !mainType {
		return reject(InvalidDOCX, "main_document_content_type")
	}
	return nil
}

func validateRelationships(name string, data []byte, parts map[string][]byte, rootPart bool) error {
	root, _ := validateXML(data)
	if root.Space != relationshipsNamespace || root.Local != "Relationships" {
		return reject(InvalidDOCX, "relationships_namespace")
	}
	base := ""
	if !rootPart {
		directory := path.Dir(name)
		if path.Base(directory) != "_rels" {
			return reject(UnsafeDOCX, "relationship_part_path")
		}
		base = path.Dir(directory)
		source := path.Join(base, strings.TrimSuffix(path.Base(name), ".rels"))
		if _, exists := parts[source]; !exists {
			return reject(UnsafeDOCX, "relationship_missing_source")
		}
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	seen := map[string]bool{}
	mainRelations, depth := 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return reject(InvalidXML, "relationships_syntax")
		}
		switch token := token.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				continue
			}
			if depth != 2 || token.Name.Space != relationshipsNamespace || token.Name.Local != "Relationship" {
				return reject(InvalidDOCX, "relationship_element")
			}
			id, relationType, target := attrValue(token, "Id"), attrValue(token, "Type"), attrValue(token, "Target")
			mode := attrValue(token, "TargetMode")
			if id == "" || seen[id] || relationType == "" || target == "" {
				return reject(InvalidDOCX, "relationship_attributes")
			}
			seen[id] = true
			if mode == "External" {
				if !inertHyperlink(relationType, target) {
					return reject(UnsafeDOCX, "external_relationship")
				}
				continue
			}
			if mode != "" && mode != "Internal" || strings.HasPrefix(target, "/") || strings.ContainsAny(target, "\\:%?#") {
				return reject(UnsafeDOCX, "external_relationship")
			}
			resolved := path.Join(base, target)
			if !safePartName(resolved) || dangerousPart(resolved) || unsafeContentType(relationType) || strings.Contains(strings.ToLower(relationType), "attachedtemplate") {
				return reject(UnsafeDOCX, "relationship_target")
			}
			if _, exists := parts[resolved]; !exists {
				return reject(UnsafeDOCX, "relationship_missing_target")
			}
			if rootPart && (relationType == mainDocumentRelationship || relationType == strictMainDocumentRelationship) {
				mainRelations++
				if resolved != "word/document.xml" {
					return reject(InvalidDOCX, "main_document_part")
				}
			}
		case xml.EndElement:
			depth--
		}
	}
	if rootPart && mainRelations != 1 {
		return reject(InvalidDOCX, "main_document_relationship")
	}
	return nil
}

func inertHyperlink(relationType, target string) bool {
	if relationType != "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" && relationType != "http://purl.oclc.org/ooxml/officeDocument/relationships/hyperlink" {
		return false
	}
	link, err := url.Parse(target)
	if err != nil || link.User != nil {
		return false
	}
	switch link.Scheme {
	case "http", "https":
		return link.Host != ""
	case "mailto":
		return link.Opaque != ""
	}
	return false
}

func extractDocument(data []byte) (string, error) {
	root, _ := validateXML(data)
	if root.Local != "document" || root.Space != wordNamespace && root.Space != strictWordNamespace {
		return "", reject(InvalidDOCX, "document_namespace")
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	stack := make([]xml.Name, 0, MaxXMLDepth)
	text := make([]byte, 0)
	pending := make([]byte, 0)
	bodyDepth, bodies := 0, 0
	appendText := func(value string) error {
		if len(pending)+len(value) > MaxTextBytes-len(text) {
			return reject(LimitExceeded, "extracted_text_bytes")
		}
		text = append(text, pending...)
		pending = pending[:0]
		text = append(text, value...)
		return nil
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", reject(InvalidXML, "document_syntax")
		}
		switch token := token.(type) {
		case xml.StartElement:
			stack = append(stack, token.Name)
			if token.Name.Space != root.Space {
				continue
			}
			if token.Name.Local == "body" {
				if len(stack) != 2 {
					return "", reject(InvalidDOCX, "document_body_position")
				}
				bodies++
				bodyDepth = len(stack)
			}
			if bodyDepth != 0 {
				separator := ""
				switch token.Name.Local {
				case "tab":
					separator = "\t"
				case "br", "cr":
					separator = "\n"
				}
				if separator != "" {
					if err := appendText(separator); err != nil {
						return "", err
					}
				}
			}
		case xml.CharData:
			if bodyDepth != 0 && len(stack) > 0 && stack[len(stack)-1] == (xml.Name{Space: root.Space, Local: "t"}) {
				if err := appendText(string(token)); err != nil {
					return "", err
				}
			}
		case xml.EndElement:
			if bodyDepth != 0 && token.Name.Space == root.Space {
				separator := ""
				switch token.Name.Local {
				case "p", "tr":
					if token.Name.Local == "tr" && len(pending) > 0 && pending[len(pending)-1] == '\t' {
						pending = pending[:len(pending)-1]
					}
					separator = "\n"
				case "tc":
					if len(pending) > 0 && pending[len(pending)-1] == '\n' {
						pending = pending[:len(pending)-1]
					}
					separator = "\t"
				}
				if len(pending)+len(separator) > MaxTextBytes {
					return "", reject(LimitExceeded, "structural_text_bytes")
				}
				pending = append(pending, separator...)
				if token.Name.Local == "body" {
					bodyDepth = 0
				}
			}
			stack = stack[:len(stack)-1]
		}
	}
	if bodies != 1 {
		return "", reject(InvalidDOCX, "document_body_count")
	}
	pending = bytes.TrimRight(pending, "\n")
	if err := appendText(""); err != nil {
		return "", err
	}
	return string(text), nil
}

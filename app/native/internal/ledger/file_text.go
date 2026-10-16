package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
	"unicode/utf8"

	"nekobot.local/native/internal/documents"
)

const docxMIME = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
const xlsxMIME = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
const pptxMIME = "application/vnd.openxmlformats-officedocument.presentationml.presentation"

type DocumentExtractor interface {
	Extract(format string, raw []byte) (documents.Result, error)
}

func (s *Store) extractFileText(filename string, raw []byte) (documents.Result, string, string, error) {
	var format, mime string
	switch strings.ToLower(filepathExt(filename)) {
	case ".docx":
		format, mime = "docx", docxMIME
	case ".xlsx":
		format, mime = "xlsx", xlsxMIME
	case ".pptx":
		format, mime = "pptx", pptxMIME
	default:
		return documents.Result{}, "", "", ErrUnsupported
	}
	var out documents.Result
	var err error
	if s.documentExtractor != nil {
		out, err = s.documentExtractor.Extract(format, raw)
	} else {
		switch format {
		case "docx":
			out, err = documents.ExtractDOCX(raw)
		case "xlsx":
			out, err = documents.ExtractXLSX(raw)
		case "pptx":
			out, err = documents.ExtractPPTX(raw)
		}
	}
	if err == nil && !validDocumentResult(out) {
		err = ErrUnavailable
	}
	if err != nil {
		out = documents.Result{}
	}
	return out, format + "_ooxml", mime, err
}

func validDocumentResult(result documents.Result) bool {
	if result.TextBytes <= 0 || result.TextBytes > documents.MaxTextBytes || len(result.Text) != result.TextBytes || !utf8.ValidString(result.Text) {
		return false
	}
	digest := sha256.Sum256([]byte(result.Text))
	return result.TextSHA256 == hex.EncodeToString(digest[:])
}

func extractedFormat(f UploadedFile) bool {
	switch strings.ToLower(filepathExt(f.Filename)) {
	case ".docx":
		return f.TextFormat == "docx_ooxml" && f.MIME == docxMIME
	case ".xlsx":
		return f.TextFormat == "xlsx_ooxml" && f.MIME == xlsxMIME
	case ".pptx":
		return f.TextFormat == "pptx_ooxml" && f.MIME == pptxMIME
	}
	return false
}

func safeExtractedText(text string) bool {
	return utf8.ValidString(text) && strings.IndexFunc(text, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }) < 0 && !hasSecret([]byte(text))
}

func extractedDescriptor(f UploadedFile) bool {
	return f.ExtractionVersion == 1 && extractedFormat(f) && f.TextBytes > 0 && f.TextBytes <= documents.MaxTextBytes && lowerHex(f.TextSHA256, 64) && safeExtractedText(f.Excerpt)
}

func filepathExt(name string) string {
	at := strings.LastIndexByte(name, '.')
	if at < 0 {
		return ""
	}
	return name[at:]
}

func fileTextIdentity(f UploadedFile) (string, int) {
	if f.ExtractionVersion == 1 {
		return f.TextSHA256, f.TextBytes
	}
	return f.SHA256, f.Bytes
}

func (s *Store) fileTextBytes(f UploadedFile, raw []byte) ([]byte, error) {
	h := sha256.Sum256(raw)
	if len(raw) != f.Bytes || hex.EncodeToString(h[:]) != f.SHA256 {
		return nil, ErrUnavailable
	}
	if f.Status != "ready_text" {
		return nil, ErrConflict
	}
	if f.ExtractionVersion == 0 {
		if !readyUploadText(f.Filename, raw) || hasSecret(raw) {
			return nil, ErrUnavailable
		}
		return raw, nil
	}
	if !extractedDescriptor(f) {
		return nil, ErrUnavailable
	}
	text, _, _, err := s.extractFileText(f.Filename, raw)
	if err != nil || text.TextSHA256 != f.TextSHA256 || text.TextBytes != f.TextBytes || !safeExtractedText(text.Text) || !strings.HasPrefix(text.Text, f.Excerpt) {
		return nil, ErrUnavailable
	}
	return []byte(text.Text), nil
}

type FileTextPreview struct {
	File            UploadedFile `json:"file"`
	ProjectRevision uint64       `json:"projectRevision"`
	Text            string       `json:"text"`
	TextSHA256      string       `json:"textSHA256"`
	TextBytes       int          `json:"textBytes"`
	PreviewBytes    int          `json:"previewBytes"`
	Truncated       bool         `json:"truncated"`
}

func (s *Store) PreviewProjectFile(project, channel, id string, projectRevision, accessRevision uint64) (FileTextPreview, error) {
	s.mu.Lock()
	f, ok := s.uploads[id]
	if !ok || f.ProjectID != project || f.ChannelID != channel {
		s.mu.Unlock()
		return FileTextPreview{}, ErrNotFound
	}
	p, ok := s.projects[project]
	if !ok || p.Revision != projectRevision || !contains(p.ChannelIDs, channel) || !f.Granted || f.AccessRevision != accessRevision || f.Status != "ready_text" {
		s.mu.Unlock()
		return FileTextPreview{}, ErrConflict
	}
	s.mu.Unlock()
	raw, err := s.readUploadedBytes(f)
	if err != nil {
		return FileTextPreview{}, err
	}
	text, err := s.fileTextBytes(f, raw)
	if err != nil {
		return FileTextPreview{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, found := s.uploads[id]
	currentProject, projectFound := s.projects[project]
	if s.closed || s.poisoned {
		return FileTextPreview{}, ErrUnavailable
	}
	if !found || current != f || !projectFound || currentProject.Revision != projectRevision || !contains(currentProject.ChannelIDs, channel) {
		return FileTextPreview{}, ErrConflict
	}
	sha, size := fileTextIdentity(f)
	preview := text
	if len(preview) > 16000 {
		preview = preview[:16000]
		for !utf8.Valid(preview) {
			preview = preview[:len(preview)-1]
		}
	}
	return FileTextPreview{File: publicFile(f), ProjectRevision: p.Revision, Text: string(preview), TextSHA256: sha, TextBytes: size, PreviewBytes: len(preview), Truncated: len(preview) < len(text)}, nil
}

func (s *Store) preparePinnedFileText(id string, epoch uint64, fileID string) (UploadedFile, []byte, error) {
	s.mu.Lock()
	w, found := s.work[id]
	if !found {
		s.mu.Unlock()
		return UploadedFile{}, nil, ErrNotFound
	}
	release, guardErr := s.executionPrincipalGuard(w)
	if guardErr != nil {
		s.mu.Unlock()
		return UploadedFile{}, nil, guardErr
	}
	defer release()
	var source UploadedFile
	if found && w.Epoch == epoch && w.Status == "running" && s.contextAllowed(w) && w.Resources != nil {
		for _, f := range w.Resources.Files {
			if f.ID == fileID {
				source = f
				break
			}
		}
	}
	s.mu.Unlock()
	if source.ID == "" {
		return UploadedFile{}, nil, ErrConflict
	}
	raw, err := s.readUploadedBytes(source)
	if err != nil {
		return source, nil, err
	}
	text, err := s.fileTextBytes(source, raw)
	return source, text, err
}

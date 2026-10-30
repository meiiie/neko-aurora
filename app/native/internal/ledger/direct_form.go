package ledger

import (
	"strings"
	"unicode/utf8"
)

type DirectFormField struct {
	Name     string   `json:"name"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Options  []string `json:"options,omitempty"`
	Required bool     `json:"required,omitempty"`
}

type DirectForm struct {
	AnswerVersion int               `json:"answerVersion,omitempty"`
	Title         string            `json:"title"`
	Fields        []DirectFormField `json:"fields"`
	// Done is presentation-only for legacy cache compatibility, never journaled true.
	Done bool `json:"done,omitempty"`
}

func CopyDirectForm(f *DirectForm) *DirectForm {
	if f == nil {
		return nil
	}
	out := *f
	out.Fields = append([]DirectFormField(nil), f.Fields...)
	for i := range out.Fields {
		out.Fields[i].Options = append([]string(nil), f.Fields[i].Options...)
	}
	return &out
}

func ValidDirectForm(f *DirectForm) bool {
	if f == nil {
		return true
	}
	text := func(s string, n int) bool {
		return utf8.ValidString(s) && strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= n
	}
	if f.Done || f.AnswerVersion < 0 || f.AnswerVersion > 1 || !text(f.Title, 80) || len(f.Fields) == 0 || len(f.Fields) > 10 {
		return false
	}
	names := map[string]bool{}
	for _, field := range f.Fields {
		if !text(field.Name, 40) || field.Name != strings.TrimSpace(field.Name) || names[field.Name] || !text(field.Label, 80) {
			return false
		}
		names[field.Name] = true
		switch field.Type {
		case "text", "number", "date", "textarea":
			if len(field.Options) > 0 {
				return false
			}
		case "select":
			if len(field.Options) == 0 || len(field.Options) > 8 {
				return false
			}
			options := map[string]bool{}
			for _, option := range field.Options {
				if !text(option, 60) || option != strings.TrimSpace(option) || options[option] {
					return false
				}
				options[option] = true
			}
		default:
			return false
		}
	}
	return true
}

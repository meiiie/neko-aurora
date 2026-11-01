package aurora

import (
	"regexp"
	"strings"
	"time"
	"unicode"

	"nekoaurora.local/native/internal/ledger"
)

var previewControl = regexp.MustCompile(`(?m)(?:^|\n)\s*(?:PLAN|CHOICES|QUESTIONS|APPROVAL|SECRET|TOOL|MEMORY|HANDOFF|SCHEDULE|FORM):|(?:PLAN|CHOICES|QUESTIONS|APPROVAL|SECRET|TOOL|MEMORY|HANDOFF|SCHEDULE|FORM):\s*[\[{]`)

func interruptedText(text string) string {
	// A cancelled response may contain half a multiline tool/card payload.
	// Keep only the same prose prefix the streaming UI can display.
	if at := previewControl.FindStringIndex(text); at != nil {
		text = text[:at[0]]
	}
	text = strings.TrimSpace(text)
	if !strings.ContainsAny(text, " \n\t") || !strings.ContainsFunc(text, unicode.IsLetter) {
		return ""
	}
	return text
}

func (s *Service) keepInterrupted(w ledger.Work, text string) error {
	queued := s.inbox(w)
	if text == "" || len(queued) == 0 {
		return nil
	}
	text = s.redact(scrubInternal(text))
	text = strings.ToValidUTF8(text[:min(len(text), 60000)], "")
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return s.update(w.RequestID, func(r *ledger.DirectRun) {
		if len(r.Partials) >= ledger.MaxSteered || len(r.Steered) >= ledger.MaxSteered {
			return
		}
		r.Partials = append(r.Partials, ledger.DirectPartial{Text: text, At: s.Now().UTC().Format(time.RFC3339Nano), BeforeRequestID: queued[0].RequestID})
	})
}

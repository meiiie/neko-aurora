package ledger

import (
	"strconv"
	"strings"
)

type DirectQuestion struct {
	ID       string   `json:"id"`
	Prompt   string   `json:"prompt"`
	Options  []string `json:"options"`
	Multiple bool     `json:"multiple,omitempty"`
}

func CopyDirectQuestions(questions []DirectQuestion) []DirectQuestion {
	if questions == nil {
		return nil
	}
	out := append([]DirectQuestion(nil), questions...)
	for i := range out {
		out[i].Options = append([]string(nil), questions[i].Options...)
	}
	return out
}

func ValidDirectQuestions(questions []DirectQuestion) bool {
	if len(questions) > 4 {
		return false
	}
	for i, q := range questions {
		if q.ID != "q"+strconv.Itoa(i+1) || !directText(q.Prompt, 800) || strings.TrimSpace(q.Prompt) == "" || len(q.Options) < 2 || len(q.Options) > 8 {
			return false
		}
		seen := map[string]bool{}
		for _, option := range q.Options {
			if !directText(option, 640) || strings.TrimSpace(option) == "" || seen[option] {
				return false
			}
			seen[option] = true
		}
	}
	return true
}

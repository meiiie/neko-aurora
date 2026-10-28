package ledger

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var formNumber = regexp.MustCompile(`^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$`)

type OwnerFormAnswer struct {
	TaskID string   `json:"taskId"`
	Values []string `json:"values"`
}

func copyOwnerInput(in OwnerMessageInput) OwnerMessageInput {
	if in.Form != nil {
		f := *in.Form
		f.Values = append([]string(nil), f.Values...)
		in.Form = &f
	}
	return in
}

func validOwnerFormInput(in OwnerMessageInput) bool {
	if in.Form == nil {
		return true
	}
	f := in.Form
	if !directText(f.TaskID, 128) || f.TaskID == "" || in.ClientNonce != "form-"+f.TaskID || in.ReferenceID != "" || in.ReplyTo != "" || len(f.Values) < 1 || len(f.Values) > 10 {
		return false
	}
	for _, v := range f.Values {
		if !utf8.ValidString(v) || utf8.RuneCountInString(v) > 2000 {
			return false
		}
	}
	return true
}

func (s *Store) validOwnerFormSource(in OwnerMessageInput, bots []string) bool {
	if !validOwnerFormInput(in) {
		return false
	}
	if in.Form == nil {
		if w, ok := s.work[strings.TrimPrefix(in.ClientNonce, "form-")]; strings.HasPrefix(in.ClientNonce, "form-") && ok && w.Direct != nil && w.Direct.Form != nil && w.Direct.Form.AnswerVersion == 1 {
			return false
		}
		return true
	}
	w, ok := s.work[in.Form.TaskID]
	conversation := "conversation-" + in.ThreadID
	if validChannelID(in.ThreadID) {
		conversation = in.ThreadID
	}
	if !ok || w.Status != "completed" || w.Direct == nil || w.Direct.Form == nil || w.ConversationID != conversation || len(bots) != 1 || bots[0] != w.BotID {
		return false
	}
	text, err := DirectFormAnswerText(w.Direct.Form, in.Form.Values)
	return err == nil && in.Text == text
}

func DirectFormAnswerText(f *DirectForm, values []string) (string, error) {
	if f == nil || f.AnswerVersion != 1 || !ValidDirectForm(f) || len(values) != len(f.Fields) {
		return "", ErrInvalid
	}
	lines := []string{"📋 " + f.Title}
	for i, field := range f.Fields {
		raw := values[i]
		value := strings.TrimSpace(raw)
		if !utf8.ValidString(raw) || utf8.RuneCountInString(raw) > 2000 || field.Required && value == "" {
			return "", ErrInvalid
		}
		if value != "" {
			switch field.Type {
			case "number":
				number, err := strconv.ParseFloat(value, 64)
				if !formNumber.MatchString(value) || err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
					return "", ErrInvalid
				}
			case "date":
				date, err := time.Parse("2006-01-02", value)
				if err != nil || date.Year() < 1 || date.Format("2006-01-02") != value {
					return "", ErrInvalid
				}
			case "select":
				if !slices.Contains(field.Options, value) {
					return "", ErrInvalid
				}
			}
		}
		if value == "" {
			value = "(bỏ trống)"
		}
		lines = append(lines, "- "+field.Label+": "+value)
	}
	text := strings.Join(lines, "\n")
	if utf8.RuneCountInString(text) > 10000 || len(text) > 32000 {
		return "", ErrInvalid
	}
	return text, nil
}

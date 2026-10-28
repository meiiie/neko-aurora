package aurora

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"nekoaurora.local/native/internal/ledger"
)

type questionSet struct {
	Choices   []string
	Questions []ledger.DirectQuestion
}

var questionsLine = regexp.MustCompile(`(?m)^[ \t]*QUESTIONS:[ \t]*`)

func splitQuestions(text string) (string, []ledger.DirectQuestion) {
	var questions []ledger.DirectQuestion
	for {
		at := questionsLine.FindStringIndex(text)
		if at == nil {
			return text, questions
		}
		decoder := json.NewDecoder(strings.NewReader(text[at[1]:]))
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil {
			return text, questions
		}
		if parsed := parseQuestions(string(raw)); parsed != nil {
			questions = parsed
		}
		text = text[:at[0]] + text[at[1]+int(decoder.InputOffset()):]
	}
}

func questionsOf(d directives) *questionSet {
	if len(d.Choices) == 0 && len(d.Questions) == 0 {
		return nil
	}
	return &questionSet{Choices: d.Choices, Questions: d.Questions}
}

func parseQuestions(raw string) []ledger.DirectQuestion {
	var questions []ledger.DirectQuestion
	if json.Unmarshal([]byte(raw), &questions) != nil || len(questions) == 0 || len(questions) > 4 {
		return nil
	}
	for i := range questions {
		questions[i].ID = "q" + itoa(i+1)
		questions[i].Prompt = clean(questions[i].Prompt, 200)
		for j := range questions[i].Options {
			questions[i].Options[j] = clean(questions[i].Options[j], 120)
		}
	}
	if !ledger.ValidDirectQuestions(questions) {
		return nil
	}
	return questions
}

type questionAnswer struct {
	ID       string `json:"id"`
	Selected []int  `json:"selected,omitempty"`
	Text     string `json:"text,omitempty"`
}

func questionAnswerText(questions []ledger.DirectQuestion, answers []questionAnswer) (string, error) {
	if len(questions) == 0 || len(answers) != len(questions) {
		return "", ErrInvalid
	}
	var lines []string
	for i, q := range questions {
		a := answers[i]
		custom := strings.TrimSpace(a.Text)
		if a.ID != q.ID || !utf8.ValidString(a.Text) || utf8.RuneCountInString(a.Text) > 2000 || len(a.Selected) > len(q.Options) || !q.Multiple && len(a.Selected) > 1 || custom != "" && len(a.Selected) > 0 || custom == "" && len(a.Selected) == 0 {
			return "", ErrInvalid
		}
		selected := map[int]bool{}
		for _, index := range a.Selected {
			if index < 0 || index >= len(q.Options) || selected[index] {
				return "", ErrInvalid
			}
			selected[index] = true
		}
		parts := []string{}
		for index, option := range q.Options {
			if selected[index] {
				parts = append(parts, option)
			}
		}
		if custom != "" {
			parts = append(parts, custom)
		}
		lines = append(lines, q.Prompt+"\n"+strings.Join(parts, "; "))
	}
	return "Trả lời câu hỏi:\n\n" + strings.Join(lines, "\n\n"), nil
}

package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func ownerFormFixture(t *testing.T, dir string) (*Store, OwnerMessageInput, OwnerMessagePlan) {
	t.Helper()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	bot := s.Snapshot().Bots[0].ID
	w, _, err := s.Accept(Input{BotID: bot, ClientNonce: "typed-form-source", Text: "Collect"})
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.DirectStart(w.RequestID, w.Epoch, DirectRun{Runtime: "direct-http"})
	if err != nil {
		t.Fatal(err)
	}
	f := directFormFixture()
	f.AnswerVersion = 1
	_, err = s.DirectFinish(w.RequestID, w.Epoch, "completed", "", func(d *DirectRun) { d.Form = f })
	if err != nil {
		t.Fatal(err)
	}
	values := []string{"Bảng"}
	text, err := DirectFormAnswerText(f, values)
	if err != nil {
		t.Fatal(err)
	}
	return s, OwnerMessageInput{ThreadID: bot, ClientNonce: "form-" + w.RequestID, Text: text, Form: &OwnerFormAnswer{TaskID: w.RequestID, Values: values}}, OwnerMessagePlan{BotIDs: []string{bot}}
}

func TestOwnerFormReceiptSourceCopiesAndReplay(t *testing.T) {
	dir := t.TempDir()
	s, in, plan := ownerFormFixture(t, dir)
	defer func() { s.Close() }()
	for name, mutate := range map[string]func(*OwnerMessageInput){
		"downgrade":      func(i *OwnerMessageInput) { i.Form = nil },
		"forged-text":    func(i *OwnerMessageInput) { i.Text = "forged" },
		"invalid-option": func(i *OwnerMessageInput) { i.Form.Values[0] = "not an option" },
		"missing-source": func(i *OwnerMessageInput) { i.Form.TaskID = "missing"; i.ClientNonce = "form-missing" },
		"wrong-nonce":    func(i *OwnerMessageInput) { i.ClientNonce = "arbitrary-nonce" },
		"reference":      func(i *OwnerMessageInput) { i.ReferenceID = "another-thread" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := copyOwnerInput(in)
			mutate(&bad)
			if _, _, err := s.AcceptOwnerMessage(bad, plan); !errors.Is(err, ErrInvalid) {
				t.Fatal("invalid form admitted", err)
			}
		})
	}
	original := copyOwnerInput(in)
	works, _, err := s.AcceptOwnerMessage(in, plan)
	if err != nil {
		t.Fatal(err)
	}
	in.Form.Values[0] = "Biểu đồ"
	r, ok := s.OwnerMessageForWork(works[0].RequestID)
	if !ok {
		t.Fatal("missing receipt")
	}
	r.Input.Form.Values[0] = "mutated read"
	r, _ = s.OwnerMessageForWork(works[0].RequestID)
	if r.Input.Form.Values[0] != "Bảng" {
		t.Fatal("receipt aliases caller")
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if replay, dup, err := s.AcceptOwnerMessage(original, plan); err != nil || !dup || replay[0].RequestID != works[0].RequestID {
		t.Fatal("receipt replay lost", err)
	}
	if _, _, err := s.AcceptOwnerMessage(in, plan); !errors.Is(err, ErrConflict) {
		t.Fatal("changed fields replayed", err)
	}
}

func TestOwnerFormReceiptRejectsLegacyNonce(t *testing.T) {
	s, in, plan := ownerFormFixture(t, t.TempDir())
	defer s.Close()
	if _, _, err := s.Accept(Input{BotID: in.ThreadID, ClientNonce: in.ClientNonce, Text: in.Text}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AcceptOwnerMessage(in, plan); !errors.Is(err, ErrConflict) {
		t.Fatal("legacy work fabricated a typed receipt", err)
	}
}

func TestOwnerFormReceiptReplayRejectsInvalidMetadata(t *testing.T) {
	for name, mutate := range map[string]func(*record){
		"missing-metadata": func(r *record) { r.Message.Input.Form = nil },
		"invalid-field":    func(r *record) { r.Message.Input.Form.Values[0] = "forged option" },
		"extra-field":      func(r *record) { r.Message.Input.Form.Values = append(r.Message.Input.Form.Values, "extra") },
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			s, in, plan := ownerFormFixture(t, dir)
			if _, _, err := s.AcceptOwnerMessage(in, plan); err != nil {
				t.Fatal(err)
			}
			path := s.file.Name()
			s.Close()
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			start := bytes.LastIndexByte(data[:len(data)-1], '\n') + 1
			var r record
			if err = json.Unmarshal(data[start:], &r); err != nil {
				t.Fatal(err)
			}
			mutate(&r)
			r.Digest, err = digestRecord(r)
			if err != nil {
				t.Fatal(err)
			}
			line, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, append(append(data[:start], line...), '\n'), 0600); err != nil {
				t.Fatal(err)
			}
			reopened, err := Open(dir)
			if reopened != nil {
				reopened.Close()
			}
			if !errors.Is(err, ErrCorrupt) {
				t.Fatal("invalid metadata replayed", err)
			}
		})
	}
}

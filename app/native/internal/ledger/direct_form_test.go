package ledger

import (
	"strings"
	"testing"
)

func directFormFixture() *DirectForm {
	return &DirectForm{Title: "Thông tin", Fields: []DirectFormField{{Name: "format", Label: "Định dạng", Type: "select", Options: []string{"Bảng", "Biểu đồ"}}}}
}

func TestDirectFormJournalAndIsolation(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	w, _, err := s.Accept(Input{BotID: s.Snapshot().Bots[0].ID, ClientNonce: "direct-form-journal", Text: "Collect"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DirectStart(w.RequestID, w.Epoch, DirectRun{Runtime: "direct-http", Form: directFormFixture()}); err == nil {
		t.Fatal("form published at start")
	}
	w, err = s.DirectStart(w.RequestID, w.Epoch, DirectRun{Runtime: "direct-http"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DirectUpdate(w.RequestID, w.Epoch, func(d *DirectRun) { d.Form = directFormFixture() }); err == nil {
		t.Fatal("intermediate form published")
	}
	if _, err = s.DirectFinish(w.RequestID, w.Epoch, "failed", "fixture", func(d *DirectRun) { d.Form = directFormFixture() }); err == nil {
		t.Fatal("failed turn published form")
	}
	original := directFormFixture()
	finished, err := s.DirectFinish(w.RequestID, w.Epoch, "completed", "", func(d *DirectRun) {
		d.Form = original
		d.Approval = &DirectApproval{Title: "Draft", Body: "Body", Kind: "send", State: "pending"}
	})
	if err != nil {
		t.Fatal(err)
	}
	original.Fields[0].Options[0] = "input mutation"
	finished.Direct.Form.Fields[0].Options[0] = "return mutation"
	for _, work := range s.Snapshot().Work {
		work.Direct.Form.Fields[0].Options[0] = "snapshot mutation"
	}
	got, _ := s.Get(w.RequestID)
	if got.Direct.Form.Fields[0].Options[0] != "Bảng" {
		t.Fatal("form aliases caller memory")
	}
	if _, err = s.DirectDecide(w.RequestID, func(a *DirectApproval, d *DirectRun) error {
		a.State = "approved"
		d.Form.Fields[0].Options[0] = "changed"
		return nil
	}); err == nil {
		t.Fatal("approval edited immutable form")
	}
	if _, err = s.DirectDecide(w.RequestID, func(a *DirectApproval, _ *DirectRun) error { a.State = "declined"; return nil }); err != nil {
		t.Fatal("ordinary approval rejected", err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err = s.Get(w.RequestID)
	if err != nil || got.Status != "completed" || got.Direct.Form.Fields[0].Options[0] != "Bảng" || got.Direct.Form.Done {
		t.Fatal("journal form lost", err)
	}
}

func TestDirectFormRejectsInvalidCompletion(t *testing.T) {
	for name, mutate := range map[string]func(*DirectForm){
		"empty-title":      func(f *DirectForm) { f.Title = " " },
		"title-limit":      func(f *DirectForm) { f.Title = strings.Repeat("界", 81) },
		"invalid-utf8":     func(f *DirectForm) { f.Title = string([]byte{0xff}) },
		"no-fields":        func(f *DirectForm) { f.Fields = nil },
		"too-many-fields":  func(f *DirectForm) { f.Fields = make([]DirectFormField, 11) },
		"empty-name":       func(f *DirectForm) { f.Fields[0].Name = "" },
		"duplicate-name":   func(f *DirectForm) { f.Fields = append(f.Fields, f.Fields[0]) },
		"unknown-type":     func(f *DirectForm) { f.Fields[0].Type = "password" },
		"options-on-text":  func(f *DirectForm) { f.Fields[0].Type = "text" },
		"no-options":       func(f *DirectForm) { f.Fields[0].Options = nil },
		"duplicate-option": func(f *DirectForm) { f.Fields[0].Options = []string{"Bảng", "Bảng"} },
		"option-limit":     func(f *DirectForm) { f.Fields[0].Options = []string{strings.Repeat("a", 61)} },
		"self-completed":   func(f *DirectForm) { f.Done = true },
	} {
		t.Run(name, func(t *testing.T) {
			s, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			w, _, err := s.Accept(Input{BotID: s.Snapshot().Bots[0].ID, ClientNonce: "invalid-form-" + name, Text: "Collect"})
			if err != nil {
				t.Fatal(err)
			}
			w, err = s.DirectStart(w.RequestID, w.Epoch, DirectRun{Runtime: "direct-http"})
			if err != nil {
				t.Fatal(err)
			}
			f := directFormFixture()
			mutate(f)
			if _, err = s.DirectFinish(w.RequestID, w.Epoch, "completed", "", func(d *DirectRun) { d.Form = f }); err == nil {
				t.Fatal("invalid schema committed")
			}
			got, _ := s.Get(w.RequestID)
			if got.Status != "running" || got.Direct.Form != nil {
				t.Fatal("rejected completion changed state")
			}
		})
	}
}

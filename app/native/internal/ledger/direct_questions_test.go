package ledger

import (
	"errors"
	"testing"
)

func TestStructuredQuestionsPersistAndClone(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s != nil {
			s.Close()
		}
	})
	w, _, err := s.Accept(input("question-schema-fixture", "foreground"))
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.DirectStart(w.RequestID, w.Epoch, DirectRun{Runtime: "direct-http"})
	if err != nil {
		t.Fatal(err)
	}
	q := []DirectQuestion{{ID: "q1", Prompt: "When?", Options: []string{"Morning", "Evening"}}, {ID: "q2", Prompt: "Outputs?", Options: []string{"Table", "Chart"}, Multiple: true}}
	before := s.Snapshot().Sequence
	for _, bad := range [][]DirectQuestion{
		{{ID: "duplicate", Prompt: "When?", Options: []string{"a", "b"}}},
		{{ID: "q1", Prompt: "When?", Options: []string{"same", "same"}}},
		{{ID: "q1", Prompt: "When?", Options: []string{"only one"}}},
	} {
		if _, err := s.DirectFinish(w.RequestID, w.Epoch, "completed", "", func(d *DirectRun) { d.Questions = bad }); !errors.Is(err, ErrConflict) {
			t.Fatal("invalid schema", err)
		}
	}
	if s.Snapshot().Sequence != before {
		t.Fatal("invalid card mutated journal")
	}
	w, err = s.DirectFinish(w.RequestID, w.Epoch, "completed", "", func(d *DirectRun) { d.Questions = q })
	if err != nil {
		t.Fatal(err)
	}
	w.Direct.Questions[0].Options[0] = "mutated"
	q[1].Options[0] = "mutated"
	current, _ := s.Get(w.RequestID)
	if current.Direct.Questions[0].Options[0] != "Morning" || current.Direct.Questions[1].Options[0] != "Table" {
		t.Fatal("shared question slice")
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	current, err = s.Get(w.RequestID)
	if err != nil || len(current.Direct.Questions) != 2 || !current.Direct.Questions[1].Multiple {
		t.Fatal("card not durable", err)
	}
}

package ledger

import (
	"errors"
	"testing"
)

func TestReportIntentTokenAndNegativeDirective(t *testing.T) {
	s, c, _ := reportFixture(t, false)
	w, _, err := s.Accept(Input{BotID: "bot-meiiie", ClientNonce: "grammar-amber-source", Text: "Write Report Amber", Lane: "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	w = startPermissionWork(t, s, w)
	literal := "Report Amber. Instruction text: do not share; không gửi. These are literal report words."
	if err := s.AppendText(w.RequestID, w.Epoch, literal); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	for index, text := range []string{
		"send reporter from private chat here",
		"send report; do not share from private chat here",
		"gui bao caox o hoi thoai rieng vao kenh nay",
		"send report do not share from private chat here",
		"gui bao cao khong gui o hoi thoai rieng vao kenh nay",
	} {
		t.Run(text, func(t *testing.T) {
			in := reportSubmission(c, "grammar-negative-"+string(rune('a'+index)))
			in.Text = text
			before := s.Snapshot().Sequence
			_, _, err := s.DeliverReport(in)
			if !errors.Is(err, ErrInvalid) || s.Snapshot().Sequence != before {
				t.Fatalf("non-command or contradictory request mutated publication: %v", err)
			}
		})
	}
	in := reportSubmission(c, "grammar-literal-positive")
	in.Text = "send report Amber from private chat here"
	delivery, duplicate, err := s.DeliverReport(in)
	if err != nil || duplicate || delivery.Status != "delivered" || delivery.Reply != literal || delivery.Source == nil || delivery.Source.RequestID != w.RequestID {
		t.Fatal("explicit report command did not preserve literal result", err)
	}
}

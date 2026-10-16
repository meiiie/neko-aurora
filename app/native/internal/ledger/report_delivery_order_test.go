package ledger

import "testing"

func TestReportDeliveryLatestUsesCompletionOrderNotIntakeOrAccess(t *testing.T) {
	s, c, _ := reportFixture(t, false)
	start := func(nonce string) Work {
		w, _, err := s.Accept(Input{BotID: "bot-meiiie", ClientNonce: nonce, Text: "Write a report", Lane: "foreground"})
		if err != nil {
			t.Fatal(err)
		}
		return startPermissionWork(t, s, w)
	}
	first := start("completion-first-intake")
	second := start("completion-second-intake")
	for _, w := range []Work{second, first} {
		if err := s.AppendText(w.RequestID, w.Epoch, "Report result "+w.RequestID); err != nil {
			t.Fatal(err)
		}
		if err := s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
			t.Fatal(err)
		}
	}
	assertLatest := func(nonce string) {
		t.Helper()
		in := reportSubmission(c, nonce)
		in.Text = "send report latest from private chat here"
		delivery, _, err := s.DeliverReport(in)
		if err != nil || delivery.Status != "delivered" || delivery.Source == nil || delivery.Source.RequestID != first.RequestID {
			t.Fatal("latest completed report selected by intake or access mutation", err)
		}
	}
	assertLatest("completion-latest-before-access")
	if _, _, err := s.SetSourceAccess(AccessInput{RequestID: second.RequestID, ExpectedRevision: 0, Granted: false, ClientNonce: "completion-second-revoke"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SetSourceAccess(AccessInput{RequestID: second.RequestID, ExpectedRevision: 1, Granted: true, ClientNonce: "completion-second-regrant"}); err != nil {
		t.Fatal(err)
	}
	assertLatest("completion-latest-after-access")
}

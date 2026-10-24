package ledger

import "testing"

func TestDirectRunJournalReplays(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	bot := s.Snapshot().Bots[0].ID
	w, _, err := s.Accept(Input{BotID: bot, ClientNonce: "direct-12345678", Text: "soạn thư"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DirectStart(w.RequestID, w.Epoch, DirectRun{Runtime: "neko-core-cli", Model: "glm-5.3", Approval: &DirectApproval{Title: "x", Body: "y", State: "approved"}}); err == nil {
		t.Fatal("a live run must not start with a decided approval")
	}
	w, err = s.DirectStart(w.RequestID, w.Epoch, DirectRun{Runtime: "neko-core-cli", Model: "glm-5.3"})
	if err != nil || w.Status != "running" || w.Core != nil {
		t.Fatalf("%v %+v", err, w)
	}
	if _, err := s.DirectStart(w.RequestID, w.Epoch, DirectRun{Runtime: "neko-core-cli"}); err == nil {
		t.Fatal("double start")
	}
	if err := s.AppendText(w.RequestID, w.Epoch, "Đã soạn."); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DirectFinish(w.RequestID, w.Epoch, "completed", "", func(d *DirectRun) {
		d.Approval = &DirectApproval{Title: "Gửi", Kind: "email", To: "a@b.co", Body: "Nội dung", State: "approved"}
	}); err == nil {
		t.Fatal("model output must not self-approve")
	}
	if _, err := s.DirectFinish(w.RequestID, w.Epoch, "completed", "", func(d *DirectRun) {
		d.Approval = &DirectApproval{Title: "Gửi", Kind: "email", To: "a@b.co", Body: "Nội dung", State: "pending"}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DirectDecide(w.RequestID, func(a *DirectApproval, _ *DirectRun) error { a.To = "evil@x.co"; a.State = "approved"; return nil }); err == nil {
		t.Fatal("recipient change accepted")
	}
	if _, err := s.DirectDecide(w.RequestID, func(a *DirectApproval, _ *DirectRun) error { a.State, a.DecidedAt = "approved", "now"; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DirectDecide(w.RequestID, func(a *DirectApproval, _ *DirectRun) error { a.State = "declined"; return nil }); err == nil {
		t.Fatal("decision flipped")
	}
	s.Close()
	s2, err := Open(dir)
	if err != nil {
		t.Fatal("journal with direct events must replay: ", err)
	}
	defer s2.Close()
	got, _ := s2.Get(w.RequestID)
	if got.Status != "completed" || got.Reply != "Đã soạn." || got.Direct.Approval.State != "approved" {
		t.Fatalf("%+v", got)
	}
}

func TestDirectStopViaCancel(t *testing.T) {
	s, _ := Open(t.TempDir())
	defer s.Close()
	bot := s.Snapshot().Bots[0].ID
	w, _, _ := s.Accept(Input{BotID: bot, ClientNonce: "direct-stop-1234", Text: "x"})
	w, _ = s.DirectStart(w.RequestID, w.Epoch, DirectRun{Runtime: "direct-http"})
	if c, err := s.Cancel(w.RequestID); err != nil || c.Status != "cancelling" {
		t.Fatalf("%v %+v", err, c)
	}
	if c, err := s.DirectStopped(w.RequestID); err != nil || c.Status != "cancelled" {
		t.Fatalf("%v %+v", err, c)
	}
}

func TestDirectResumeReplaysAndIsBounded(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	bot := s.Snapshot().Bots[0].ID
	w, _, _ := s.Accept(Input{BotID: bot, ClientNonce: "direct-resume-1", Text: "việc dài"})
	for i := 0; i < MaxDirectResumes; i++ {
		w, _ = s.Get(w.RequestID)
		w, err := s.DirectStart(w.RequestID, w.Epoch, DirectRun{Runtime: "direct-http", LeaseUntil: "2026-10-23T00:00:00Z"})
		if err != nil {
			t.Fatal(err)
		}
		_ = s.AppendText(w.RequestID, w.Epoch, "nửa chừng")
		if w, err = s.DirectResume(w.RequestID); err != nil || w.Status != "waiting_host" || w.Direct != nil || w.Reply != "" || w.DirectResumes != i+1 {
			t.Fatalf("resume %d: %v %+v", i, err, w)
		}
	}
	w, _ = s.Get(w.RequestID)
	w, _ = s.DirectStart(w.RequestID, w.Epoch, DirectRun{Runtime: "direct-http"})
	if _, err := s.DirectResume(w.RequestID); err == nil {
		t.Fatal("resume must be bounded")
	}
	s.Close()
	s2, err := Open(dir)
	if err != nil {
		t.Fatal("journal with direct_resumed must replay: ", err)
	}
	defer s2.Close()
	if got, _ := s2.Get(w.RequestID); got.Status != "running" || got.DirectResumes != MaxDirectResumes {
		t.Fatalf("%+v", got)
	}
	// a finished run is never resumed
	_, _ = s2.DirectFinish(w.RequestID, w.Epoch, "completed", "", nil)
	if _, err := s2.DirectResume(w.RequestID); err == nil {
		t.Fatal("finished run resumed")
	}
}

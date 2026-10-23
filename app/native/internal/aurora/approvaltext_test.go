package aurora

import (
	"testing"

	"nekoaurora.local/native/internal/ledger"
)

func TestComputerApprovalTextAfterDecision(t *testing.T) {
	for st, want := range map[string]string{"pending": computerAskText, "approved": "Đã chạy lệnh trên máy tính của mình.", "declined": "Bạn đã từ chối lệnh này, mình không chạy."} {
		w := ledger.Work{RequestID: "r1", BotID: "b", Reply: computerAskText, Direct: &ledger.DirectRun{Approval: &ledger.DirectApproval{Kind: "computer", State: st}}}
		var got string
		for _, m := range workMessages(w, false) {
			if m.Approval != nil {
				got = m.Text
			}
		}
		if got != want {
			t.Errorf("%s: got %q want %q", st, got, want)
		}
	}
}

func TestIsEnglishAndPromise(t *testing.T) {
	if !isEnglish("What is the latest stable Python release and when was it released?") || isEnglish("Python bản mới nhất là gì?") || isEnglish("ok") {
		t.Fatal("isEnglish")
	}
	if !promiseOnly(directives{Text: "Mình kiểm tra lại file trước khi báo nhé."}) {
		t.Fatal("promise not caught")
	}
}

func TestWelcomeTurnDoesNotAbsorbFirstRequest(t *testing.T) {
	svc, st := newSvc(t)
	bot := firstBot(svc)
	w, _, _ := st.Accept(ledger.Input{BotID: bot, ClientNonce: "onboard-u1", Text: "Chào Aurora!"})
	st.Accept(ledger.Input{BotID: bot, ClientNonce: "user-req-1", Text: "chạy lệnh echo hi"})
	if n := len(svc.inbox(w)); n != 0 {
		t.Fatalf("welcome absorbed %d messages", n)
	}
}

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

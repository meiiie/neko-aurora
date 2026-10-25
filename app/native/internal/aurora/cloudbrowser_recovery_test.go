package aurora

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"nekoaurora.local/native/internal/cloudbrowser"
	"nekoaurora.local/native/internal/ledger"
)

func TestCloudInterruptedApprovalRecoversUnknownWithoutRedispatch(t *testing.T) {
	t.Setenv("AURORA_COMPUTER", "off")
	for _, phase := range []string{"before-dispatch", "after-dispatch", "receipt-saved", "pending", "declined"} {
		t.Run(phase, func(t *testing.T) {
			dir := t.TempDir()
			s, store, closeService := networkManualService(t, dir)
			transport := &cloudTestTransport{}
			client, err := cloudbrowser.New(cloudbrowser.Config{URL: "https://cloud.example.test", Key: []byte(strings.Repeat("fixture!", 4)), StateDir: filepath.Join(dir, "cloud"), HTTP: &http.Client{Transport: transport}})
			if err != nil {
				t.Fatal(err)
			}
			s.Cloud = client
			work := networkRunningWork(t, store, "bot-davin", "conversation-bot-davin", "cloud-crash", "Click the observed button")
			tool := &toolCtx{}
			if _, err := s.runCloudTool(context.Background(), work, tool, toolCall{"cloud_browser_action", `{"name":"cloud_browser_action","kind":"click","epoch":7,"observationId":"fixture-observation","targetId":"fixture-target"}`}); err != nil {
				t.Fatal(err)
			}
			approval := s.parked(tool, "").Approval
			work, err = store.DirectFinish(work.RequestID, work.Epoch, "completed", "", func(run *ledger.DirectRun) { run.Approval = approval })
			if err != nil {
				t.Fatal(err)
			}
			decision := "approved"
			if phase == "pending" || phase == "declined" {
				decision = phase
			}
			if decision != "pending" {
				work, err = store.DirectDecide(work.RequestID, func(a *ledger.DirectApproval, _ *ledger.DirectRun) error { a.State = decision; return nil })
				if err != nil {
					t.Fatal(err)
				}
			}
			if phase == "after-dispatch" {
				proposal, err := s.cloudApprovedProposal(work, work.Direct.Approval)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := client.Do(context.Background(), proposal.Command); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "receipt-saved" {
				s.setDelivery(work.RequestID, ledger.DirectDelivery{Status: "sent", Detail: "dispatched"}, "Fixture receipt saved")
			}
			calls := len(transport.calls)
			closeService()
			if phase == "after-dispatch" {
				replayStore, err := ledger.Open(dir)
				if err != nil {
					t.Fatal(err)
				}
				(&Service{L: replayStore, Cloud: client}).recoverCloudApprovals()
				replayStore.Close()
			}
			_, reopened, closeReopened := networkManualService(t, dir)
			recovered, err := reopened.Get(work.RequestID)
			if err != nil {
				t.Fatal(err)
			}
			got := recovered.Direct.Approval
			switch phase {
			case "before-dispatch", "after-dispatch":
				if got.Delivery == nil || got.Delivery.Status != "failed" || got.Delivery.Detail != "unknown" || !strings.Contains(got.Outcome, "không tự") {
					t.Fatalf("interrupted approval did not recover honestly: %+v", got)
				}
			case "receipt-saved":
				if got.Delivery == nil || got.Delivery.Detail != "dispatched" || got.Outcome != "Fixture receipt saved" {
					t.Fatal("recovery overwrote a saved receipt")
				}
			default:
				if got.State != decision || got.Delivery != nil {
					t.Fatal("recovery changed an unapproved action")
				}
			}
			if len(transport.calls) != calls {
				t.Fatal("startup replayed a browser action")
			}
			closeReopened()
			_, second, _ := networkManualService(t, dir)
			again, err := second.Get(work.RequestID)
			if err != nil || (got.Delivery != nil && (again.Direct.Approval.Delivery == nil || *again.Direct.Approval.Delivery != *got.Delivery)) {
				t.Fatal("recovered outcome was not durable and idempotent")
			}
		})
	}
}

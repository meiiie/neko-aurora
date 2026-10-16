package ledger

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestPublishedReportStoreSnapshotMasksStaleDerivedManifestAndFrozenGroup(t *testing.T) {
	s, c, source := reportFixture(t, true)
	if _, _, err := s.DeliverReport(reportSubmission(c, "snapshot-published-delivery")); err != nil {
		t.Fatal(err)
	}
	s.ConfigureContextAdmission(true)
	completed := admitPublished(t, s, publishedWork(t, s, c, "snapshot-derived-completed"))
	if err := s.AppendText(completed.RequestID, completed.Epoch, "STALE-DERIVED-ONLY-SNAPSHOT-CANARY"); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(completed.RequestID, completed.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	completed, _ = s.Get(completed.RequestID)
	child := publishedWork(t, s, c, "snapshot-derived-child")
	manifest, _ := json.Marshal(child.RuntimeContext)
	if !bytes.Contains(manifest, []byte("STALE-DERIVED-ONLY-SNAPSHOT-CANARY")) {
		t.Fatal("regression fixture lacks derived-only manifest content")
	}
	g, _, err := s.CreateCollaboration(CollaborationInput{ChannelID: c.ID, ExpectedChannelRevision: c.Revision, Participants: []string{"bot-meiiie", "bot-davin"}, CoordinatorBotID: "bot-meiiie", Goal: "Compare the published report", MaxStages: 3, ClientNonce: "snapshot-frozen-group"})
	if err != nil {
		t.Fatal(err)
	}
	if g.PublishedReportContext == nil || len(g.PublishedReportContext.Pins) != 1 {
		t.Fatal("regression fixture lacks frozen publication")
	}
	active := s.Snapshot()
	for _, w := range active.Work {
		if w.RequestID == completed.RequestID && !reflect.DeepEqual(w.Core, completed.Core) {
			t.Fatal("active execution binding changed by snapshot")
		}
	}
	if _, _, err = s.SetSourceAccess(AccessInput{RequestID: source.RequestID, Granted: false, ClientNonce: "snapshot-source-revoke"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.file.Name())
	snapshot := s.Snapshot()
	for _, w := range snapshot.Work {
		if w.RequestID != completed.RequestID && w.RequestID != child.RequestID {
			continue
		}
		if w.RuntimeContext != nil {
			t.Fatal("stale derived RuntimeContext remains in Store.Snapshot")
		}
		if w.Reply != "" || w.Context != nil || w.Core != nil {
			t.Fatal("stale derived payload remains in Store.Snapshot")
		}
	}
	for _, group := range snapshot.Collaborations {
		if group.ID == g.ID && group.PublishedReportContext != nil {
			t.Fatal("frozen publication remains in Store.Snapshot collaboration")
		}
	}
	projected, _ := json.Marshal(snapshot)
	if bytes.Contains(projected, []byte("STALE-DERIVED-ONLY-SNAPSHOT-CANARY")) {
		t.Fatal("derived-only content escaped snapshot mask")
	}
	internal, _ := s.Get(child.RequestID)
	if internal.RuntimeContext == nil || internal.Context == nil {
		t.Fatal("snapshot masking changed internal Get")
	}
	if stored := s.collaborations[g.ID]; stored.PublishedReportContext == nil {
		t.Fatal("snapshot masking changed internal frozen group")
	}
	after, _ := os.ReadFile(s.file.Name())
	if !bytes.Equal(before, after) {
		t.Fatal("snapshot projection mutated journal")
	}
}

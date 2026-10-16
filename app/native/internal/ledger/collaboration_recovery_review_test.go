package ledger

import (
	"errors"
	"nekobot.local/native/internal/connections"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func groupRecoveryAccountFixture(t *testing.T) (*Store, Collaboration, Work) {
	t.Helper()
	s, _, g, w, _ := groupRecoveryFixture(t, 2, func(s *Store, in *CollaborationInput) {
		catalog, err := connections.Open(filepath.Join(t.TempDir(), "accounts"), authorityVault{})
		if err != nil {
			t.Fatal(err)
		}
		a, _, err := catalog.SaveAccount(connections.AccountInput{Name: "Review fixture", Provider: "openai-compatible", Endpoint: "https://example.invalid/v1", Model: "fixture-model", Secret: "fixture-only-review-account", ClientNonce: "group-review-account-create"})
		if err != nil {
			t.Fatal(err)
		}
		s.EnableAccounts(catalog)
		in.AccountID, in.AccountRevision = a.ID, a.Revision
	})
	return s, g, w
}

func TestGroupServiceRecoveryAccountRecordedTimeBoundaries(t *testing.T) {
	s, g, w := groupRecoveryAccountFixture(t)
	at := mustIssued(w.AccountLease)
	if !groupBudgetAt(g, at) || !collaborationRecoveryAccountAt(w, at) {
		t.Fatal("inclusive issue boundary")
	}
	for _, kind := range []string{"pre-issue", "expiry", "post-expiry", "malformed-issue", "malformed-expiry", "empty-window", "missing-lease", "expired-marker"} {
		t.Run(kind, func(t *testing.T) {
			x := copyWork(w)
			when := at
			switch kind {
			case "pre-issue":
				when = at.Add(-time.Nanosecond)
			case "expiry":
				when = mustExpiry(w.AccountLease)
			case "post-expiry":
				when = mustExpiry(w.AccountLease).Add(time.Nanosecond)
			case "malformed-issue":
				x.AccountLease.IssuedAt = "invalid"
			case "malformed-expiry":
				x.AccountLease.ExpiresAt = "invalid"
			case "empty-window":
				x.AccountLease.ExpiresAt = x.AccountLease.IssuedAt
			case "missing-lease":
				x.AccountLease = nil
			case "expired-marker":
				x.AccountExpiredAt = mustExpiry(w.AccountLease).Format(time.RFC3339Nano)
			}
			if collaborationRecoveryAccountAt(x, when) {
				t.Fatal("invalid recorded account time admitted")
			}
		})
	}
	in := groupRecoverInput(g, w, "group-review-recorded-time")
	out, _, err := s.RecoverCollaboration(g.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	event := s.events[len(s.events)-1]
	s.collaborations[g.ID], s.work[w.RequestID] = g, w
	delete(s.collaborationActions, in.ClientNonce)
	s.now = func() time.Time { return mustExpiry(w.AccountLease).Add(time.Hour) }
	s.accounts = nil
	if !validReplay(s, event) {
		t.Fatal("replay consulted current clock or catalog")
	}
	if !reflect.DeepEqual(event.Work, out.Work) {
		t.Fatal("saved recovery drift")
	}
}

func TestGroupServiceRecoveryRecordedAccountExpiryCannotRewind(t *testing.T) {
	s, g, w := groupRecoveryAccountFixture(t)
	if err := s.expireAccount(w, mustExpiry(w.AccountLease)); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Get(w.RequestID)
	when := mustIssued(w.AccountLease).Add(time.Nanosecond)
	s.now = func() time.Time { return when }
	in := groupRecoverInput(g, current, "group-review-rewound-expiry")
	before := s.Snapshot().Sequence
	if _, _, err := s.RecoverCollaboration(g.ID, in); !errors.Is(err, ErrConflict) || s.Snapshot().Sequence != before {
		t.Fatal("live expiry marker bypassed by rewind", err)
	}
	recovered := copyCollaboration(g)
	recovered.Status, recovered.Reason = "running", ""
	recovered.Revision++
	work := copyWork(current)
	work.Status, work.DesiredState, work.Failure = "waiting_host", "running", ""
	work.Epoch++
	work.TaskRevision++
	event := Event{Type: "collaboration.recovered", Nonce: in.ClientNonce, Collaboration: &recovered, Work: work, CollaborationRecovery: &CollaborationRecoveryEvent{Input: in, IssuedAt: when.UTC().Format(time.RFC3339Nano)}}
	if validReplay(s, event) {
		t.Fatal("replay rewound irreversible account expiry")
	}
	dir := s.dir
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		re, err := Open(dir)
		if err != nil {
			t.Fatal("genuine expiry journal replay", err)
		}
		got, _ := re.Get(w.RequestID)
		if got.AccountExpiredAt != current.AccountExpiredAt || got.AccountExpiredAt == "" {
			t.Fatal("irreversible marker lost")
		}
		if validReplay(re, event) {
			t.Fatal("restarted replay rewound expiry")
		}
		if err := re.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGroupServiceRecoveryReceiptMissKindMatrixAndRestart(t *testing.T) {
	s, _, g, w, _ := groupRecoveryFixture(t, 1)
	in := groupRecoverInput(g, w, "group-review-recover-receipt")
	out, _, err := s.RecoverCollaboration(g.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	cancel := CollaborationCancelInput{ExpectedRevision: out.Collaboration.Revision, ClientNonce: "group-review-cancel-receipt"}
	if _, _, err = s.CancelCollaboration(g.ID, cancel); err != nil {
		t.Fatal(err)
	}
	other, _, err := s.Accept(input("group-review-task-receipt", "foreground"))
	if err != nil {
		t.Fatal(err)
	}
	control := controlInput(other, "pause", "group-review-control-receipt")
	if _, _, err = s.ControlTask(control); err != nil {
		t.Fatal(err)
	}
	commands := []struct{ kind, nonce string }{{"collaboration_create", g.ClientNonce}, {"collaboration_recover", in.ClientNonce}, {"collaboration_cancel", cancel.ClientNonce}, {"task_control", control.ClientNonce}}
	for restart := 0; restart < 3; restart++ {
		before := s.Snapshot().Sequence
		for _, known := range commands {
			for _, query := range commands {
				id := ""
				if query.kind == "collaboration_cancel" || query.kind == "collaboration_recover" {
					id = g.ID
				}
				r, e := s.CommandReceipt(query.kind, known.nonce, id)
				if known.kind == query.kind {
					if e != nil || !r.Found {
						t.Fatal("successful original receipt lost", known.kind, e)
					}
				} else if !errors.Is(e, ErrConflict) {
					t.Fatal("wrong-kind known nonce represented as missing", known.kind, query.kind, e)
				}
			}
		}
		for _, query := range commands {
			id := ""
			if query.kind == "collaboration_cancel" || query.kind == "collaboration_recover" {
				id = g.ID
			}
			r, e := s.CommandReceipt(query.kind, "group-review-never-posted", id)
			if e != nil || r.Found {
				t.Fatal("unknown nonce invented", query.kind, e)
			}
		}
		if s.Snapshot().Sequence != before {
			t.Fatal("read-only receipt changed journal")
		}
		dir := s.dir
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		s, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
	}
	defer s.Close()
}

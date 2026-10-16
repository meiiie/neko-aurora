package ledger

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"nekobot.local/native/internal/syncstream"
)

func prefixCursor(t *testing.T, e syncstream.Envelope, sequence uint64) string {
	t.Helper()
	token, err := (syncstream.Cursor{Version: 1, InstanceID: e.InstanceID, ScopeID: e.ScopeID, Sequence: sequence}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func prefixStore(t *testing.T) (*Store, LocalSyncScope, syncstream.Envelope) {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	scope := LocalSyncScope{BotID: "bot-meiiie"}
	initial, err := s.LocalSync(scope, "")
	if err != nil {
		t.Fatal(err)
	}
	return s, scope, initial
}

func prefixClient(t *testing.T, initial syncstream.Envelope) *syncstream.Client {
	t.Helper()
	c, err := syncstream.NewClient(initial.InstanceID, initial.ScopeID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Apply(initial); err != nil {
		t.Fatal(err)
	}
	return c
}

func prefixDrain(t *testing.T, s *Store, scope LocalSyncScope, initial syncstream.Envelope, c *syncstream.Client) (int, bool) {
	t.Helper()
	highWater := uint64(len(s.events))
	pages, sawEmpty := 0, false
	for c.Cursor().Sequence < highWater {
		after := c.Cursor().Sequence
		e, err := s.LocalSync(scope, prefixCursor(t, initial, after))
		if err != nil {
			t.Fatalf("representable prefix after %d: %v", after, err)
		}
		if e.After != after || e.To <= after || e.HighWater != highWater || e.InstanceID != initial.InstanceID || e.ScopeID != initial.ScopeID || len(e.Frames) > syncstream.MaxFrames {
			t.Fatal("page lost progress, capture or binding")
		}
		if _, err = syncstream.Encode(e); err != nil {
			t.Fatal("page exceeds a protocol bound", err)
		}
		for i, f := range e.Frames {
			if f.Sequence != after+uint64(i)+1 {
				t.Fatal("filtered sequence was skipped")
			}
			sawEmpty = sawEmpty || len(f.Updates) == 0
		}
		if e.To < highWater && len(e.Frames) < syncstream.MaxFrames {
			next, err := s.LocalSync(scope, prefixCursor(t, initial, e.To))
			if err != nil || len(next.Frames) == 0 {
				t.Fatal("next representable page missing", err)
			}
			candidate := append(append([]syncstream.Frame{}, e.Frames...), next.Frames[0])
			if _, err = syncstream.NewIncrement(e.InstanceID, e.ScopeID, after, highWater, candidate); !errors.Is(err, syncstream.ErrLimit) {
				t.Fatal("page stopped before the largest valid prefix", err)
			}
		}
		if _, err = c.Apply(e); err != nil {
			t.Fatal("client could not reconcile page", err)
		}
		if outcome, err := c.Apply(e); err != nil || outcome != syncstream.Ignored {
			t.Fatal("duplicate page changed replica", err)
		}
		pages++
		if pages > int(highWater)+1 {
			t.Fatal("pager failed to converge")
		}
	}
	fresh, err := s.LocalSync(scope, "")
	if err != nil || !reflect.DeepEqual(c.Objects(), fresh.Snapshot) {
		t.Fatal("paged client differs from complete snapshot", err)
	}
	return pages, sawEmpty
}

func TestLocalSyncPrefixEncodedBudgetConvergesAndRestarts(t *testing.T) {
	s, scope, initial := prefixStore(t)
	in := input("prefix-byte-intake", "background")
	in.Text = strings.Repeat("<&", 1024)
	w, _, err := s.Accept(in)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 120; i++ {
		action := "pause"
		if i%2 == 1 {
			action = "resume"
		}
		r, _, err := s.ControlTask(controlInput(w, action, fmt.Sprintf("prefix-byte-control-%03d", i)))
		if err != nil {
			t.Fatal(err)
		}
		w = r.Work
		if i%7 == 0 {
			other := input(fmt.Sprintf("prefix-excluded-%03d", i), "background")
			other.BotID = "bot-davin"
			other.Text = "excluded-private-canary"
			if _, _, err = s.Accept(other); err != nil {
				t.Fatal(err)
			}
		}
	}
	journal, err := os.ReadFile(s.file.Name())
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(s.Snapshot())
	c := prefixClient(t, initial)
	pages, sawEmpty := prefixDrain(t, s, scope, initial, c)
	if pages < 3 || !sawEmpty {
		t.Fatal("fixture did not exercise encoded pagination and filtered frames")
	}
	after, _ := json.Marshal(s.Snapshot())
	raw, _ := os.ReadFile(s.file.Name())
	if !bytes.Equal(before, after) || sha256.Sum256(raw) != sha256.Sum256(journal) {
		t.Fatal("sync reads mutated journal or work")
	}
	dir := s.dir
	s.Close()
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	pages, _ = prefixDrain(t, reopened, scope, initial, prefixClient(t, initial))
	if pages < 3 {
		t.Fatal("restart lost the bounded replay history")
	}
	final, err := reopened.LocalSync(scope, prefixCursor(t, initial, c.Cursor().Sequence))
	if err != nil || len(final.Frames) != 0 || final.To != final.HighWater {
		t.Fatal("completed cursor changed after restart", err)
	}
}

func prefixProjectionWork(t *testing.T, s *Store) Work {
	t.Helper()
	w, _, err := s.Accept(input("prefix-projection-seed", "background"))
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func prefixArtifacts(count int) []Artifact {
	artifacts := make([]Artifact, count)
	for i := range artifacts {
		artifacts[i] = Artifact{ID: fmt.Sprintf("%064x", i+1), Filename: fmt.Sprintf("fixture-%03d.txt", i), SHA256: strings.Repeat("a", 64), Bytes: 1}
	}
	return artifacts
}

func TestLocalSyncPrefixUpdateBudgetProjection(t *testing.T) {
	s, scope, initial := prefixStore(t)
	w := prefixProjectionWork(t, s)
	w.Artifacts = prefixArtifacts(6)
	s.events = nil
	for sequence := uint64(1); sequence <= 128; sequence++ {
		s.events = append(s.events, Event{Sequence: sequence, Type: "work.artifact", Work: w})
	}
	s.work[w.RequestID] = copyWork(w)
	before, _ := json.Marshal(s.events)
	c := prefixClient(t, initial)
	first, err := s.LocalSync(scope, prefixCursor(t, initial, 0))
	if err != nil {
		t.Fatal("512-update prefix missing", err)
	}
	if first.To != 64 || len(first.Frames) != 64 {
		t.Fatal("512-update budget did not choose exactly 64 eight-update frames")
	}
	pages, _ := prefixDrain(t, s, scope, initial, c)
	after, _ := json.Marshal(s.events)
	if pages != 2 || !bytes.Equal(before, after) {
		t.Fatal("synthetic update projection did not converge without mutation")
	}
}

func prefixOversizeWork(w Work, kind string) Work {
	switch kind {
	case "text":
		w.Text = strings.Repeat("x", (12<<10)+1)
	case "reply":
		w.Reply = strings.Repeat("x", (12<<10)+1)
	case "encoded_object":
		w.Text = strings.Repeat("<&", 2048)
	case "updates":
		w.Artifacts = prefixArtifacts(syncstream.MaxObjects)
	}
	return w
}

func TestLocalSyncPrefixFirstUnrepresentableFails(t *testing.T) {
	for _, kind := range []string{"text", "reply", "encoded_object", "updates"} {
		t.Run(kind, func(t *testing.T) {
			s, scope, initial := prefixStore(t)
			w := prefixOversizeWork(prefixProjectionWork(t, s), kind)
			s.events = []Event{{Sequence: 1, Type: "work.artifact", Work: w}}
			if _, err := s.LocalSync(scope, prefixCursor(t, initial, 0)); !errors.Is(err, syncstream.ErrLimit) {
				t.Fatal("first frame advanced or silently truncated", err)
			}
			if _, err := s.LocalSync(scope, ""); !errors.Is(err, syncstream.ErrLimit) {
				t.Fatal("complete snapshot quota changed", err)
			}
		})
	}
}

func TestLocalSyncPrefixLaterUnrepresentablePreservesEarlierFrames(t *testing.T) {
	for _, kind := range []string{"text", "reply", "encoded_object", "updates"} {
		for _, at := range []int{2, 130} {
			t.Run(fmt.Sprintf("%s_at_%d", kind, at), func(t *testing.T) {
				s, scope, initial := prefixStore(t)
				w := prefixProjectionWork(t, s)
				s.events = nil
				for i := 1; i < at; i++ {
					s.events = append(s.events, Event{Sequence: uint64(i), Type: "work.control", Work: w})
				}
				s.events = append(s.events, Event{Sequence: uint64(at), Type: "work.artifact", Work: prefixOversizeWork(w, kind)})
				c := prefixClient(t, initial)
				for c.Cursor().Sequence < uint64(at-1) {
					e, err := s.LocalSync(scope, prefixCursor(t, initial, c.Cursor().Sequence))
					if err != nil || e.To >= uint64(at) || e.To <= c.Cursor().Sequence || e.HighWater != uint64(at) {
						t.Fatal("later unrepresentable frame hid or skipped valid prefix", err)
					}
					if _, err = c.Apply(e); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := s.LocalSync(scope, prefixCursor(t, initial, c.Cursor().Sequence)); !errors.Is(err, syncstream.ErrLimit) {
					t.Fatal("unrepresentable next frame was skipped", err)
				}
				if _, err := s.LocalSync(scope, ""); !errors.Is(err, syncstream.ErrLimit) {
					t.Fatal("full-snapshot quota changed", err)
				}
			})
		}
	}
}

func TestLocalSyncPrefixAlreadyConsumedOversizeDoesNotBlockNewFrames(t *testing.T) {
	s, scope, initial := prefixStore(t)
	w := prefixProjectionWork(t, s)
	s.events = []Event{{Sequence: 1, Type: "work.control", Work: prefixOversizeWork(w, "reply")}, {Sequence: 2, Type: "work.control", Work: w}}
	e, err := s.LocalSync(scope, prefixCursor(t, initial, 1))
	if err != nil || e.After != 1 || e.To != 2 || len(e.Frames) != 1 || e.Frames[0].Sequence != 2 {
		t.Fatal("already consumed history blocked current page", err)
	}
}

func TestLocalSyncPrefixOversizePublishedReportStopsAtItsSequence(t *testing.T) {
	s, c, w := reportFixture(t, false)
	scope := LocalSyncScope{BotID: "bot-meiiie", ChannelID: c.ID}
	if _, _, err := s.RenameBot("bot-meiiie", BotNameInput{Name: "Prefix report fixture", ClientNonce: "prefix-report-rename"}); err != nil {
		t.Fatal(err)
	}
	d, _, err := s.DeliverReport(reportSubmission(c, "prefix-report-delivery"))
	if err != nil || !d.Active || d.Source == nil || d.Source.RequestID != w.RequestID {
		t.Fatal(err)
	}
	initial, err := s.LocalSync(scope, "")
	if err != nil {
		t.Fatal(err)
	}
	last := len(s.events) - 1
	d.Reply = strings.Repeat("r", (12<<10)+1)
	s.events[last].ReportDelivery = &d
	fresh, err := s.LocalSync(scope, prefixCursor(t, initial, initial.To-2))
	if err != nil || fresh.To != uint64(last) || len(fresh.Frames) != 1 || fresh.HighWater != uint64(last+1) {
		t.Fatal("oversize published report hid earlier frame", err)
	}
	if _, err = s.LocalSync(scope, prefixCursor(t, initial, fresh.To)); !errors.Is(err, syncstream.ErrLimit) {
		t.Fatal("oversize report skipped", err)
	}
	if _, err = s.LocalSync(scope, ""); !errors.Is(err, syncstream.ErrLimit) {
		t.Fatal("snapshot silently truncated published report", err)
	}
}

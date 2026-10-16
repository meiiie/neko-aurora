package syncstream

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func botObject(revision uint64, name string) Object {
	return Object{Kind: "bot", ID: "bot-a", Revision: revision, Bot: &Bot{Name: name, Alias: "neko"}}
}

func TestTwoClientsConcurrentDuplicateSchedules(t *testing.T) {
	clients := []*Client{newReady(t), newReady(t)}
	page := mustIncrement(t, 0, 2,
		Frame{Sequence: 1, Updates: []Object{messageObject("message-a", "user", "hello")}},
		Frame{Sequence: 2, Updates: []Object{botObject(2, "Renamed")}})
	var workers sync.WaitGroup
	errorsCh := make(chan error, 32)
	for _, client := range clients {
		for i := 0; i < 16; i++ {
			workers.Add(1)
			go func(c *Client) { defer workers.Done(); _, err := c.Apply(page); errorsCh <- err }(client)
		}
	}
	workers.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(clients[0].Objects(), clients[1].Objects()) {
		t.Fatal("concurrent clients diverged")
	}
	for _, c := range clients {
		if c.Cursor().Sequence != 2 || c.NeedsSnapshot() {
			t.Fatal("duplicate schedule corrupted cursor")
		}
	}
}

func TestClientReplayOverlapConflictAndUnprovenHistory(t *testing.T) {
	first := mustIncrement(t, 0, 3,
		Frame{Sequence: 1, Updates: []Object{messageObject("message-a", "user", "hello")}},
		Frame{Sequence: 2, Updates: []Object{botObject(2, "Renamed")}})
	c := newReady(t)
	if _, err := c.Apply(first); err != nil {
		t.Fatal(err)
	}
	overlap := mustIncrement(t, 1, 3, first.Frames[1], Frame{Sequence: 3, Updates: []Object{fileObject()}})
	if outcome, err := c.Apply(overlap); err != nil || outcome != Applied {
		t.Fatalf("consistent overlap: %s %v", outcome, err)
	}
	if outcome, err := c.Apply(first); err != nil || outcome != Ignored {
		t.Fatalf("old replay: %s %v", outcome, err)
	}
	if c.HighWater() != 3 || c.Cursor().Sequence != 3 {
		t.Fatal("old replay regressed counters")
	}
	before := c.Objects()
	altered := mustIncrement(t, 1, 3, Frame{Sequence: 2, Updates: []Object{botObject(2, "Different")}})
	if _, err := c.Apply(altered); !errors.Is(err, ErrConflict) {
		t.Fatalf("same cursor changed frame: %v", err)
	}
	if !reflect.DeepEqual(before, c.Objects()) || c.Cursor().Sequence != 3 {
		t.Fatal("conflicting replay mutated projection")
	}
	if _, err := c.Apply(mustSnapshot(t, 3, before...)); err != nil || c.NeedsSnapshot() {
		t.Fatalf("consistent recovery snapshot: %v", err)
	}
	fresh, _ := NewClient("journal-a", "scope-a")
	if _, err := fresh.Apply(mustSnapshot(t, 3, before...)); err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.Apply(first); !errors.Is(err, ErrResync) {
		t.Fatalf("unobserved history silently ignored: %v", err)
	}
}

func TestClientRevisionFloorsDeletionAtomicityAndCopies(t *testing.T) {
	c := newReady(t)
	first := mustIncrement(t, 0, 1, Frame{Sequence: 1, Updates: []Object{botObject(2, "Renamed"), messageObject("message-a", "user", "hello")}})
	if _, err := c.Apply(first); err != nil {
		t.Fatal(err)
	}
	before := c.Objects()
	badMessage := messageObject("message-a", "user", "changed at same revision")
	bad := mustIncrement(t, 1, 3, Frame{Sequence: 2, Updates: []Object{botObject(3, "Must not leak")}}, Frame{Sequence: 3, Updates: []Object{badMessage}})
	if _, err := c.Apply(bad); !errors.Is(err, ErrConflict) {
		t.Fatalf("entity conflict: %v", err)
	}
	if c.Cursor().Sequence != 1 || !reflect.DeepEqual(c.Objects(), before) {
		t.Fatal("partially applied rejected batch")
	}
	if _, err := c.Apply(mustSnapshot(t, 1, before...)); err != nil {
		t.Fatal(err)
	}
	deleteObject := Object{Kind: "message", ID: "message-a", Revision: 2, Deleted: true}
	if _, err := c.Apply(mustIncrement(t, 1, 2, Frame{Sequence: 2, Updates: []Object{deleteObject}})); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Apply(mustSnapshot(t, 3, botObject(2, "Renamed"))); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Apply(mustIncrement(t, 3, 4, Frame{Sequence: 4, Updates: []Object{messageObject("message-a", "user", "hello")}})); !errors.Is(err, ErrConflict) {
		t.Fatalf("deleted entity resurrected with older revision: %v", err)
	}
	if _, err := c.Apply(mustSnapshot(t, 3, botObject(2, "Renamed"))); err != nil {
		t.Fatal(err)
	}
	resurrected := messageObject("message-a", "user", "new explicit data")
	resurrected.Revision = 3
	last := mustIncrement(t, 3, 4, Frame{Sequence: 4, Updates: []Object{resurrected}})
	if _, err := c.Apply(last); err != nil {
		t.Fatal(err)
	}
	last.Frames[0].Updates[0].Message.Text = "mutated input"
	copy := c.Objects()
	copy[0].Bot.Name = "mutated output"
	if c.Objects()[0].Bot.Name != "Renamed" || c.Objects()[1].Message.Text != "new explicit data" {
		t.Fatal("client shared mutable DTO pointers")
	}
	if _, err := c.Apply(mustSnapshot(t, 5, botObject(1, "Regressed"), resurrected)); !errors.Is(err, ErrConflict) {
		t.Fatalf("new snapshot regressed revision: %v", err)
	}
}

func TestClientExplicitInstanceResetKeepsScopeAndRequiresSnapshot(t *testing.T) {
	c := newReady(t)
	if err := c.ResetInstance("journal-a"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("same instance reset: %v", err)
	}
	if err := c.ResetInstance("journal-b"); err != nil {
		t.Fatal(err)
	}
	if c.Cursor().InstanceID != "journal-b" || c.Cursor().ScopeID != "scope-a" || c.Cursor().Sequence != 0 || c.HighWater() != 0 || len(c.Objects()) != 0 || !c.NeedsSnapshot() {
		t.Fatal("explicit reset did not clear only the pinned instance state")
	}
	if _, err := c.Apply(mustSnapshot(t, 0, botObject(1, "Neko"))); !errors.Is(err, ErrInstance) {
		t.Fatalf("old instance accepted after reset: %v", err)
	}
	newInstance, err := NewSnapshot("journal-b", "scope-a", 0, []Object{botObject(1, "New journal")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Apply(newInstance); err != nil || c.NeedsSnapshot() {
		t.Fatalf("new instance snapshot: %v", err)
	}
}

func TestClientBoundedReplayHistoryAndOldSnapshotProof(t *testing.T) {
	c := newReady(t)
	old := mustSnapshot(t, 0, botObject(1, "Neko"))
	for sequence := uint64(1); sequence <= ReplayWindow+1; sequence++ {
		if _, err := c.Apply(mustIncrement(t, sequence-1, sequence, Frame{Sequence: sequence, Updates: []Object{}})); err != nil {
			t.Fatal(err)
		}
	}
	if len(c.frames) > ReplayWindow || len(c.anchors) > ReplayWindow {
		t.Fatal("replay history is unbounded")
	}
	if _, err := c.Apply(old); !errors.Is(err, ErrResync) {
		t.Fatalf("evicted snapshot silently ignored: %v", err)
	}
	if _, err := c.Apply(mustSnapshot(t, ReplayWindow+1, botObject(1, "Neko"))); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Apply(mustIncrement(t, 0, ReplayWindow+1, Frame{Sequence: 1, Updates: []Object{}})); !errors.Is(err, ErrResync) {
		t.Fatalf("evicted frame silently ignored: %v", err)
	}
	if _, err := c.Apply(mustSnapshot(t, ReplayWindow, botObject(1, "Altered"))); !errors.Is(err, ErrConflict) {
		t.Fatalf("known old snapshot changed: %v", err)
	}
}

func TestClientObjectAndStateBudgetsStayAtomic(t *testing.T) {
	objects := make([]Object, MaxObjects)
	for i := range objects {
		objects[i] = Object{Kind: "bot", ID: fmt.Sprintf("bot-%03d", i), Revision: 1, Bot: &Bot{Name: "Neko"}}
	}
	c, _ := NewClient("journal-a", "scope-a")
	if _, err := c.Apply(mustSnapshot(t, 0, objects...)); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Apply(mustIncrement(t, 0, 1, Frame{Sequence: 1, Updates: []Object{botObject(1, "Too many")}})); !errors.Is(err, ErrLimit) {
		t.Fatalf("object budget: %v", err)
	}
	if c.Cursor().Sequence != 0 || len(c.Objects()) != MaxObjects {
		t.Fatal("object budget failure changed state")
	}
	large := newReady(t)
	var sequence uint64
	failed := false
	for page := 0; page < 11; page++ {
		updates := make([]Object, 48)
		for i := range updates {
			updates[i] = messageObject(fmt.Sprintf("message-%03d", page*48+i), "user", strings.Repeat("x", 4<<10))
		}
		e := mustIncrement(t, sequence, sequence+1, Frame{Sequence: sequence + 1, Updates: updates})
		before := large.Objects()
		_, err := large.Apply(e)
		if errors.Is(err, ErrLimit) {
			if sequence == 0 || large.Cursor().Sequence != sequence || !reflect.DeepEqual(before, large.Objects()) {
				t.Fatal("state budget failure changed state")
			}
			failed = true
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		sequence++
	}
	if !failed {
		t.Fatal("client state budget was not enforced")
	}
}

func messageObject(id, role, text string) Object {
	return Object{Kind: "message", ID: id, Revision: 1, Message: &Message{BotID: "bot-a", ChannelID: "channel-a", Role: role, Text: text, Status: "completed"}}
}

func fileObject() Object {
	return Object{Kind: "file", ID: "file-a", Revision: 1, File: &File{Name: "note.txt", MIME: "text/plain", Size: 12, SHA256: digest([]byte("fixture-file")), BotID: "bot-a", ChannelID: "channel-a", ProjectID: "project-a", Status: "accepted", Version: 1, AccessRevision: 1}}
}

func mustSnapshot(t *testing.T, sequence uint64, objects ...Object) Envelope {
	t.Helper()
	e, err := NewSnapshot("journal-a", "scope-a", sequence, objects)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func mustIncrement(t *testing.T, after, highWater uint64, frames ...Frame) Envelope {
	t.Helper()
	e, err := NewIncrement("journal-a", "scope-a", after, highWater, frames)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func newReady(t *testing.T) *Client {
	t.Helper()
	c, err := NewClient("journal-a", "scope-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Apply(mustSnapshot(t, 0, botObject(1, "Neko"))); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestClientRequiresCompleteSnapshotAndPinnedBinding(t *testing.T) {
	c, err := NewClient("journal-a", "scope-a")
	if err != nil {
		t.Fatal(err)
	}
	if !c.NeedsSnapshot() {
		t.Fatal("new client does not require a snapshot")
	}
	if _, err = c.Apply(mustIncrement(t, 0, 1, Frame{Sequence: 1, Updates: []Object{botObject(2, "Renamed")}})); !errors.Is(err, ErrResync) {
		t.Fatalf("increment before snapshot: %v", err)
	}
	for _, tc := range []struct {
		name     string
		instance string
		scope    string
		want     error
	}{
		{"instance", "journal-b", "scope-a", ErrInstance},
		{"scope", "journal-a", "scope-b", ErrScope},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newReady(t)
			before := client.Cursor()
			wrong, e := NewSnapshot(tc.instance, tc.scope, 4, []Object{botObject(1, "Wrong")})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = client.Apply(wrong); !errors.Is(e, tc.want) {
				t.Fatalf("got %v; want %v", e, tc.want)
			}
			if client.Cursor() != before {
				t.Fatal("rejected envelope changed cursor or binding")
			}
		})
	}
}

func TestClientRejectsGapAndChangedSameCursorAtomically(t *testing.T) {
	c := newReady(t)
	initial := c.Objects()
	gap := mustIncrement(t, 1, 2, Frame{Sequence: 2, Updates: []Object{botObject(2, "Gap")}})
	if _, err := c.Apply(gap); !errors.Is(err, ErrGap) {
		t.Fatalf("gap: %v", err)
	}
	if c.Cursor().Sequence != 0 || !reflect.DeepEqual(initial, c.Objects()) || !c.NeedsSnapshot() {
		t.Fatal("gap changed projection or did not fence resync")
	}
	if _, err := c.Apply(mustSnapshot(t, 2, botObject(2, "Renamed"))); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Apply(mustSnapshot(t, 2, botObject(3, "Altered"))); !errors.Is(err, ErrConflict) {
		t.Fatalf("same cursor changed snapshot: %v", err)
	}
	if c.Cursor().Sequence != 2 || c.Objects()[0].Bot.Name != "Renamed" {
		t.Fatal("conflicting snapshot changed state")
	}
}

func TestTwoOfflineClientStateFixtures(t *testing.T) {
	initial := mustSnapshot(t, 0, botObject(1, "Neko"))
	first := mustIncrement(t, 0, 4,
		Frame{Sequence: 1, Updates: []Object{messageObject("message-a", "user", "Xin chào")}},
		Frame{Sequence: 2, Updates: []Object{botObject(2, "Neko renamed")}})
	second := mustIncrement(t, 2, 4,
		Frame{Sequence: 3, Updates: []Object{fileObject()}},
		Frame{Sequence: 4, Updates: []Object{}})
	resnapshot := mustSnapshot(t, 4, botObject(2, "Neko renamed"), messageObject("message-a", "user", "Xin chào"), fileObject())
	third := mustIncrement(t, 4, 6,
		Frame{Sequence: 5, Updates: []Object{messageObject("message-b", "assistant", "Chào bạn")}},
		Frame{Sequence: 6, Updates: []Object{botObject(3, "Neko final")}})
	envelopes := map[string]Envelope{"snapshot": initial, "first": first, "second": second, "resnapshot": resnapshot, "third": third, "old": first, "second-gap": second}
	var final []Object
	for _, name := range []string{"client-a.json", "client-b.json"} {
		t.Run(name, func(t *testing.T) {
			var fixture struct {
				Client         string   `json:"client"`
				Steps          []string `json:"steps"`
				ExpectedCursor uint64   `json:"expectedCursor"`
			}
			data, err := os.ReadFile("testdata/" + name)
			if err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(data, &fixture); err != nil {
				t.Fatal(err)
			}
			c, err := NewClient("journal-a", "scope-a")
			if err != nil {
				t.Fatal(err)
			}
			for _, step := range fixture.Steps {
				wire, err := Encode(envelopes[step])
				if err != nil {
					t.Fatal(err)
				}
				e, err := Decode(wire)
				if err != nil {
					t.Fatal(err)
				}
				_, err = c.Apply(e)
				if step == "second-gap" {
					if !errors.Is(err, ErrGap) {
						t.Fatalf("out-of-order packet accepted: %v", err)
					}
					continue
				}
				if err != nil {
					t.Fatalf("%s %s: %v", fixture.Client, step, err)
				}
			}
			if c.Cursor().Sequence != fixture.ExpectedCursor || c.HighWater() != 6 || c.NeedsSnapshot() {
				t.Fatalf("incorrect final cursor: %+v", c.Cursor())
			}
			objects := c.Objects()
			if len(objects) != 4 || objects[0].Bot.Name != "Neko final" {
				t.Fatal("incorrect reconciled projection")
			}
			if final == nil {
				final = objects
			} else if !reflect.DeepEqual(final, objects) {
				t.Fatal("two clients diverged")
			}
		})
	}
}

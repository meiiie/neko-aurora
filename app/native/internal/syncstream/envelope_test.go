package syncstream

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestRejectUnsafeAndUnboundedEnvelope(t *testing.T) {
	cases := []struct {
		name     string
		envelope Envelope
		want     error
	}{
		{"unknown version", Envelope{Version: 2}, ErrInvalid},
		{"unsafe counter", Envelope{Version: Version, InstanceID: "journal-a", ScopeID: "scope-a", Mode: Snapshot, To: MaxSafeSequence + 1, HighWater: MaxSafeSequence + 1, Snapshot: []Object{}, Frames: []Frame{}}, ErrInvalid},
		{"cursor ahead", Envelope{Version: Version, InstanceID: "journal-a", ScopeID: "scope-a", Mode: Increment, After: 3, To: 2, HighWater: 2, Snapshot: []Object{}, Frames: []Frame{}}, ErrInvalid},
		{"oversize frame batch", Envelope{Version: Version, InstanceID: "journal-a", ScopeID: "scope-a", Mode: Increment, To: MaxFrames + 1, HighWater: MaxFrames + 1, Snapshot: []Object{}, Frames: make([]Frame, MaxFrames+1)}, ErrLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.envelope.Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("got %v; want %v", err, tc.want)
			}
		})
	}
}

func TestEnvelopeRoundTripIntegrityAndNoHiddenDTO(t *testing.T) {
	e := mustSnapshot(t, 12, messageObject("message-a", "assistant", "Tiếng Việt: chào bạn"), botObject(2, "Neko"), fileObject())
	wire, err := Encode(e)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeReader(bytes.NewReader(wire))
	if err != nil || !reflect.DeepEqual(e, got) {
		t.Fatalf("roundtrip: %v", err)
	}
	if e.Snapshot[0].Kind != "bot" {
		t.Fatal("constructor did not canonicalize order")
	}
	tampered := e
	tampered.HighWater++
	tampered.To++
	if err := tampered.Validate(); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("tampered counters: %v", err)
	}
	for _, name := range []string{"contextPin", "runtimeContext", "core", "accountLease", "secret", "bytes", "content"} {
		private := strings.Replace(string(wire), `"name":"Neko"`, `"name":"Neko","`+name+`":{"fixture":true}`, 1)
		if _, err := Decode([]byte(private)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("private field %s accepted: %v", name, err)
		}
	}
	for _, bad := range []string{
		strings.Replace(string(wire), `"after":0`, `"after":null`, 1),
		strings.Replace(string(wire), `"deleted":false`, `"deleted":null`, 1),
		strings.Replace(string(wire), `"version":1`, `"version":1,"Version":1`, 1),
		strings.Replace(string(wire), `"name":"Neko"`, `"name":"Neko","name":"Neko"`, 1),
		strings.Replace(string(wire), `"deleted":false,`, "", 1),
		strings.Replace(string(wire), `"frames":[]`, `"frames":null`, 1),
	} {
		if _, err := Decode([]byte(bad)); err == nil {
			t.Fatal("noncanonical schema accepted")
		}
	}
}

func TestFileInitialZeroAccessRevisionPreserved(t *testing.T) {
	o := fileObject()
	o.File.AccessRevision = 0
	e, err := NewSnapshot("journal-a", "scope-a", 0, []Object{o})
	if err != nil {
		t.Fatalf("canonical initial access revision rejected: %v", err)
	}
	wire, err := Encode(e)
	if err != nil {
		t.Fatal(err)
	}
	e, err = Decode(wire)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := NewClient("journal-a", "scope-a")
	if _, err = c.Apply(e); err != nil {
		t.Fatal(err)
	}
	if c.Objects()[0].File.AccessRevision != 0 {
		t.Fatal("initial file counter was fabricated")
	}
	o.File.AccessRevision = MaxSafeSequence + 1
	if _, err = NewSnapshot("journal-a", "scope-a", 0, []Object{o}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unsafe access revision accepted: %v", err)
	}
	o.File.AccessRevision = 0
	o.File.Version = 0
	if _, err = NewSnapshot("journal-a", "scope-a", 0, []Object{o}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero file version accepted: %v", err)
	}
}

func TestEnvelopeModeGapAndBudgetContracts(t *testing.T) {
	initial := botObject(1, "Neko")
	for _, tc := range []struct {
		name  string
		build func() (Envelope, error)
		want  error
	}{
		{"snapshot not complete", func() (Envelope, error) {
			return Seal(Envelope{Version: Version, InstanceID: "journal-a", ScopeID: "scope-a", Mode: Snapshot, To: 1, HighWater: 2, Snapshot: []Object{initial}})
		}, ErrInvalid},
		{"gap", func() (Envelope, error) {
			return NewIncrement("journal-a", "scope-a", 0, 2, []Frame{{Sequence: 2, Updates: []Object{}}})
		}, ErrGap},
		{"empty backlog", func() (Envelope, error) {
			return NewIncrement("journal-a", "scope-a", 0, 1, []Frame{})
		}, ErrGap},
		{"descending", func() (Envelope, error) {
			return NewIncrement("journal-a", "scope-a", 0, 2, []Frame{{Sequence: 2, Updates: []Object{}}, {Sequence: 1, Updates: []Object{}}})
		}, ErrGap},
		{"duplicate entity", func() (Envelope, error) { return NewSnapshot("journal-a", "scope-a", 0, []Object{initial, initial}) }, ErrInvalid},
		{"private role", func() (Envelope, error) {
			return NewSnapshot("journal-a", "scope-a", 0, []Object{messageObject("message-a", "system", "fixture")})
		}, ErrInvalid},
		{"union conflict", func() (Envelope, error) {
			o := initial
			o.Message = messageObject("message-a", "user", "fixture").Message
			return NewSnapshot("journal-a", "scope-a", 0, []Object{o})
		}, ErrInvalid},
		{"unsafe metadata", func() (Envelope, error) {
			o := fileObject()
			o.File.Size = MaxSafeSequence + 1
			return NewSnapshot("journal-a", "scope-a", 0, []Object{o})
		}, ErrInvalid},
		{"object encoded budget", func() (Envelope, error) {
			return NewSnapshot("journal-a", "scope-a", 0, []Object{messageObject("message-a", "user", strings.Repeat("<", 12<<10))})
		}, ErrLimit},
		{"snapshot object count", func() (Envelope, error) { return NewSnapshot("journal-a", "scope-a", 0, make([]Object, MaxObjects+1)) }, ErrLimit},
		{"aggregate updates", func() (Envelope, error) {
			return NewIncrement("journal-a", "scope-a", 0, 2, []Frame{{Sequence: 1, Updates: make([]Object, MaxObjects)}, {Sequence: 2, Updates: []Object{initial}}})
		}, ErrLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.build(); !errors.Is(err, tc.want) {
				t.Fatalf("got %v; want %v", err, tc.want)
			}
		})
	}
	objects := make([]Object, 30)
	for i := range objects {
		objects[i] = messageObject(fmt.Sprintf("message-%03d", i), "user", strings.Repeat("x", 10<<10))
	}
	if _, err := NewSnapshot("journal-a", "scope-a", 0, objects); !errors.Is(err, ErrLimit) {
		t.Fatalf("aggregate wire budget: %v", err)
	}
	frames := make([]Frame, MaxFrames)
	for i := range frames {
		frames[i] = Frame{Sequence: uint64(i + 1), Updates: []Object{}}
	}
	if _, err := NewIncrement("journal-a", "scope-a", 0, MaxFrames, frames); err != nil {
		t.Fatalf("exact max frame bound: %v", err)
	}
	last := mustIncrement(t, MaxSafeSequence-1, MaxSafeSequence, Frame{Sequence: MaxSafeSequence, Updates: []Object{}})
	if err := last.Validate(); err != nil {
		t.Fatalf("safe integer boundary: %v", err)
	}
}

func TestCursorCanonicalIdentityScopeAndAheadFence(t *testing.T) {
	c := Cursor{Version: Version, InstanceID: "journal-a", ScopeID: "scope-a", Sequence: MaxSafeSequence}
	token, err := c.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseCursor(token)
	if err != nil || got != c {
		t.Fatalf("cursor roundtrip: %v", err)
	}
	for _, tc := range []struct {
		instance string
		scope    string
		water    uint64
		want     error
	}{
		{"journal-b", "scope-a", MaxSafeSequence, ErrInstance},
		{"journal-a", "scope-b", MaxSafeSequence, ErrScope},
		{"journal-a", "scope-a", MaxSafeSequence - 1, ErrAhead},
	} {
		if err := c.Check(tc.instance, tc.scope, tc.water); !errors.Is(err, tc.want) {
			t.Fatalf("binding check: %v", err)
		}
	}
	for _, raw := range []string{
		`{"version":1,"instanceId":"journal-a","scopeId":"scope-a","sequence":-1}`,
		`{"version":1,"instanceId":"journal-a","scopeId":"scope-a","sequence":9007199254740992}`,
		`{"version":1,"instanceId":"journal-a","scopeId":"scope-a","sequence":1.5}`,
		`{"version":1,"instanceId":"journal-a","scopeId":"scope-a","sequence":1,"sequence":2}`,
		`{"version":1,"instanceId":"journal-a","scopeId":"scope-a","sequence":0,"token":"fixture"}`,
		`{"sequence":0,"version":1,"instanceId":"journal-a","scopeId":"scope-a"}`,
	} {
		if _, err := ParseCursor(base64.RawURLEncoding.EncodeToString([]byte(raw))); err == nil {
			t.Fatal("unsafe or noncanonical cursor accepted")
		}
	}
	if _, err := ParseCursor(token + "="); err == nil {
		t.Fatal("padded cursor accepted")
	}
	if _, err := ParseCursor(strings.Repeat("x", MaxCursorBytes+1)); !errors.Is(err, ErrLimit) {
		t.Fatalf("cursor bound: %v", err)
	}
}

func FuzzDecodeEnvelope(f *testing.F) {
	e, _ := NewSnapshot("journal-a", "scope-a", 0, []Object{botObject(1, "Neko")})
	wire, _ := Encode(e)
	f.Add(wire)
	f.Add([]byte(`{"version":1,"version":2}`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, data []byte) {
		e, err := Decode(data)
		if err != nil {
			return
		}
		encoded, err := Encode(e)
		if err != nil {
			t.Fatal("decoded envelope cannot encode", err)
		}
		if len(encoded) > MaxEnvelopeBytes {
			t.Fatal("decoded envelope exceeds budget")
		}
		if _, err := Decode(encoded); err != nil {
			t.Fatal("encoded envelope cannot decode", err)
		}
	})
}

func TestDecodeRejectsAmbiguousOrPrivateWire(t *testing.T) {
	cases := []struct {
		name string
		data string
	}{
		{"unknown private field", `{"version":1,"runtimeContext":{"hidden":"fixture"}}`},
		{"duplicate version", `{"version":2,"version":1}`},
		{"missing required keys", `{"version":1}`},
		{"oversize", `{"padding":"` + strings.Repeat("x", MaxEnvelopeBytes) + `"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Decode([]byte(tc.data)); err == nil {
				t.Fatal("ambiguous, private or oversized envelope accepted")
			}
		})
	}
	if _, err := Decode([]byte(`{} {}`)); err == nil {
		t.Fatal("trailing JSON accepted")
	}
	if !json.Valid([]byte(`{"version":1}`)) {
		t.Fatal("fixture is invalid JSON")
	}
}

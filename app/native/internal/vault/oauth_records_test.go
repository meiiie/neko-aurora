package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var recordFixtureID = strings.Repeat("a", 64)

func fixtureOAuthRecord() OAuthRecord {
	return OAuthRecord{Data: json.RawMessage(`{"workspace":"fixture-workspace","phase":"pending","revision":1}`), Secrets: map[string]string{"state": "fixture-private-state", "verifier": "fixture-private-verifier", "refreshToken": "fixture-private-refresh", "accessToken": "fixture-private-refresh-longer"}}
}

func TestOAuthRecordEncryptedIsolatedAndRedactedAfterReopen(t *testing.T) {
	dir := t.TempDir()
	v, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	record := fixtureOAuthRecord()
	if err := v.SetOAuthRecord(recordFixtureID, record); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Set(Meta{Name: "PUBLIC_FIXTURE"}, "public-fixture-secret"); err != nil {
		t.Fatal(err)
	}
	name := oauthRecordPrefix + recordFixtureID
	if _, ok := v.Get(name); ok || v.Has(name) {
		t.Fatal("private record exposed by Get/Has")
	}
	if _, ok := v.Meta(name); ok {
		t.Fatal("private record metadata exposed")
	}
	if len(v.List()) != 1 || len(v.Items()) != 0 {
		t.Fatal("private record appeared in public listing")
	}
	if _, err := v.Set(Meta{Name: name}, "replacement-fixture"); !errors.Is(err, ErrName) {
		t.Fatal("public Set accepted private key")
	}
	if err := v.Delete(name); !errors.Is(err, ErrName) {
		t.Fatal("public Delete accepted private key")
	}
	if _, err := v.GetOAuthRecord(strings.Repeat("b", 64)); !errors.Is(err, ErrMissing) {
		t.Fatal("record identity not enforced")
	}
	disk, err := os.ReadFile(v.path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(disk, record.Data) {
		t.Fatal("state saved without encryption")
	}
	for _, value := range record.Secrets {
		if bytes.Contains(disk, []byte(value)) {
			t.Fatal("credential saved without encryption")
		}
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.GetOAuthRecord(recordFixtureID)
	if err != nil || !reflect.DeepEqual(record, got) {
		t.Fatal("encrypted record did not survive reopen")
	}
	got.Data[0] = '['
	got.Secrets["state"] = "changed-fixture"
	got, err = reopened.GetOAuthRecord(recordFixtureID)
	if err != nil || !reflect.DeepEqual(record, got) {
		t.Fatal("returned record aliases live state")
	}
	var log bytes.Buffer
	var values []string
	for _, value := range record.Secrets {
		values = append(values, value)
	}
	if _, err := LogWriter(&log).Write([]byte(strings.Join(values, " "))); err != nil {
		t.Fatal(err)
	}
	for _, value := range values {
		if strings.Contains(log.String(), value) {
			t.Fatal("OAuth secret was not individually redacted")
		}
	}
	if err := reopened.DeleteOAuthRecord(recordFixtureID); err != nil {
		t.Fatal(err)
	}
	final, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer final.Close()
	if _, err := final.GetOAuthRecord(recordFixtureID); !errors.Is(err, ErrMissing) || len(final.List()) != 1 {
		t.Fatal("deletion did not persist or changed public secret")
	}
}

func TestOAuthRecordBoundsAndHostedKey(t *testing.T) {
	dir := t.TempDir()
	key := bytes.Repeat([]byte{1}, 32)
	v, err := OpenWithKey(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	for _, id := range []string{"", "../x", strings.Repeat("A", 64), strings.Repeat("a", 63), oauthRecordPrefix + recordFixtureID} {
		if err := v.SetOAuthRecord(id, fixtureOAuthRecord()); !errors.Is(err, ErrName) {
			t.Fatal("invalid record ID accepted")
		}
		if _, err := v.GetOAuthRecord(id); !errors.Is(err, ErrName) {
			t.Fatal("invalid record lookup accepted")
		}
		if err := v.DeleteOAuthRecord(id); !errors.Is(err, ErrName) {
			t.Fatal("invalid record delete accepted")
		}
	}
	for label, mutate := range map[string]func(*OAuthRecord){
		"empty-data":          func(r *OAuthRecord) { r.Data = nil },
		"array-data":          func(r *OAuthRecord) { r.Data = json.RawMessage(`[]`) },
		"invalid-json":        func(r *OAuthRecord) { r.Data = json.RawMessage(`{"broken"`) },
		"invalid-utf8":        func(r *OAuthRecord) { r.Data = []byte{'{', '"', 'x', '"', ':', '"', 255, '"', '}'} },
		"large-data":          func(r *OAuthRecord) { r.Data = json.RawMessage(`{"x":"` + strings.Repeat("x", 16<<10) + `"}`) },
		"invalid-field":       func(r *OAuthRecord) { r.Secrets["bad.field"] = "fixture-value" },
		"short-secret":        func(r *OAuthRecord) { r.Secrets["token"] = "abc" },
		"nul-secret":          func(r *OAuthRecord) { r.Secrets["token"] = "fixture\x00secret" },
		"invalid-secret-utf8": func(r *OAuthRecord) { r.Secrets["token"] = "fixture" + string([]byte{255}) },
		"large-secret":        func(r *OAuthRecord) { r.Secrets["token"] = strings.Repeat("x", 8193) },
		"large-envelope": func(r *OAuthRecord) {
			for _, k := range []string{"one", "two", "three", "four", "five"} {
				r.Secrets[k] = strings.Repeat("x", 8192)
			}
		},
		"many-fields": func(r *OAuthRecord) {
			for c := 'a'; c <= 'z'; c++ {
				r.Secrets[string(c)] = "fixture-value"
			}
		},
	} {
		t.Run(label, func(t *testing.T) {
			r := fixtureOAuthRecord()
			mutate(&r)
			if err := v.SetOAuthRecord(recordFixtureID, r); !errors.Is(err, ErrValue) {
				t.Fatal("invalid record accepted")
			}
		})
	}
	if err := v.SetOAuthRecord(recordFixtureID, fixtureOAuthRecord()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vault.key")); !os.IsNotExist(err) {
		t.Fatal("hosted key was written to disk")
	}
	if _, err := OpenWithKey(dir, bytes.Repeat([]byte{2}, 32)); !errors.Is(err, ErrCorrupt) {
		t.Fatal("wrong hosted key accepted")
	}
	if strings.Contains(v.Redact("fixture-private-state"), "fixture-private-state") {
		t.Fatal("wrong-key open changed live redaction")
	}
}

func TestOAuthRecordPersistenceFailureRestoresPreviousState(t *testing.T) {
	for _, stage := range []string{"create", "replace"} {
		t.Run(stage, func(t *testing.T) {
			dir := t.TempDir()
			v, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer v.Close()
			original := fixtureOAuthRecord()
			if err := v.SetOAuthRecord(recordFixtureID, original); err != nil {
				t.Fatal(err)
			}
			path := v.path
			disk, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			bad := t.TempDir()
			if stage == "create" {
				file := filepath.Join(bad, "file")
				if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
				bad = filepath.Join(file, "vault.json")
			}
			v.path = bad
			update := fixtureOAuthRecord()
			update.Secrets["refreshToken"] = "fixture-new-rotated-token"
			setErr := v.SetOAuthRecord(recordFixtureID, update)
			deleteErr := v.DeleteOAuthRecord(recordFixtureID)
			newErr := v.SetOAuthRecord(strings.Repeat("b", 64), update)
			v.path = path
			if setErr == nil || deleteErr == nil || newErr == nil {
				t.Fatal("fixture did not fail writes")
			}
			got, err := v.GetOAuthRecord(recordFixtureID)
			if err != nil || !reflect.DeepEqual(got, original) {
				t.Fatal("failed persistence changed live record")
			}
			if _, err := v.GetOAuthRecord(strings.Repeat("b", 64)); !errors.Is(err, ErrMissing) {
				t.Fatal("failed creation published record")
			}
			if strings.Contains(v.Redact(original.Secrets["refreshToken"]), original.Secrets["refreshToken"]) {
				t.Fatal("failed persistence lost redaction")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(disk, after) {
				t.Fatal("failed persistence changed disk")
			}
			reopened, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			got, err = reopened.GetOAuthRecord(recordFixtureID)
			if err != nil || !reflect.DeepEqual(got, original) {
				t.Fatal("failed persistence changed reopened record")
			}
		})
	}
}

func TestOAuthRecordRejectsInvalidEnvelopeAndIdentity(t *testing.T) {
	for _, mode := range []string{"version", "unknown-field", "wrong-kind", "wrong-name", "oversize", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			v, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer v.Close()
			if err := v.SetOAuthRecord(recordFixtureID, fixtureOAuthRecord()); err != nil {
				t.Fatal(err)
			}
			name := oauthRecordPrefix + recordFixtureID
			e := v.items[name]
			if mode == "wrong-kind" {
				e.Kind = "plain"
			} else if mode == "wrong-name" {
				e.Name = oauthRecordPrefix + strings.Repeat("b", 64)
			} else if mode == "oversize" {
				e.CT = make([]byte, maxOAuthRecordBytes+v.aead.Overhead()+1)
			} else if mode != "duplicate" {
				value := v.cache[name]
				if mode == "version" {
					value = strings.Replace(value, `"version":1`, `"version":9`, 1)
				} else {
					value = strings.TrimSuffix(value, "}") + `,"unexpected":true}`
				}
				e.CT = v.aead.Seal(nil, e.Nonce, []byte(value), aad(e.V, e.Name))
			}
			entries := []entry{e}
			if mode == "duplicate" {
				entries = append(entries, e)
			}
			b, err := json.Marshal(entries)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(v.path, b, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(dir); !errors.Is(err, ErrCorrupt) {
				t.Fatal("invalid encrypted record accepted")
			}
			var log bytes.Buffer
			_, _ = LogWriter(&log).Write([]byte("fixture-private-state"))
			if strings.Contains(log.String(), "fixture-private-state") {
				t.Fatal("failed open replaced active record redaction")
			}
		})
	}
}

package vault

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestItemHintsPersistEncryptedAndNeverFill(t *testing.T) {
	dir, v, it := itemMergeFixture(t)
	const hint = "Only use this fixture for business trips"
	got, err := v.SaveItem(it.ID, "login", "Travel", map[string]string{}, hint)
	if err != nil || got.UseHint != hint {
		t.Fatal("hint save failed")
	}
	if !reflect.DeepEqual(got.Filled, []string{"site", "username", "password"}) {
		t.Fatal("hint became a credential field or save metadata omitted existing fields")
	}
	b, err := os.ReadFile(v.path)
	if err != nil || bytes.Contains(b, []byte(hint)) {
		t.Fatal("hint was not stored inside sealed payload")
	}
	m, _ := v.Meta(itemName(it.ID))
	if strings.Contains(m.Hint, hint) || strings.Contains(m.Label, hint) {
		t.Fatal("hint leaked into plaintext metadata")
	}
	v2 := itemReopen(t, dir)
	listed, ok := v2.Item(it.ID)
	if !ok || listed.UseHint != hint {
		t.Fatal("hint did not survive reopen")
	}
	f, ok := v2.Fields(it.ID)
	if !ok || len(f) != 3 || f[itemUseHint] != "" {
		t.Fatal("internal hint became a fillable field")
	}
	got, err = v2.SaveItem(it.ID, "login", "Renamed", nil)
	if err != nil || got.UseHint != hint {
		t.Fatal("legacy label-only update lost hint")
	}
	got, err = v2.SaveItem(it.ID, "login", "Renamed", nil, "")
	if err != nil || got.UseHint != "" {
		t.Fatal("explicit empty did not clear hint")
	}
	if final, ok := itemReopen(t, dir).Item(it.ID); !ok || final.UseHint != "" {
		t.Fatal("clear did not survive reopen")
	}
}

func TestItemHintsValidationAndRollback(t *testing.T) {
	_, v, it := itemMergeFixture(t)
	for _, hint := range []string{strings.Repeat("猫", 501), "zero\x00byte", string([]byte{0xff})} {
		if _, err := v.SaveItem(it.ID, "login", "Changed", nil, hint); !errors.Is(err, ErrValue) {
			t.Fatal("invalid hint accepted")
		}
	}
	if _, err := v.SaveItem(it.ID, "login", "Changed", nil, "a", "b"); !errors.Is(err, ErrValue) {
		t.Fatal("ambiguous options accepted")
	}
	if _, err := v.SaveItem(it.ID, "login", "Changed", map[string]string{itemUseHint: "smuggled"}); !errors.Is(err, ErrType) {
		t.Fatal("reserved hint field accepted as a credential")
	}
	if _, err := v.SaveItem(it.ID, "login", "Before", nil, strings.Repeat("猫", 500)); err != nil {
		t.Fatal("500 Unicode code points rejected")
	}
	before, _ := v.Item(it.ID)
	path := v.path
	v.path = t.TempDir()
	_, err := v.SaveItem(it.ID, "login", "After", nil, "replacement")
	v.path = path
	if err == nil {
		t.Fatal("fixture save unexpectedly succeeded")
	}
	after, _ := v.Item(it.ID)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed hint save changed item metadata")
	}
}

func TestItemHintsIncludedInAggregateLimit(t *testing.T) {
	_, v, it := itemMergeFixture(t)
	fields := map[string]string{"password": strings.Repeat("\x01", 512), "notes": strings.Repeat("\x02", 512), "username": strings.Repeat("\x03", 300)}
	if _, err := v.SaveItem(it.ID, "login", "Fixture", fields); err != nil {
		t.Fatal("near-limit fixture did not fit")
	}
	if _, err := v.SaveItem(it.ID, "login", "Fixture", nil, strings.Repeat("h", 500)); !errors.Is(err, ErrValue) {
		t.Fatal("hint bypassed aggregate payload limit")
	}
	if got, ok := v.Item(it.ID); !ok || got.UseHint != "" {
		t.Fatal("rejected oversized hint changed public metadata")
	}
}

func TestItemHintsConcurrentCredentialEditPreservesHint(t *testing.T) {
	dir, v, it := itemMergeFixture(t)
	g := itemPauseNextNonce(t, v)
	var once sync.Once
	unblock := func() { once.Do(func() { close(g.release) }) }
	t.Cleanup(unblock)
	first, second := make(chan error, 1), make(chan error, 1)
	go func() { _, err := v.SaveItem(it.ID, "login", "Travel", nil, "Use for business trips"); first <- err }()
	select {
	case <-g.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("hint write did not reach nonce boundary")
	}
	go func() {
		_, err := v.SaveItem(it.ID, "login", "Travel", map[string]string{"password": "fixture-hints-updated"})
		second <- err
	}()
	var secondErr error
	finished := false
	select {
	case secondErr = <-second:
		finished = true
	case <-time.After(250 * time.Millisecond):
	}
	unblock()
	if err := itemWaitResult(t, first); err != nil {
		t.Fatal(err)
	}
	if !finished {
		secondErr = itemWaitResult(t, second)
	}
	if secondErr != nil {
		t.Fatal(secondErr)
	}
	for _, store := range []*Vault{v, itemReopen(t, dir)} {
		got, ok := store.Item(it.ID)
		f, _ := store.Fields(it.ID)
		if !ok || got.UseHint != "Use for business trips" || f["password"] != "fixture-hints-updated" {
			t.Fatal("concurrent edit lost hint or credential patch")
		}
	}
}

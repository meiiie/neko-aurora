package vault

import (
	"bytes"
	"crypto/cipher"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type itemNonceGate struct {
	cipher.AEAD
	paused  atomic.Bool
	entered chan struct{}
	release chan struct{}
}

func (g *itemNonceGate) NonceSize() int {
	if g.paused.CompareAndSwap(false, true) {
		close(g.entered)
		<-g.release
	}
	return g.AEAD.NonceSize()
}

func itemMergeFixture(t *testing.T) (string, *Vault, Item) {
	t.Helper()
	dir := t.TempDir()
	v, err := OpenWithKey(dir, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	it, err := v.SaveItem("", "login", "Fixture login", map[string]string{"password": "fixture-merge-original", "username": "before@example.test", "site": "example.test"})
	if err != nil {
		t.Fatal(err)
	}
	return dir, v, it
}

func itemPauseNextNonce(t *testing.T, v *Vault) *itemNonceGate {
	t.Helper()
	g := &itemNonceGate{AEAD: v.aead, entered: make(chan struct{}), release: make(chan struct{})}
	v.aead = g
	return g
}

func itemWaitResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("item operation did not finish")
		return nil
	}
}

func TestItemMergeConcurrentEditsPreserveBothPatches(t *testing.T) {
	dir, v, it := itemMergeFixture(t)
	g := itemPauseNextNonce(t, v)
	var release sync.Once
	unblock := func() { release.Do(func() { close(g.release) }) }
	t.Cleanup(unblock)
	first, second := make(chan error, 1), make(chan error, 1)
	go func() {
		_, err := v.SaveItem(it.ID, "login", "Fixture login", map[string]string{"username": "after@example.test"})
		first <- err
	}()
	select {
	case <-g.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("write did not reach nonce boundary")
	}
	go func() {
		_, err := v.SaveItem(it.ID, "login", "Fixture login", map[string]string{"password": "fixture-merge-new"})
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
		fields, ok := store.Fields(it.ID)
		if !ok || fields["username"] != "after@example.test" || fields["password"] != "fixture-merge-new" || fields["site"] != "example.test" {
			t.Fatal("acknowledged disjoint update lost a field")
		}
	}
}

func itemReopen(t *testing.T, dir string) *Vault {
	t.Helper()
	v, err := OpenWithKey(dir, make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	return v
}

func TestItemMergeDeleteCannotBeUndoneByOlderUpdate(t *testing.T) {
	dir, v, it := itemMergeFixture(t)
	g := itemPauseNextNonce(t, v)
	var release sync.Once
	unblock := func() { release.Do(func() { close(g.release) }) }
	t.Cleanup(unblock)
	update, del := make(chan error, 1), make(chan error, 1)
	go func() {
		_, err := v.SaveItem(it.ID, "login", "Changed", map[string]string{"username": "changed@example.test"})
		update <- err
	}()
	select {
	case <-g.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("write did not reach nonce boundary")
	}
	go func() { del <- v.DeleteItem(it.ID) }()
	finished := false
	var deleteErr error
	select {
	case deleteErr = <-del:
		finished = true
	case <-time.After(250 * time.Millisecond):
	}
	unblock()
	if err := itemWaitResult(t, update); err != nil && !errors.Is(err, ErrMissing) {
		t.Fatal(err)
	}
	if !finished {
		deleteErr = itemWaitResult(t, del)
	}
	if deleteErr != nil {
		t.Fatal(deleteErr)
	}
	for _, store := range []*Vault{v, itemReopen(t, dir)} {
		if _, ok := store.Fields(it.ID); ok {
			t.Fatal("acknowledged deletion was resurrected by an older update")
		}
	}
}

func TestItemMergeRejectsCorruptStoredFields(t *testing.T) {
	for _, payload := range []string{"null", "{broken", "[]", `{"password":42}`} {
		t.Run(payload, func(t *testing.T) {
			dir, v, it := itemMergeFixture(t)
			if _, err := v.Set(Meta{Name: itemName(it.ID), Kind: "vault:login", Label: "Before"}, payload); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(v.path)
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := v.Fields(it.ID); ok {
				t.Error("corrupt fields reported as a valid item")
			}
			if _, err := v.SaveItem(it.ID, "login", "After", map[string]string{"password": "fixture-replacement"}); !errors.Is(err, ErrCorrupt) {
				t.Error("update did not reject corrupt stored fields")
			}
			after, err := os.ReadFile(v.path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("failed update replaced corrupt stored bytes")
			}
			stored, ok := itemReopen(t, dir).Get(itemName(it.ID))
			if !ok || stored != payload {
				t.Fatal("failed update changed the sealed payload")
			}
		})
	}
}

func TestItemMergeSaveFailureRollsBackAllViews(t *testing.T) {
	for _, stage := range []string{"create", "replace"} {
		t.Run(stage, func(t *testing.T) {
			dir, v, it := itemMergeFixture(t)
			path := v.path
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			items := v.Items()
			fields, _ := v.Fields(it.ID)
			if stage == "create" {
				block := filepath.Join(t.TempDir(), "block")
				if err := os.WriteFile(block, []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
				v.path = filepath.Join(block, "vault.json")
			} else {
				v.path = t.TempDir()
			}
			_, err = v.SaveItem(it.ID, "login", "Changed", map[string]string{"password": "fixture-new-not-committed"})
			v.path = path
			if err == nil {
				t.Fatal("fixture save unexpectedly succeeded")
			}
			got, ok := v.Fields(it.ID)
			if !ok || !reflect.DeepEqual(got, fields) || !reflect.DeepEqual(v.Items(), items) {
				t.Fatal("failed save changed live item or metadata")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("failed save changed original stored bytes")
			}
			got, ok = itemReopen(t, dir).Fields(it.ID)
			if !ok || !reflect.DeepEqual(got, fields) {
				t.Fatal("failed save changed reopened item")
			}
			if strings.Contains(v.Redact("fixture-merge-original"), "fixture-merge-original") {
				t.Fatal("failed save disabled existing field redaction")
			}
		})
	}
}

func TestItemMergeRetainsAggregateValueLimit(t *testing.T) {
	_, v, it := itemMergeFixture(t)
	_, err := v.SaveItem(it.ID, "login", "Changed", map[string]string{"password": strings.Repeat("\x01", 512), "notes": strings.Repeat("\x02", 512), "username": strings.Repeat("\x03", 512)})
	if !errors.Is(err, ErrValue) {
		t.Fatal("escaped aggregate payload limit was bypassed")
	}
	if f, ok := v.Fields(it.ID); !ok || f["password"] != "fixture-merge-original" {
		t.Fatal("rejected aggregate changed existing fields")
	}
}

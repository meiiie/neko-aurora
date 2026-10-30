package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func contactFixture(t *testing.T) (string, *Vault, Item) {
	t.Helper()
	dir := t.TempDir()
	v := itemReopen(t, dir)
	it, err := v.SaveItem("", "identity", "Fixture contact", map[string]string{"name": "Fixture Owner", "email": "first@example.test", "phone": "+84 900 111 222", "address": "Fixture address"}, "Travel only")
	if err != nil {
		t.Fatal(err)
	}
	return dir, v, it
}

func TestItemContactsShortPhoneMask(t *testing.T) {
	_, v, _ := contactFixture(t)
	for _, phone := range []string{"123", "1234"} {
		it, err := v.SaveItem("", "identity", "Short fixture", map[string]string{"phone": phone})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(it.Contacts)
		if bytes.Contains(b, []byte(phone)) {
			t.Fatal("metadata exposed the entire short phone value")
		}
	}
}

func TestItemContactsLegacyEditFencesUnmigratedEditor(t *testing.T) {
	_, v, it := contactFixture(t)
	if it.Contacts.Revision != "0" {
		t.Fatal("fixture must start before migration")
	}
	if _, err := v.SaveItem(it.ID, "identity", it.Label, map[string]string{"email": "changed-legacy@example.test"}); err != nil {
		t.Fatal(err)
	}
	_, err := v.ChangeContact(it.ID, ContactChange{Revision: it.Contacts.Revision, Kind: "email", Action: "remove", EntryID: "legacy-email"})
	if !errors.Is(err, ErrContactConflict) {
		t.Fatal("stale unmigrated editor can remove a changed legacy contact")
	}
	fields, _ := v.Fields(it.ID)
	if fields["email"] != "changed-legacy@example.test" {
		t.Fatal("legacy replacement was removed")
	}
}

func TestItemContactsConcurrentGeneralEdit(t *testing.T) {
	dir, v, it := contactFixture(t)
	gate := itemPauseNextNonce(t, v)
	var once sync.Once
	release := func() { once.Do(func() { close(gate.release) }) }
	t.Cleanup(release)
	contact, other := make(chan error, 1), make(chan error, 1)
	go func() {
		_, err := v.ChangeContact(it.ID, ContactChange{Revision: "0", Kind: "email", Action: "add", Value: "new@example.test"})
		contact <- err
	}()
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("contact edit did not enter save")
	}
	go func() {
		_, err := v.SaveItem(it.ID, "identity", "New label", map[string]string{"address": "Updated address"})
		other <- err
	}()
	release()
	if err := itemWaitResult(t, contact); err != nil {
		t.Fatal(err)
	}
	if err := itemWaitResult(t, other); err != nil {
		t.Fatal(err)
	}
	for _, store := range []*Vault{v, itemReopen(t, dir)} {
		fields, c, err := store.RevealItem(it.ID)
		if err != nil || len(c.Emails) != 2 || fields["address"] != "Updated address" {
			t.Fatal("concurrent general edit lost contact mutation")
		}
	}
}

func TestItemContactsCorruptCollectionFailsClosed(t *testing.T) {
	_, v, it := contactFixture(t)
	it = contactEdit(t, v, it, "email", "add", "", "new@example.test")
	v.mu.Lock()
	fields, _ := v.fieldsLocked(it.ID)
	fields[itemContacts] = "{"
	b, _ := json.Marshal(fields)
	_, err := v.setLocked(v.items[itemName(it.ID)].Meta, string(b))
	v.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := v.RevealItem(it.ID); !errors.Is(err, ErrCorrupt) {
		t.Fatal("corrupt collection revealed")
	}
	if _, err := v.ChangeContact(it.ID, ContactChange{Revision: it.Contacts.Revision, Kind: "email", Action: "add", Value: "another@example.test"}); !errors.Is(err, ErrCorrupt) {
		t.Fatal("corrupt collection edited")
	}
	if _, err := v.SaveItem(it.ID, "identity", "Changed", map[string]string{}); !errors.Is(err, ErrCorrupt) {
		t.Fatal("legacy save overwrote corrupt collection")
	}
}

func contactEdit(t *testing.T, v *Vault, it Item, kind, action, id, value string) Item {
	t.Helper()
	got, err := v.ChangeContact(it.ID, ContactChange{Revision: it.Contacts.Revision, Kind: kind, Action: action, EntryID: id, Value: value})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestItemContactsMigratePrimaryRevealAndReopen(t *testing.T) {
	dir, v, it := contactFixture(t)
	if it.Contacts == nil || it.Contacts.Revision != "0" || len(it.Contacts.Emails) != 1 || len(it.Contacts.Phones) != 1 {
		t.Fatal("legacy contacts missing")
	}
	it = contactEdit(t, v, it, "email", "add", "", "second@example.test")
	second := it.Contacts.Emails[1].ID
	it = contactEdit(t, v, it, "phone", "add", "", "+84 900 333 444")
	it = contactEdit(t, v, it, "email", "primary", second, "")
	for _, store := range []*Vault{v, itemReopen(t, dir)} {
		fields, contacts, err := store.RevealItem(it.ID)
		if err != nil || fields["email"] != "second@example.test" || contacts.Emails[0].ID != second || contacts.Emails[1].Value != "first@example.test" || len(contacts.Phones) != 2 || fields["address"] != "Fixture address" {
			t.Fatal("contact values lost or primary order changed")
		}
		fill, ok := store.Fields(it.ID)
		if !ok || len(fill) != 4 || fill["email"] != "second@example.test" || fill[itemContacts] != "" || fill[itemUseHint] != "" {
			t.Fatal("internal collection leaked into fill fields")
		}
		metadata, _ := json.Marshal(store.Items())
		disk, _ := os.ReadFile(filepath.Join(dir, "vault.json"))
		for _, value := range []string{"first@example.test", "second@example.test", "+84 900 111 222", "+84 900 333 444"} {
			if bytes.Contains(metadata, []byte(value)) || bytes.Contains(disk, []byte(value)) || strings.Contains(store.Redact("contact "+value), value) {
				t.Fatal("contact exposed in metadata/storage/logs")
			}
		}
	}
	it = contactEdit(t, v, it, "email", "replace", second, "replacement@example.test")
	it = contactEdit(t, v, it, "email", "remove", second, "")
	fields, _, _ := v.RevealItem(it.ID)
	if fields["email"] != "first@example.test" || it.UseHint != "Travel only" {
		t.Fatal("removing primary lost fallback or guidance")
	}
	it = contactEdit(t, v, it, "email", "remove", it.Contacts.Emails[0].ID, "")
	fields, _, _ = itemReopen(t, dir).RevealItem(it.ID)
	if fields["email"] != "" || len(it.Contacts.Emails) != 0 {
		t.Fatal("last contact removal not durable")
	}
}

func TestItemContactsConcurrentRevisionAndLegacyEdits(t *testing.T) {
	_, v, it := contactFixture(t)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, value := range []string{"one@example.test", "two@example.test"} {
		wg.Add(1)
		go func(value string) {
			defer wg.Done()
			_, err := v.ChangeContact(it.ID, ContactChange{Revision: "0", Kind: "email", Action: "add", Value: value})
			errs <- err
		}(value)
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, ErrContactConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal("concurrent stale mutation was accepted")
	}
	it, _ = v.Item(it.ID)
	prior := it.Contacts.Revision
	got, err := v.SaveItem(it.ID, "identity", "Renamed", map[string]string{"address": "New address"})
	if err != nil || got.Contacts.Revision != prior || len(got.Contacts.Emails) != 2 {
		t.Fatal("unrelated edit lost channels or changed contact revision")
	}
	got, err = v.SaveItem(it.ID, "identity", "Renamed", map[string]string{"email": "primary-new@example.test"})
	if err != nil || got.Contacts.Revision == prior || len(got.Contacts.Emails) != 2 {
		t.Fatal("legacy primary edit did not preserve channels and fence old editor")
	}
	_, err = v.ChangeContact(it.ID, ContactChange{Revision: prior, Kind: "phone", Action: "add", Value: "+1 212 555 0100"})
	if !errors.Is(err, ErrContactConflict) {
		t.Fatal("stale revision accepted after legacy edit")
	}
}

func TestItemContactsFailureRollback(t *testing.T) {
	for _, action := range []string{"add", "replace", "remove", "primary"} {
		t.Run(action, func(t *testing.T) {
			dir, v, it := contactFixture(t)
			it = contactEdit(t, v, it, "email", "add", "", "second@example.test")
			before, contacts, _ := v.RevealItem(it.ID)
			bytesBefore, _ := os.ReadFile(v.path)
			change := ContactChange{Revision: it.Contacts.Revision, Kind: "email", Action: action, EntryID: it.Contacts.Emails[1].ID}
			if action == "add" {
				change.EntryID = ""
			}
			if action == "add" || action == "replace" {
				change.Value = "next@example.test"
			}
			original := v.path
			v.path = t.TempDir()
			_, err := v.ChangeContact(it.ID, change)
			v.path = original
			if err == nil {
				t.Fatal("failed save accepted")
			}
			bytesAfter, _ := os.ReadFile(v.path)
			if !bytes.Equal(bytesBefore, bytesAfter) {
				t.Fatal("failed save changed disk")
			}
			for _, store := range []*Vault{v, itemReopen(t, dir)} {
				fields, got, err := store.RevealItem(it.ID)
				if err != nil || !reflect.DeepEqual(before, fields) || !reflect.DeepEqual(contacts, got) {
					t.Fatal("failed save changed contacts or revision")
				}
				if strings.Contains(store.Redact("second@example.test"), "second@example.test") {
					t.Fatal("failed save lost secondary redaction")
				}
			}
		})
	}
}

func TestItemContactsRejectInvalidWithoutMutation(t *testing.T) {
	_, v, it := contactFixture(t)
	cases := []ContactChange{
		{Revision: "0", Kind: "email", Action: "add", Value: "first@example.test"},
		{Revision: "0", Kind: "email", Action: "add", Value: "Name <new@example.test>"},
		{Revision: "0", Kind: "email", Action: "add", Value: "bad\n@example.test"},
		{Revision: "0", Kind: "email", Action: "add", Value: strings.Repeat("x", 255) + "@example.test"},
		{Revision: "0", Kind: "phone", Action: "add", Value: "call me"},
		{Revision: "0", Kind: "phone", Action: "add", Value: "123"},
		{Revision: "0", Kind: "email", Action: "add", EntryID: "unexpected", Value: "new@example.test"},
		{Revision: "0", Kind: "email", Action: "remove", EntryID: "unknown"},
		{Revision: "0", Kind: "email", Action: "primary", EntryID: "legacy-email", Value: "unexpected"},
		{Revision: "stale", Kind: "email", Action: "remove", EntryID: "legacy-email"},
		{Revision: "0", Kind: "fax", Action: "add", Value: "12345"},
		{Kind: "email", Action: "add", Value: "new@example.test"},
	}
	before, _ := json.Marshal(v.Items())
	for i, c := range cases {
		if _, err := v.ChangeContact(it.ID, c); err == nil {
			t.Fatalf("invalid case %d accepted", i)
		}
		after, _ := json.Marshal(v.Items())
		if !bytes.Equal(before, after) {
			t.Fatalf("invalid case %d changed state", i)
		}
	}
	for i := 1; i < maxContacts; i++ {
		it = contactEdit(t, v, it, "email", "add", "", strings.Repeat("a", i)+"@example.test")
	}
	if _, err := v.ChangeContact(it.ID, ContactChange{Revision: it.Contacts.Revision, Kind: "email", Action: "add", Value: "overflow@example.test"}); !errors.Is(err, ErrValue) {
		t.Fatal("contact count exceeded")
	}
	if err := v.DeleteItem(it.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := v.ChangeContact(it.ID, ContactChange{Revision: it.Contacts.Revision, Kind: "email", Action: "add", Value: "new@example.test"}); !errors.Is(err, ErrMissing) {
		t.Fatal("deleted item resurrected")
	}
}

package vault

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
)

func paymentFixture(t *testing.T) (string, *Vault, Item, Item) {
	t.Helper()
	dir, v, _ := itemMergeFixture(t)
	a, err := v.SaveItem("", "card", "Fixture travel", map[string]string{"number": "4111111111111111", "cvc": "123"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := v.SaveItem("", "card", "Fixture spare", map[string]string{"number": "5555555555554444", "cvc": "456"})
	if err != nil {
		t.Fatal(err)
	}
	return dir, v, a, b
}

func preferredPayment(t *testing.T, v *Vault, want string) {
	t.Helper()
	count := 0
	for _, it := range v.Items() {
		if it.PreferredPayment {
			count++
			if it.Type != "card" || it.ID != want {
				t.Fatal("wrong preferred card")
			}
		}
	}
	if want == "" && count != 0 || want != "" && count != 1 {
		t.Fatal("preference cardinality mismatch")
	}
}

func TestItemPaymentPreferencePersistenceAndClear(t *testing.T) {
	dir, v, a, b := paymentFixture(t)
	preferredPayment(t, v, "")
	for _, id := range []string{a.ID, b.ID, "", a.ID} {
		if err := v.PreferPayment(id); err != nil {
			t.Fatal(err)
		}
		preferredPayment(t, v, id)
		preferredPayment(t, itemReopen(t, dir), id)
		if id != "" && v.Items()[0].ID != id {
			t.Fatal("preferred payment not first in metadata")
		}
	}
	if _, err := v.SaveItem(a.ID, "card", "Renamed", map[string]string{"exp": "12/30"}, "Travel only"); err != nil {
		t.Fatal(err)
	}
	preferredPayment(t, itemReopen(t, dir), a.ID)
	before, _ := v.Fields(a.ID)
	if err := v.PreferPayment(b.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := v.Fields(a.ID)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("preference changed credential values")
	}
	if _, ok := after["preferredPayment"]; ok {
		t.Fatal("preference became a fillable field")
	}
	if err := v.DeleteItem(b.ID); err != nil {
		t.Fatal(err)
	}
	preferredPayment(t, itemReopen(t, dir), "")
	raw, err := os.ReadFile(v.path)
	if err != nil || bytes.Contains(raw, []byte("4111111111111111")) {
		t.Fatal("card number exposed in preference persistence")
	}
}

func TestItemPaymentPreferenceValidationRollback(t *testing.T) {
	dir, v, a, b := paymentFixture(t)
	if err := v.PreferPayment(a.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"not-an-id", "deadbeef"} {
		if !errors.Is(v.PreferPayment(id), ErrMissing) {
			t.Fatal("invalid card accepted")
		}
	}
	for _, it := range v.Items() {
		if it.Type == "login" && !errors.Is(v.PreferPayment(it.ID), ErrType) {
			t.Fatal("login accepted as payment")
		}
	}
	before := v.Items()
	path := v.path
	v.path = t.TempDir()
	err := v.PreferPayment(b.ID)
	v.path = path
	if err == nil || !reflect.DeepEqual(before, v.Items()) {
		t.Fatal("failed preference changed RAM metadata")
	}
	preferredPayment(t, itemReopen(t, dir), a.ID)
	v.path = t.TempDir()
	err = v.PreferPayment("")
	v.path = path
	if err == nil {
		t.Fatal("failed clear acknowledged")
	}
	preferredPayment(t, v, a.ID)
	preferredPayment(t, itemReopen(t, dir), a.ID)
}

func TestItemPaymentPreferenceSerializesWithEditsAndDelete(t *testing.T) {
	dir, v, a, b := paymentFixture(t)
	g := itemPauseNextNonce(t, v)
	edited := make(chan error, 1)
	go func() { _, err := v.SaveItem(a.ID, "card", "Edited", nil); edited <- err }()
	<-g.entered
	chosen := make(chan error, 1)
	go func() { chosen <- v.PreferPayment(a.ID) }()
	close(g.release)
	if err := itemWaitResult(t, edited); err != nil {
		t.Fatal(err)
	}
	if err := itemWaitResult(t, chosen); err != nil {
		t.Fatal(err)
	}
	preferredPayment(t, itemReopen(t, dir), a.ID)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := a.ID
			if i%2 == 0 {
				id = b.ID
			}
			errs <- v.PreferPayment(id)
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	items := v.Items()
	if len(items) < 1 || !items[0].PreferredPayment {
		t.Fatal("concurrent choices lost default")
	}
	preferredPayment(t, itemReopen(t, dir), items[0].ID)
	if err := v.DeleteItem(a.ID); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(v.PreferPayment(a.ID), ErrMissing) {
		t.Fatal("deleted card was reselected")
	}
	if _, ok := v.Item(a.ID); ok {
		t.Fatal("preference resurrected card")
	}
}

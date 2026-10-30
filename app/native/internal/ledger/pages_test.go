package ledger

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"nekoaurora.local/native/internal/identity"
)

func pageStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, dir
}

func pageInput() PageInput {
	return PageInput{Title: "Tài liệu Aurora", Text: "# Xin chào\n\nBản gốc.\n", ClientNonce: "page-create-fixture"}
}

func TestPageVersionsReceiptsAndArchiveSurviveRestart(t *testing.T) {
	s, dir := pageStore(t)
	in := pageInput()
	first, duplicate, err := s.SavePage(in)
	if err != nil || duplicate || first.Revision != 1 || first.Text != in.Text {
		t.Fatal("initial page", err)
	}
	edit := PageInput{ID: first.ID, BaseRevision: 1, Title: first.Title, Text: "Bản sửa 🐈\n", ClientNonce: "page-edit-fixture"}
	second, _, err := s.SavePage(edit)
	if err != nil || second.Revision != 2 || second.SHA256 == first.SHA256 {
		t.Fatal("edit", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal("reopen", err)
	}
	defer reopened.Close()
	before := reopened.Snapshot().Sequence
	for _, input := range []PageInput{in, edit} {
		p, duplicate, err := reopened.SavePage(input)
		if err != nil || !duplicate || p.Revision != input.BaseRevision+1 || p.Text != input.Text {
			t.Fatal("lost immutable receipt", err)
		}
	}
	if reopened.Snapshot().Sequence != before {
		t.Fatal("duplicate save appended another frame")
	}
	changed := edit
	changed.Text = "different nonce payload"
	if _, _, err := reopened.SavePage(changed); !errors.Is(err, ErrConflict) {
		t.Fatal("nonce reuse allowed", err)
	}
	changed.ClientNonce = "page-stale-fixture"
	if _, _, err := reopened.SavePage(changed); !errors.Is(err, ErrConflict) {
		t.Fatal("stale revision allowed", err)
	}
	archive := edit
	archive.BaseRevision, archive.Archived, archive.ClientNonce = 2, true, "page-archive-fixture"
	if p, _, err := reopened.SavePage(archive); err != nil || !p.Archived || p.Revision != 3 {
		t.Fatal("archive", err)
	}
	restore := PageInput{ID: first.ID, BaseRevision: 3, Title: first.Title, Text: first.Text, ClientNonce: "page-restore-fixture"}
	if p, _, err := reopened.SavePage(restore); err != nil || p.Archived || p.Revision != 4 || p.SHA256 != first.SHA256 {
		t.Fatal("restore as new version", err)
	}
	if p, err := reopened.Page(first.ID, 1); err != nil || p != first {
		t.Fatal("old version changed", err)
	}
	if p, found := reopened.PageReceipt(edit.ClientNonce); !found || p != second {
		t.Fatal("old receipt moved to the current revision")
	}
}

func TestPageConcurrentEditorsHaveOneWinner(t *testing.T) {
	s, _ := pageStore(t)
	p, _, err := s.SavePage(pageInput())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := s.SavePage(PageInput{ID: p.ID, BaseRevision: 1, Title: p.Title, Text: fmt.Sprint(i), ClientNonce: fmt.Sprintf("page-competing-%d", i)})
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("got %d successful competing writes", winners)
	}
}

func TestPageValidationLimitsAndHistoryPagination(t *testing.T) {
	s, _ := pageStore(t)
	for _, mutate := range []func(*PageInput){
		func(p *PageInput) { p.Title = " " }, func(p *PageInput) { p.Title = "line\nbreak" },
		func(p *PageInput) { p.Title = strings.Repeat("x", 513) }, func(p *PageInput) { p.Text = string([]byte{255}) },
		func(p *PageInput) { p.Text = "nul\x00" }, func(p *PageInput) { p.Text = strings.Repeat("x", MaxPageTextBytes+1) },
		func(p *PageInput) { p.ID = "../../outside" }, func(p *PageInput) { p.BaseRevision = 1 },
		func(p *PageInput) { p.Archived = true }, func(p *PageInput) { p.ClientNonce = "bad" },
	} {
		in := pageInput()
		mutate(&in)
		if _, _, err := s.SavePage(in); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid page admitted", err)
		}
	}
	in := pageInput()
	in.Text = strings.Repeat("x", MaxPageTextBytes)
	p, _, err := s.SavePage(in)
	if err != nil {
		t.Fatal("exact text bound", err)
	}
	for i := 2; i <= 55; i++ {
		p, _, err = s.SavePage(PageInput{ID: p.ID, BaseRevision: p.Revision, Title: p.Title, Text: fmt.Sprint(i), ClientNonce: fmt.Sprintf("page-history-%d", i)})
		if err != nil {
			t.Fatal(err)
		}
	}
	batch, err := s.PageHistory(p.ID, 0)
	if err != nil || len(batch) != 50 || batch[0].Revision != 55 || batch[49].Revision != 6 {
		t.Fatal("history page", err)
	}
	older, err := s.PageHistory(p.ID, 6)
	if err != nil || len(older) != 5 || older[4].Revision != 1 {
		t.Fatal("history cursor", err)
	}
	if _, err := s.Page(p.ID, 56); !errors.Is(err, ErrNotFound) {
		t.Fatal("invented revision", err)
	}
	for i := 1; i < MaxPages; i++ {
		in := pageInput()
		in.ClientNonce = fmt.Sprintf("page-limit-%d", i)
		if _, _, err := s.SavePage(in); err != nil {
			t.Fatal(err)
		}
	}
	in.ClientNonce = "page-over-limit"
	if _, _, err := s.SavePage(in); !errors.Is(err, ErrPageLimit) {
		t.Fatal("page count bound", err)
	}
}

func TestPageFailedWriteAndReadOnlyDoNotPublishRevision(t *testing.T) {
	s, dir := pageStore(t)
	p, _, err := s.SavePage(pageInput())
	if err != nil {
		t.Fatal(err)
	}
	in := PageInput{ID: p.ID, BaseRevision: 1, Title: p.Title, Text: "failed update", ClientNonce: "page-failed-fixture"}
	if err := s.file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SavePage(in); !errors.Is(err, ErrUnavailable) {
		t.Fatal("failed write acknowledged", err)
	}
	if current, _ := s.Page(p.ID, 0); current != p {
		t.Fatal("failed write changed current page")
	}
	if _, found := s.PageReceipt(in.ClientNonce); found {
		t.Fatal("failed write published a receipt")
	}
	s.Close()
	ro, err := OpenReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	if _, _, err := ro.SavePage(in); !errors.Is(err, ErrReadOnly) {
		t.Fatal("read-only mutation", err)
	}
	if current, _ := ro.Page(p.ID, 0); current != p {
		t.Fatal("read-only restore lost the durable page")
	}
}

func TestPageReplayRejectsForgedRecordsAndEventCopiesAreIndependent(t *testing.T) {
	s, _ := pageStore(t)
	if _, _, err := s.SavePage(pageInput()); err != nil {
		t.Fatal(err)
	}
	all, err := s.Events(0)
	if err != nil {
		t.Fatal(err)
	}
	e := all[len(all)-1]
	before, _ := pageStore(t)
	if !validReplay(before, e) {
		t.Fatal("valid page event rejected")
	}
	for _, mutate := range []func(*PageChange){
		func(p *PageChange) { p.Page.Revision++ }, func(p *PageChange) { p.Page.Text = "tampered" },
		func(p *PageChange) { p.Page.SHA256 = strings.Repeat("0", 64) }, func(p *PageChange) { p.Page.Title = "changed" },
		func(p *PageChange) { p.Page.ID = "bad" }, func(p *PageChange) { p.Page.SavedAt = "bad" },
	} {
		change := *e.Aurora.Page
		mutate(&change)
		forged := Event{Type: "aurora.page", Sequence: e.Sequence, Aurora: &AuroraRecord{Page: &change}}
		if validReplay(before, forged) {
			t.Fatal("forged page accepted")
		}
	}
	scoped := s.scopedEventLocked(identity.VerifiedPrincipal{ActorID: "other-human"}, e)
	if len(scoped.Event) != 0 {
		t.Fatal("private owner page published to a different actor")
	}
	original := *e.Aurora.Page
	e.Aurora.Page.Page.Text = "caller mutation"
	e.Aurora.Page.Input.Text = "caller input mutation"
	again, _ := s.Events(0)
	if !reflect.DeepEqual(*again[len(again)-1].Aurora.Page, original) {
		t.Fatal("returned event aliases the journal projection")
	}
}

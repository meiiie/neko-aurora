package ledger

import (
	"encoding/json"
	"nekobot.local/native/internal/identity"
	"strings"
	"testing"
	"time"
)

func TestFixtureMemberExportMatchesSourceRevocationAcrossReplay(t *testing.T) {
	s, a, p, now := fixtureLedger(t)
	c := fixtureChannel(t, s, p["owner"])
	w, _, err := s.AcceptFor(p["bob"], Input{BotID: "bot-meiiie", ClientNonce: "member-export-source", Text: "MEMBER_EXPORT_REVOKED_INPUT_FIXTURE", Lane: "foreground"}, SubmissionScope{ChannelID: c.ID, ExpectedChannelRevision: &c.Revision})
	if err != nil {
		t.Fatal(err)
	}
	w = fixtureAdmit(t, s, w)
	if err = s.AppendText(w.RequestID, w.Epoch, "MEMBER_EXPORT_REVOKED_REPLY_FIXTURE"); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(31 * time.Second)
	if a.Valid(p["bob"]) {
		t.Fatal("fixture sender session did not expire")
	}
	ticket, err := a.BootstrapTicket("fixture-reader", "reader-device")
	if err != nil {
		t.Fatal(err)
	}
	_, reader, err := a.Exchange(ticket)
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"json", "text"} {
		raw, err := s.ScopedExportHistory(reader, c.ID, "", format)
		if err != nil || !strings.Contains(string(raw), "MEMBER_EXPORT_REVOKED_REPLY_FIXTURE") {
			t.Fatal("completed history was removed by sender session expiry", format, err)
		}
	}
	dir := s.dir
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	legacy, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = legacy.SetSourceAccess(AccessInput{RequestID: w.RequestID, ExpectedRevision: 0, Granted: false, ClientNonce: "member-export-source-revoke"}); err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if err = legacy.Close(); err != nil {
		t.Fatal(err)
	}
	check := func(label string, current *Store) {
		t.Helper()
		current.ConfigureIdentity(a)
		snapshot, err := current.ScopedSnapshot(reader)
		if err != nil {
			t.Fatal(err)
		}
		events, err := current.ScopedEvents(reader, 0)
		if err != nil {
			t.Fatal(err)
		}
		sync, err := current.ScopedLocalSync(reader, LocalSyncScope{BotID: w.BotID, ChannelID: c.ID}, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, projection := range []any{snapshot, events, sync} {
			raw, _ := json.Marshal(projection)
			if strings.Contains(string(raw), "MEMBER_EXPORT_REVOKED_REPLY_FIXTURE") || strings.Contains(string(raw), w.Text) {
				t.Fatal("member projection retained revoked content", label)
			}
		}
		for _, format := range []string{"json", "text"} {
			raw, err := current.ScopedExportHistory(reader, c.ID, "", format)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "MEMBER_EXPORT_REVOKED_REPLY_FIXTURE") || strings.Contains(string(raw), w.Text) {
				t.Fatal("member export retained content hidden by other projections", label, format)
			}
		}
		ownerRaw, err := current.ExportSharedHistory(c.ID, "", "json")
		if err != nil || !strings.Contains(string(ownerRaw), "MEMBER_EXPORT_REVOKED_REPLY_FIXTURE") {
			t.Fatal("owner historical export semantics changed", label, err)
		}
	}
	current, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	check("current", current)
	current.Close()
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	check("replay", reopened)
	if !identity.ValidPrincipal(reader) {
		t.Fatal("reader fixture malformed")
	}
}

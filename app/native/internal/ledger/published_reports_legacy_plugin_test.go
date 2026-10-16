package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishedReportLegacyPluginIdentityCollision(t *testing.T) {
	dir := t.TempDir()
	manifest := PluginManifest{Name: "Legacy external report descriptor", Version: "0.1.0", License: "fixture-only", Tools: []PluginTool{{Name: "legacy.report", Description: "Unverified fixture descriptor", InputSchema: json.RawMessage(`{"type":"object","properties":{},"required":[],"additionalProperties":false}`)}}}
	in, err := normalizePluginInput(PluginInput{Version: 1, ID: "neko-published-reports", Action: "import", ClientNonce: "legacy-reader-import", Manifest: &manifest})
	if err != nil {
		t.Fatal(err)
	}
	old := PluginRecord{ID: in.ID, Revision: 1, Publisher: "unverified-local-descriptor", Status: "manifest_only_disabled", PluginManifest: *in.Manifest}
	r := record{Version: 1, Event: Event{Sequence: 1, Type: "plugin.lifecycle", Plugin: &PluginEvent{Version: 1, Input: in, Plugin: old}}}
	r.Digest, _ = digestRecord(r)
	journal, _ := json.Marshal(r)
	journal = append(journal, '\n')
	if source := os.Getenv("NEKO_LEGACY_READER_JOURNAL"); source != "" {
		journal, err = os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "transactions.jsonl")
	if err = os.WriteFile(path, journal, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal("legitimate historical disabled descriptor prevents Open", err)
	}
	defer func() { s.Close() }()
	p, found := s.plugin("neko-published-reports")
	if !found || p.TrustedBuiltin || p.Publisher != "unverified-local-descriptor" || p.Status != "manifest_only_disabled" || p.Revision != 1 {
		t.Fatal("historical descriptor silently upgraded")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(journal, after) {
		t.Fatal("historical journal overwritten")
	}
	c := sharedChannel(t, s, "Legacy shadow destination", "legacy-shadow-channel")
	w, _, err := s.Accept(Input{BotID: "bot-meiiie", ClientNonce: "legacy-shadow-private", Text: "Write report result", Lane: "foreground"})
	if err != nil {
		t.Fatal(err)
	}
	w = startPermissionWork(t, s, w)
	if _, err = s.ExecuteTool(w.RequestID, w.Epoch, invocation("Legacy shadow report file")); err != nil {
		t.Fatal(err)
	}
	if err = s.AppendText(w.RequestID, w.Epoch, "Legacy shadow published report reply"); err != nil {
		t.Fatal(err)
	}
	if err = s.Finish(w.RequestID, w.Epoch, "completed", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.DeliverReport(reportSubmission(c, "legacy-shadow-delivery")); err != nil {
		t.Fatal(err)
	}
	consumer := publishedWork(t, s, c, "legacy-shadow-consumer")
	if !s.contextAllowed(consumer) || len(consumer.Context.PublishedReports) != 1 || consumer.Context.PublishedReports[0].Reply == "" || pinnedTool(consumer, "report.read_artifact") || hasPublishedArtifacts(consumer) {
		t.Fatal("shadow descriptor granted reader or suppressed safe reply")
	}
	if _, _, err = s.ControlPlugin(PluginInput{Version: 1, ID: p.ID, Action: "enable", ExpectedRevision: p.Revision, ClientNonce: "legacy-shadow-enable"}); !errors.Is(err, ErrUnsupported) {
		t.Fatal("unverified descriptor enabled", err)
	}
	for _, action := range []string{"update", "remove"} {
		current, _ := s.plugin(p.ID)
		input := PluginInput{Version: 1, ID: p.ID, Action: action, ExpectedRevision: current.Revision, ClientNonce: "legacy-shadow-" + action}
		if action == "update" {
			copy := current.PluginManifest
			copy.Name = "Updated old unverified descriptor"
			input.Manifest = &copy
		}
		input, err = normalizePluginInput(input)
		if err != nil {
			t.Fatal(err)
		}
		next, err := s.pluginMutation(input)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.commit(Event{Type: "plugin.lifecycle", Plugin: &PluginEvent{Version: 1, Input: input, Plugin: next}}); err != nil {
			t.Fatal(err)
		}
		s.Close()
		s, err = Open(dir)
		if err != nil {
			t.Fatal("legacy descriptor lifecycle replay", action, err)
		}
		got, _ := s.plugin(p.ID)
		if got.TrustedBuiltin || got.Revision != next.Revision || got.Status != next.Status {
			t.Fatal("legacy lifecycle rebound builtin", action)
		}
	}
	reserved, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer reserved.Close()
	if _, _, err = reserved.ControlPlugin(in); !errors.Is(err, ErrConflict) {
		t.Fatal("current reserved reader import accepted", err)
	}
}

package ledger

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestAdapterInstallationCatalogExactReadonlyProjection(t *testing.T) {
	s, r := executorFixture(t)
	c := sharedChannel(t, s, "Same visible name", "install-catalog-channel")
	p, _, e := s.SaveProject(ProjectInput{Name: "Fixture project", Goal: "PRIVATE-GOAL-CANARY", ChannelIDs: []string{c.ID}, ClientNonce: "installation-project-nonce"})
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = s.Accept(Input{BotID: "bot-meiiie", Text: "PRIVATE-WORK-CANARY", Lane: "foreground", ClientNonce: "installation-private-message"})
	if e != nil {
		t.Fatal(e)
	}
	before := s.Snapshot().Sequence
	out, e := s.AdapterInstallationCatalog()
	if e != nil {
		t.Fatal(e)
	}
	pin, e := r.ReadInstallation()
	if e != nil || out.Installation != pin || out.Version != 1 || out.OwnerUserID != "local-owner" || len(out.Projects) != 1 || out.Projects[0].ID != p.ID {
		t.Fatal("wrong identity", e)
	}
	wire, _ := json.Marshal(out)
	for _, secret := range []string{"PRIVATE-", "goal", "work", "context", "bootId", "lease", "account", "files", "path"} {
		if strings.Contains(string(wire), secret) {
			t.Fatal("private data exposed", secret)
		}
	}
	out.Projects[0].ChannelIDs[0] = "caller-mutation"
	fresh, e := s.AdapterInstallationCatalog()
	if e != nil || s.Snapshot().Sequence != before || !reflect.DeepEqual(fresh.Projects[0].ChannelIDs, p.ChannelIDs) {
		t.Fatal("read mutated state", e)
	}
	if _, _, e = r.Revoke(pin, "installation-catalog-revoke"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.AdapterInstallationCatalog(); e == nil {
		t.Fatal("revoked installation exposed")
	}
}
func TestAdapterInstallationCatalogMissingRegistryFailsClosed(t *testing.T) {
	s, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, e = s.AdapterInstallationCatalog(); e == nil {
		t.Fatal("unconfigured installation exposed")
	}
	s.ConfigureExecutor(nil)
	if _, e = s.AdapterInstallationCatalog(); e == nil {
		t.Fatal("nil installation exposed")
	}
}

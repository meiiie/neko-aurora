package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFixtureBootstrapDoesNotOverwriteAndHasFreshIssuer(t *testing.T) {
	dir := t.TempDir()
	config, output := filepath.Join(dir, "fixture.json"), filepath.Join(dir, "private-tickets.json")
	raw := []byte(`{"version":1,"fixtureOnly":true,"authorityId":"fixture-office","spaceId":"local-space","sessionTTLSeconds":300,"actors":[{"actorId":"local-owner","deviceId":"device-alice","deviceRevision":1},{"actorId":"fixture-bob","deviceId":"device-bob","deviceRevision":1}]}`)
	if err := os.WriteFile(config, raw, 0600); err != nil {
		t.Fatal(err)
	}
	a, c, err := loadFixtureIdentity(config, output)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := loadFixtureIdentity(config, output)
	if err != nil || a.IssuerEpoch() == b.IssuerEpoch() {
		t.Fatal("fresh authority epoch required")
	}
	if err = writeFixtureTickets(output, a, c); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Sessions []struct{ ActorID, DeviceID, Ticket string }
	}
	if json.Unmarshal(saved, &result) != nil || len(result.Sessions) != 2 {
		t.Fatal("private fixture tickets missing")
	}
	for _, row := range result.Sessions {
		bearer, p, err := a.Exchange(row.Ticket)
		if err != nil || p.ActorID != row.ActorID || p.DeviceID != row.DeviceID || bearer == p.SessionID || bearer == row.Ticket {
			t.Fatal("ticket exchange not identity bound")
		}
		if _, _, err := a.Exchange(row.Ticket); err == nil {
			t.Fatal("ticket reused")
		}
	}
	if _, _, err := loadFixtureIdentity(config, output); err == nil {
		t.Fatal("existing output accepted")
	}
	if writeFixtureTickets(output, b, c) == nil {
		t.Fatal("existing output overwritten")
	}
	after, _ := os.ReadFile(output)
	if string(after) != string(saved) {
		t.Fatal("private output changed")
	}
}

func TestFixtureConfigFailsClosedBeforeBootstrap(t *testing.T) {
	dir := t.TempDir()
	path, output := filepath.Join(dir, "fixture.json"), filepath.Join(dir, "private-tickets.json")
	for _, raw := range []string{
		`{"version":1,"fixtureOnly":false}`,
		`{"version":1,"version":1,"fixtureOnly":true}`,
		`{"version":1,"fixtureOnly":true,"issuerEpoch":1}`,
		`{"version":1,"fixtureOnly":true,"authorityId":"fixture-office","spaceId":"foreign","sessionTTLSeconds":300,"actors":[]}`,
	} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := loadFixtureIdentity(path, output); err == nil {
			t.Fatal("invalid fixture config accepted")
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("invalid config created output")
		}
	}
}

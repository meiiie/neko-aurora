package runtime

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProfileCatalogRejectsAuthorityAndCredentialExpansion(t *testing.T) {
	valid := []byte(`{"version":1,"profiles":[{"id":"fixture","provider":"openai_compat","model":"fixture-model","authKind":"api_key","active":true,"configurationStatus":"configured","credentialReadiness":"not_checked","profileRevision":"pr1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","providerOrigin":"http://127.0.0.1:9123","bridgeTransport":"supported"}]}`)
	if _, err := parseProfileCatalog(valid, true, "http://127.0.0.1:9123"); err != nil {
		t.Fatal("synthetic public catalog refused", err)
	}
	var object map[string]any
	if err := json.Unmarshal(valid, &object); err != nil {
		t.Fatal(err)
	}
	profile := object["profiles"].([]any)[0].(map[string]any)
	for _, test := range []struct {
		field string
		value any
	}{
		{"apiKey", "fixture-only-secret"},
		{"credentialReadiness", "verified"},
		{"providerOrigin", "https://fixture-user@fixture.example"},
		{"providerOrigin", "https://fixture.example/secret-path?fixture-token=value"},
		{"profileRevision", "unkeyed-secret-digest"},
	} {
		old, existed := profile[test.field]
		profile[test.field] = test.value
		raw, _ := json.Marshal(object)
		if _, err := parseProfileCatalog(raw, true, "http://127.0.0.1:9123"); err == nil {
			t.Errorf("expanded public catalog accepted: %s", test.field)
		}
		if existed {
			profile[test.field] = old
		} else {
			delete(profile, test.field)
		}
	}
	duplicate := append(append([]byte(nil), valid[:len(valid)-1]...), []byte(`,"version":1}`)...)
	if _, err := parseProfileCatalog(duplicate, true, "http://127.0.0.1:9123"); err == nil {
		t.Fatal("duplicate catalog keys accepted")
	}
}

func TestProfileCatalogListsLocalPresetsWithoutExpandingFixtureExecution(t *testing.T) {
	profile := map[string]any{"id": "local", "provider": "openai_compat", "model": "local-fixture", "authKind": "api_key", "active": false, "configurationStatus": "configured", "credentialReadiness": "not_checked", "profileRevision": "pr1_" + strings.Repeat("a", 64), "providerOrigin": "http://localhost:11434", "bridgeTransport": "supported"}
	raw, _ := json.Marshal(map[string]any{"version": 1, "profiles": []any{profile}})
	report, err := parseProfileCatalog(raw, true, "http://127.0.0.1:9123")
	if err != nil || len(report.Profiles) != 1 || report.Profiles[0].BridgeTransport != "unsupported" {
		t.Fatal("harmless preset must remain visible but ungranted", err)
	}
}

func TestProfileCatalogOriginAndCountBounds(t *testing.T) {
	for _, origin := range []string{"http://external.example", "https://fixture.example?key=x", "https://fixture.example/#token", "https://user:fixture@fixture.example", "file:///fixture"} {
		if validProfileOrigin(origin, false) {
			t.Errorf("unapproved network origin accepted: %s", origin)
		}
	}
	if !validProfileOrigin("https://api.openai.com", false) || !validProfileOrigin("http://127.0.0.1:9123", true) {
		t.Fatal("bounded origins refused")
	}
}

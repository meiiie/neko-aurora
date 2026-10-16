package runtime

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func authV2Catalog(t *testing.T, environment bool) map[string]any {
	t.Helper()
	now := time.Now().UnixMilli()
	p := map[string]any{"id": "fixture-auth", "provider": "kimi", "model": "fixture-model", "authKind": "kimi_oauth", "active": true, "configurationStatus": "configured", "credentialReadiness": "not_checked", "profileRevision": "pr2_" + strings.Repeat("a", 64), "providerOrigin": "http://127.0.0.1:9123", "bridgeTransport": "supported", "credentialSource": "oauth_cache", "authPolicy": "cached_token_no_refresh", "credentialExpiresAtUnixMs": now + 240000}
	r := map[string]any{"version": 2, "profiles": []any{p}}
	if environment {
		p["provider"], p["authKind"], p["credentialSource"], p["authPolicy"] = "openai_compat", "api_key", "process_env", "static_key"
		p["credentialSnapshotId"] = "cs1_" + strings.Repeat("b", 32)
		r["sourceSnapshot"] = map[string]any{"id": p["credentialSnapshotId"], "createdAtUnixMs": now, "expiresAtUnixMs": now + 300000}
	}
	return r
}

func TestAuthV2CatalogAcceptsBoundedCachedAndEnvironmentPolicy(t *testing.T) {
	for _, env := range []bool{false, true} {
		raw, err := json.Marshal(authV2Catalog(t, env))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parseProfileCatalog(raw, true, "http://127.0.0.1:9123"); err != nil {
			t.Errorf("bounded v2 fixture refused (environment=%v): %v", env, err)
		}
	}
}

func TestAuthV2CatalogRejectsRefreshAndUnboundEnvironment(t *testing.T) {
	for _, field := range []string{"authPolicy", "credentialSnapshotId", "credentialExpiresAtUnixMs", "apiKey"} {
		r := authV2Catalog(t, true)
		p := r["profiles"].([]any)[0].(map[string]any)
		switch field {
		case "authPolicy":
			p[field] = "refresh_automatically"
		case "credentialSnapshotId":
			delete(p, field)
		case "credentialExpiresAtUnixMs":
			p[field] = uint64(1 << 53)
		case "apiKey":
			p[field] = "fixture-only-private-value"
		}
		raw, _ := json.Marshal(r)
		if _, err := parseProfileCatalog(raw, true, "http://127.0.0.1:9123"); err == nil {
			t.Errorf("v2 catalog expanded %s accepted", field)
		}
	}
}

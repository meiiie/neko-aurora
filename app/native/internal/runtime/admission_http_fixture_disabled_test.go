//go:build !neko_admission_http_fixture

package runtime

import (
	"encoding/json"
	"nekobot.local/native/internal/ledger"
	"os"
	"path/filepath"
	"testing"
)

func TestAdmissionHTTPFixtureAbsentTagCannotActivate(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "admission-observation.json")
	config := filepath.Join(dir, "config.json")
	raw, _ := json.Marshal(map[string]any{"version": 1, "clientNonce": "owned-http-default-negative", "stage": "pre-bind", "observationPath": marker})
	if err := os.WriteFile(config, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NEKO_ADMISSION_HTTP_FIXTURE_CONFIG", config)
	admissionHTTPFixtureObserve("pre-bind", ledger.Work{ClientNonce: "owned-http-default-negative"}, ledger.CoreBinding{}, 1)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("default build activated fixture env", err)
	}
}

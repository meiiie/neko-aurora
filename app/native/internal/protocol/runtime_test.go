package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProfileBridgePinsSeparateHistoryFromExecution(t *testing.T) {
	var pins map[string]struct {
		SHA256               string `json:"sha256"`
		ProfileBridgeVersion int    `json:"profileBridgeVersion"`
		HistoricalReplayOnly bool   `json:"historicalReplayOnly"`
	}
	if err := json.Unmarshal(runtimePins, &pins); err != nil {
		t.Fatal(err)
	}
	current := pins["windows_x64"]
	if current.ProfileBridgeVersion != 2 || current.HistoricalReplayOnly || !CoreProfileBridgeAllowed(current.SHA256) {
		t.Fatal("current compatible runtime must be executable")
	}
	history := pins["windows_x64_previous_profile_bridge_credential_echo"]
	if !history.HistoricalReplayOnly || !CoreProfileBridgeKnown(history.SHA256) || CoreProfileBridgeAllowed(history.SHA256) {
		t.Fatal("historical profile runtime must remain readable without fresh authority")
	}
	for _, digest := range []string{"", strings.Repeat("0", 64), pins["windows_x64_previous_principal_device"].SHA256} {
		if CoreProfileBridgeAllowed(digest) || CoreProfileBridgeKnown(digest) {
			t.Fatal("runtime without the profile contract must stay denied")
		}
	}
}

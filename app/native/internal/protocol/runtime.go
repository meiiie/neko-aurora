package protocol

import (
	_ "embed"
	"encoding/json"
)

//go:embed core-runtimes-v1.json
var runtimePins []byte

func CoreProfileBridgeAllowed(digest string) bool {
	return profileBridgePin(digest, true)
}

func CoreProfileBridgeKnown(digest string) bool {
	return profileBridgePin(digest, false)
}

func CoreProfileAuthBridgeAllowed(digest string) bool {
	var pins map[string]struct {
		SHA256               string `json:"sha256"`
		ProfileBridgeVersion int    `json:"profileBridgeVersion"`
		HistoricalReplayOnly bool   `json:"historicalReplayOnly"`
	}
	if json.Unmarshal(runtimePins, &pins) != nil {
		return false
	}
	for _, pin := range pins {
		if pin.SHA256 == digest && pin.ProfileBridgeVersion == 2 && !pin.HistoricalReplayOnly {
			return true
		}
	}
	return false
}

func profileBridgePin(digest string, fresh bool) bool {
	var pins map[string]struct {
		SHA256               string `json:"sha256"`
		ProfileBridgeVersion int    `json:"profileBridgeVersion"`
		HistoricalReplayOnly bool   `json:"historicalReplayOnly"`
	}
	if json.Unmarshal(runtimePins, &pins) != nil {
		return false
	}
	for _, p := range pins {
		if p.SHA256 == digest && (p.ProfileBridgeVersion == 1 || p.ProfileBridgeVersion == 2) && (!fresh || !p.HistoricalReplayOnly) {
			return true
		}
	}
	return false
}

func CoreRequesterAdmission(digest string) bool {
	var pins map[string]struct {
		SHA256           string `json:"sha256"`
		ManifestVersions []int  `json:"manifestVersions"`
	}
	if json.Unmarshal(runtimePins, &pins) != nil {
		return false
	}
	for _, p := range pins {
		if p.SHA256 != digest {
			continue
		}
		for _, version := range p.ManifestVersions {
			if version == 2 {
				return true
			}
		}
	}
	return false
}

func CoreContextAdmission(digest string) bool {
	var pins map[string]struct {
		SHA256          string `json:"sha256"`
		ContextProtocol int    `json:"contextProtocol"`
	}
	if json.Unmarshal(runtimePins, &pins) != nil {
		return false
	}
	for _, p := range pins {
		if p.SHA256 == digest && p.ContextProtocol == 1 {
			return true
		}
	}
	return false
}

func CoreRuntimeAllowed(digest string) bool {
	var pins map[string]struct {
		SHA256 string `json:"sha256"`
	}
	if json.Unmarshal(runtimePins, &pins) != nil {
		return false
	}
	for _, pin := range pins {
		if pin.SHA256 == digest {
			return true
		}
	}
	return false
}
func CorePriorReceiptRetry(digest string) bool {
	var pins map[string]struct {
		SHA256            string `json:"sha256"`
		PriorReceiptRetry bool   `json:"priorReceiptRetry"`
	}
	if json.Unmarshal(runtimePins, &pins) != nil {
		return false
	}
	for _, pin := range pins {
		if pin.SHA256 == digest && pin.PriorReceiptRetry {
			return true
		}
	}
	return false
}

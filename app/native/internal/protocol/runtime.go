package protocol

import (
	_ "embed"
	"encoding/json"
)

//go:embed core-runtimes-v1.json
var runtimePins []byte

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

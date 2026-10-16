package protocol

import "encoding/json"

type CoreDiscoveryPin struct {
	Version              string `json:"version"`
	SHA256               string `json:"sha256"`
	ContextProtocol      int    `json:"contextProtocol"`
	ManifestVersions     []int  `json:"manifestVersions"`
	HistoricalReplayOnly bool   `json:"historicalReplayOnly"`
}

func CurrentWindowsCoreDiscoveryPin() CoreDiscoveryPin {
	var pins map[string]CoreDiscoveryPin
	if json.Unmarshal(runtimePins, &pins) != nil {
		return CoreDiscoveryPin{}
	}
	p := pins["windows_x64"]
	if p.HistoricalReplayOnly || p.ContextProtocol != 1 {
		return CoreDiscoveryPin{}
	}
	p.ManifestVersions = append([]int(nil), p.ManifestVersions...)
	return p
}

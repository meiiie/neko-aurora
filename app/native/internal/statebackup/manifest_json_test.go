package statebackup

import (
	"errors"
	"strings"
	"testing"
)

func TestManifestJSONRejectsAmbiguousOrUnboundedInputs(t *testing.T) {
	cases := map[string][]byte{
		"duplicate":          []byte(`{"version":1,"version":2}`),
		"case_duplicate":     []byte(`{"version":1,"Version":2}`),
		"nested_duplicate":   []byte(`{"files":[{"path":"a","path":"b"}]}`),
		"unknown":            []byte(`{"unknown":1}`),
		"extra_document":     []byte(`{"version":1} {}`),
		"invalid_utf8":       {'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'},
		"unpaired_surrogate": []byte(`{"snapshotId":"\ud800"}`),
		"fraction":           []byte(`{"version":1.0}`),
		"exponent":           []byte(`{"version":1e0}`),
		"negative_zero":      []byte(`{"version":-0}`),
		"unsafe_integer":     []byte(`{"sequence":9007199254740992}`),
		"depth":              []byte(strings.Repeat("[", 34) + "1" + strings.Repeat("]", 34)),
		"oversized":          []byte(strings.Repeat(" ", int(maxManifest)+1)),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			var manifest Manifest
			if !errors.Is(decodeManifestJSON(raw, &manifest), ErrInvalid) {
				t.Fatal("accepted ambiguous or unbounded manifest")
			}
		})
	}
}

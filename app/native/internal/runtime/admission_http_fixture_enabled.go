//go:build neko_admission_http_fixture

package runtime

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"nekobot.local/native/internal/executors"
	"nekobot.local/native/internal/ledger"
	"nekobot.local/native/internal/protocol"
)

type admissionHTTPFixtureConfig struct {
	Version         int    `json:"version"`
	ClientNonce     string `json:"clientNonce"`
	Stage           string `json:"stage"`
	ObservationPath string `json:"observationPath"`
}

var admissionHTTPFixtureNonce = regexp.MustCompile(`^owned-http-[A-Za-z0-9_-]{8,100}$`)

func readAdmissionHTTPFixtureConfig(path string) (admissionHTTPFixtureConfig, bool) {
	var out admissionHTTPFixtureConfig
	if !filepath.IsAbs(path) {
		return out, false
	}
	f, err := os.Open(path)
	if err != nil {
		return out, false
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(raw) > 4096 {
		return out, false
	}
	raw, err = protocol.Canonical(raw)
	if err != nil {
		return out, false
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if dec.Decode(&out) != nil || out.Version != 1 || !admissionHTTPFixtureNonce.MatchString(out.ClientNonce) || (out.Stage != "pre-bind" && out.Stage != "post-durable-pre-ack") || !filepath.IsAbs(out.ObservationPath) || filepath.Clean(filepath.Dir(out.ObservationPath)) != filepath.Clean(filepath.Dir(path)) || filepath.Base(out.ObservationPath) != "admission-observation.json" {
		return admissionHTTPFixtureConfig{}, false
	}
	return out, true
}

func admissionHTTPFixtureMatches(c admissionHTTPFixtureConfig, stage string, w ledger.Work, b ledger.CoreBinding) bool {
	return c.Stage == stage && c.ClientNonce == w.ClientNonce && w.ExecutorPolicyVersion == 1 && w.ExecutorPin != nil && w.CurrentAttemptLease != nil && b.ExecutorProof != nil && executors.MatchRuntimeProof(*w.CurrentAttemptLease, *b.ExecutorProof) && b.CoreSHA256 == w.ExecutorPin.CoreSHA256 && b.NativeProcessID != 0 && ledger.ValidProcessGeneration(b.NativeProcessGeneration) && w.ContextProtocolVersion == 1 && w.Context != nil && b.ContextAdmission != nil && b.ContextAdmission.Version == 1 && b.ContextAdmission.SessionID == b.SessionID && b.ContextAdmission.ContextPinSHA256 == w.Context.SHA256 && b.ContextAdmission.ManifestSHA256 == w.RuntimeContextSHA256
}

func admissionHTTPFixtureObserve(stage string, w ledger.Work, b ledger.CoreBinding, sequence uint64) {
	c, ok := readAdmissionHTTPFixtureConfig(os.Getenv("NEKO_ADMISSION_HTTP_FIXTURE_CONFIG"))
	if !ok || !admissionHTTPFixtureMatches(c, stage, w, b) {
		return
	}
	raw, err := json.Marshal(map[string]any{"version": 1, "kind": "non_authority_fixture_observation", "stage": stage, "requestId": w.RequestID, "attemptId": w.AttemptID, "clientNonce": w.ClientNonce, "sequence": sequence, "timeNs": strconv.FormatInt(time.Now().UnixNano(), 10), "binding": b})
	if err != nil || len(raw) > 16384 {
		return
	}
	f, err := os.OpenFile(c.ObservationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return
	}
	n, err := f.Write(raw)
	if err == nil && n == len(raw) {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || n != len(raw) || closeErr != nil {
		return
	}
	// Fixture builds only: pause the genuine event, never create admission or stop proof.
	<-time.After(15 * time.Second)
}

package providerexec

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"nekobot.local/native/internal/protocol"
)

var ErrSavedAuthority = errors.New("saved account authority unavailable")

const savedSafeInt int64 = 9007199254740991

var savedHex = regexp.MustCompile(`^[a-f0-9]{64}$`)
var savedID = regexp.MustCompile(`^[A-Za-z0-9_-]{3,128}$`)
var savedAccountID = regexp.MustCompile(`^account-[a-f0-9]{32}$`)
var savedBoot = regexp.MustCompile(`^executor-boot-[a-f0-9]{32}$`)

type SavedTaskLimits struct {
	MaxInputBytes   int64 `json:"maxInputBytes"`
	MaxOutputTokens int64 `json:"maxOutputTokens"`
	MaxRequests     int64 `json:"maxRequests"`
	MaxSteps        int64 `json:"maxSteps"`
	MaxSeconds      int64 `json:"maxSeconds"`
}
type SavedTaskBinding struct {
	FixtureOnly          bool            `json:"fixtureOnly"`
	AccountID            string          `json:"accountId"`
	AccountRevision      int64           `json:"accountRevision"`
	CredentialGeneration int64           `json:"credentialGeneration"`
	Provider             string          `json:"provider"`
	Endpoint             string          `json:"endpoint"`
	Model                string          `json:"model"`
	RequestID            string          `json:"requestId"`
	AttemptID            string          `json:"attemptId"`
	Epoch                int64           `json:"epoch"`
	LeaseFence           int64           `json:"leaseFence"`
	ContextSHA256        string          `json:"contextSHA256"`
	ConsentID            string          `json:"consentId"`
	ConsentRevision      int64           `json:"consentRevision"`
	ExpiresAtUnixMS      int64           `json:"expiresAtUnixMS"`
	Limits               SavedTaskLimits `json:"limits"`
}

func savedCanonical(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, ErrSavedAuthority
	}
	out, err := protocol.Canonical(raw)
	if err != nil {
		return nil, ErrSavedAuthority
	}
	return out, nil
}
func exactSavedKeys(raw []byte, keys []string) bool {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || len(obj) != len(keys) {
		return false
	}
	for _, key := range keys {
		if _, ok := obj[key]; !ok {
			return false
		}
	}
	return true
}
func savedText(s string, max int) bool {
	if s == "" || len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 32 || r == 127 || r == 0xfffd {
			return false
		}
	}
	return true
}
func savedPositive(n int64, max int64) bool { return n >= 1 && n <= max }
func (b SavedTaskBinding) valid() bool {
	if !savedAccountID.MatchString(b.AccountID) || !savedID.MatchString(b.RequestID) || !savedID.MatchString(b.AttemptID) || !savedID.MatchString(b.ConsentID) || !savedHex.MatchString(b.ContextSHA256) || !savedText(b.Endpoint, 512) || !savedText(b.Model, 120) || (b.Provider != "openai-compatible" && b.Provider != "anthropic") {
		return false
	}
	for _, n := range []int64{b.AccountRevision, b.CredentialGeneration, b.Epoch, b.LeaseFence, b.ConsentRevision, b.ExpiresAtUnixMS} {
		if !savedPositive(n, savedSafeInt) {
			return false
		}
	}
	l := b.Limits
	if !savedPositive(l.MaxInputBytes, 64000) || !savedPositive(l.MaxOutputTokens, 4096) || !savedPositive(l.MaxRequests, 8) || !savedPositive(l.MaxSteps, 6) || !savedPositive(l.MaxSeconds, 600) {
		return false
	}
	u, err := url.Parse(b.Endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(b.Endpoint, "#") || u.Opaque != "" {
		return false
	}
	if !savedCanonicalEndpoint(b.Endpoint, u) {
		return false
	}
	if b.FixtureOnly {
		return u.Scheme == "http" && u.Hostname() == "127.0.0.1" && u.Port() != "" && u.Port() != "0"
	}
	return u.Scheme == "https"
}
func ParseSavedTaskBinding(raw []byte) (SavedTaskBinding, error) {
	var b SavedTaskBinding
	canonical, err := protocol.Canonical(raw)
	if err != nil {
		return b, ErrSavedAuthority
	}
	keys := []string{"fixtureOnly", "accountId", "accountRevision", "credentialGeneration", "provider", "endpoint", "model", "requestId", "attemptId", "epoch", "leaseFence", "contextSHA256", "consentId", "consentRevision", "expiresAtUnixMS", "limits"}
	if !exactSavedKeys(canonical, keys) {
		return b, ErrSavedAuthority
	}
	var obj map[string]json.RawMessage
	json.Unmarshal(canonical, &obj)
	if !bytes.Equal(obj["fixtureOnly"], []byte("true")) && !bytes.Equal(obj["fixtureOnly"], []byte("false")) {
		return b, ErrSavedAuthority
	}
	if !exactSavedKeys(obj["limits"], []string{"maxInputBytes", "maxOutputTokens", "maxRequests", "maxSteps", "maxSeconds"}) || json.Unmarshal(canonical, &b) != nil || !b.valid() {
		return SavedTaskBinding{}, ErrSavedAuthority
	}
	return b, nil
}
func SavedTaskBindingHash(b SavedTaskBinding) (string, error) {
	if !b.valid() {
		return "", ErrSavedAuthority
	}
	raw, err := savedCanonical(b)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}

// Preserve endpoint identity across the Go/Rust/Core boundary. These checks
// reject common WHATWG URL normalization rather than changing an authorized URL.
func savedCanonicalEndpoint(endpoint string, u *url.URL) bool {
	if !strings.HasPrefix(endpoint, u.Scheme+"://") || u.Path == "" || u.Hostname() != strings.ToLower(u.Hostname()) {
		return false
	}
	for _, c := range []byte(endpoint) {
		if c <= 32 || c >= 127 || strings.ContainsRune("\\\"<>`{}", rune(c)) {
			return false
		}
	}
	if strings.HasSuffix(u.Host, ":") {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 0 || n > 65535 || strconv.Itoa(n) != port || (u.Scheme == "https" && n == 443) || (u.Scheme == "http" && n == 80) {
			return false
		}
	}
	for _, segment := range strings.Split(u.EscapedPath(), "/") {
		decoded, err := url.PathUnescape(segment)
		if err != nil || decoded == "." || decoded == ".." {
			return false
		}
	}
	return true
}

package executors

import (
	"bytes"
	"encoding/json"
	"errors"
	"nekobot.local/native/internal/protocol"
	"strings"
)

const SafeInteger uint64 = 9007199254740991
const MaxLeaseMilliseconds uint64 = 600000
const MaxWireBytes = 1024

var ErrInvalid = errors.New("executor_invalid")
var ErrConflict = errors.New("executor_conflict")
var ErrUnavailable = errors.New("executor_unavailable")
var ErrExpired = errors.New("executor_lease_expired")

type Descriptor struct {
	HostSHA256         string `json:"hostSha256"`
	CoreSHA256         string `json:"coreSha256"`
	RootSHA256         string `json:"rootSha256"`
	CapabilitiesSHA256 string `json:"capabilitiesSha256"`
}
type InstallPin struct {
	Version    int    `json:"version"`
	ExecutorID string `json:"executorId"`
	Generation uint64 `json:"generation"`
	Descriptor
}
type TaskScope struct {
	RequestID       string `json:"requestId"`
	AttemptID       string `json:"attemptId"`
	WorkEpoch       uint64 `json:"workEpoch"`
	AuthoritySHA256 string `json:"authoritySha256"`
}
type AttemptLease struct {
	Version    int    `json:"version"`
	ID         string `json:"id"`
	ExecutorID string `json:"executorId"`
	Generation uint64 `json:"generation"`
	BootID     string `json:"bootId"`
	TaskScope
	Fence       uint64 `json:"fence"`
	IssuedAtMs  uint64 `json:"issuedAtMs"`
	ExpiresAtMs uint64 `json:"expiresAtMs"`
}
type RuntimeProof struct {
	Version    int    `json:"version"`
	ExecutorID string `json:"executorId"`
	Generation uint64 `json:"generation"`
	BootID     string `json:"bootId"`
	LeaseID    string `json:"leaseId"`
	TaskScope
	Fence uint64 `json:"fence"`
}

func positive(n uint64) bool { return n > 0 && n <= SafeInteger }
func hex(s string, size int) bool {
	if len(s) != size {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func id(s, prefix string) bool {
	return strings.HasPrefix(s, prefix) && hex(strings.TrimPrefix(s, prefix), 32)
}
func ValidDescriptor(d Descriptor) bool {
	return hex(d.HostSHA256, 64) && hex(d.CoreSHA256, 64) && hex(d.RootSHA256, 64) && hex(d.CapabilitiesSHA256, 64)
}
func ValidInstallPin(p InstallPin) bool {
	return p.Version == 1 && id(p.ExecutorID, "executor-") && positive(p.Generation) && ValidDescriptor(p.Descriptor)
}
func validScope(s TaskScope) bool {
	return id(s.RequestID, "req-") && id(s.AttemptID, "attempt-") && positive(s.WorkEpoch) && hex(s.AuthoritySHA256, 64)
}
func ValidLease(l AttemptLease) bool {
	return l.Version == 1 && id(l.ID, "executor-lease-") && id(l.ExecutorID, "executor-") && positive(l.Generation) && id(l.BootID, "executor-boot-") && validScope(l.TaskScope) && positive(l.Fence) && positive(l.IssuedAtMs) && positive(l.ExpiresAtMs) && l.ExpiresAtMs > l.IssuedAtMs && l.ExpiresAtMs-l.IssuedAtMs <= MaxLeaseMilliseconds
}
func ValidRuntimeProof(p RuntimeProof) bool {
	return p.Version == 1 && id(p.ExecutorID, "executor-") && positive(p.Generation) && id(p.BootID, "executor-boot-") && id(p.LeaseID, "executor-lease-") && validScope(p.TaskScope) && positive(p.Fence)
}
func strict(data []byte, out any) error {
	if len(data) == 0 || len(data) > MaxWireBytes {
		return ErrInvalid
	}
	canonical, e := protocol.Canonical(data)
	if e != nil {
		return ErrInvalid
	}
	dec := json.NewDecoder(bytes.NewReader(canonical))
	dec.DisallowUnknownFields()
	if e = dec.Decode(out); e != nil {
		return ErrInvalid
	}
	rendered, e := json.Marshal(out)
	if e != nil {
		return ErrInvalid
	}
	exact, e := protocol.Canonical(rendered)
	if e != nil || !bytes.Equal(exact, canonical) {
		return ErrInvalid
	}
	return nil
}
func ParseInstallPin(b []byte) (p InstallPin, e error) {
	e = strict(b, &p)
	if e == nil && !ValidInstallPin(p) {
		e = ErrInvalid
	}
	return
}
func ParseLease(b []byte) (l AttemptLease, e error) {
	e = strict(b, &l)
	if e == nil && !ValidLease(l) {
		e = ErrInvalid
	}
	return
}
func ParseRuntimeProof(b []byte) (p RuntimeProof, e error) {
	e = strict(b, &p)
	if e == nil && !ValidRuntimeProof(p) {
		e = ErrInvalid
	}
	return
}
func ProofFor(l AttemptLease) (RuntimeProof, error) {
	if !ValidLease(l) {
		return RuntimeProof{}, ErrInvalid
	}
	return RuntimeProof{Version: 1, ExecutorID: l.ExecutorID, Generation: l.Generation, BootID: l.BootID, LeaseID: l.ID, TaskScope: l.TaskScope, Fence: l.Fence}, nil
}
func MatchRuntimeProof(l AttemptLease, p RuntimeProof) bool {
	expected, e := ProofFor(l)
	return e == nil && ValidRuntimeProof(p) && expected == p
}

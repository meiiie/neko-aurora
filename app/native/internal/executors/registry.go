package executors

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	hexenc "encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const stateName = "executor-v1.json"
const HeartbeatMilliseconds = 15000
const maxStateBytes = 4 * 1024 * 1024
const maxLeases = 4096
const maxMutations = 1024

type Snapshot struct {
	Pin          InstallPin `json:"pin"`
	BootID       string     `json:"bootId"`
	Ready        bool       `json:"ready"`
	Reason       string     `json:"reason"`
	Revision     uint64     `json:"revision"`
	ReadyUntilMs uint64     `json:"readyUntilMs"`
}
type leaseRecord struct {
	Lease   AttemptLease `json:"lease"`
	Retired string       `json:"retired"`
}
type mutationReceipt struct {
	Kind       string     `json:"kind"`
	Expected   InstallPin `json:"expected"`
	Descriptor Descriptor `json:"descriptor"`
	Result     Snapshot   `json:"result"`
}
type registryState struct {
	Version      int                        `json:"version"`
	Pin          InstallPin                 `json:"pin"`
	BootID       string                     `json:"bootId"`
	Revision     uint64                     `json:"revision"`
	Fence        uint64                     `json:"fence"`
	Revoked      bool                       `json:"revoked"`
	Ready        bool                       `json:"ready"`
	Reason       string                     `json:"reason"`
	ReadyUntilMs uint64                     `json:"readyUntilMs"`
	ObservedAtMs uint64                     `json:"observedAtMs"`
	Leases       map[string]leaseRecord     `json:"leases"`
	Mutations    map[string]mutationReceipt `json:"mutations"`
}
type Registry struct {
	mu       sync.Mutex
	dir      string
	lock     *os.File
	state    registryState
	closed   bool
	poisoned bool
	clock    func() time.Time
	write    func([]byte) error
	diskSHA  [32]byte
	exists   bool
}

func Open(dir string, d Descriptor) (*Registry, error) {
	if !ValidDescriptor(d) || !filepath.IsAbs(dir) {
		return nil, ErrInvalid
	}
	dir = filepath.Clean(dir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, ErrUnavailable
	}
	if !safeDirectory(dir) {
		return nil, ErrUnavailable
	}
	lockPath := filepath.Join(dir, "executor-v1.lock")
	if info, err := os.Lstat(lockPath); err == nil && !info.Mode().IsRegular() {
		return nil, ErrUnavailable
	} else if err != nil && !os.IsNotExist(err) {
		return nil, ErrUnavailable
	}
	lock, err := lockWriter(lockPath)
	if err != nil {
		return nil, ErrUnavailable
	}
	if !singleFile(lock) {
		lock.Close()
		return nil, ErrUnavailable
	}
	r := &Registry{dir: dir, lock: lock, clock: time.Now}
	r.write = r.atomicWrite
	marker, err := readMarker(lock)
	if err != nil {
		lock.Close()
		return nil, ErrUnavailable
	}
	path := filepath.Join(dir, stateName)
	data, err := readSingle(path, maxStateBytes)
	if err == nil {
		if strictState(data, &r.state) != nil || !validState(r.state) || r.state.Pin.Descriptor != d || marker != r.state.Pin.ExecutorID {
			lock.Close()
			return nil, ErrUnavailable
		}
		r.exists = true
		r.diskSHA = sha256.Sum256(data)
	} else if os.IsNotExist(err) {
		if marker != "" {
			lock.Close()
			return nil, ErrUnavailable
		}
		eid, e := randomID("executor-")
		if e != nil {
			lock.Close()
			return nil, ErrUnavailable
		}
		r.state = registryState{Version: 1, Pin: InstallPin{Version: 1, ExecutorID: eid, Generation: 1, Descriptor: d}, Leases: map[string]leaseRecord{}, Mutations: map[string]mutationReceipt{}, Reason: "starting"}
		if _, e = lock.WriteAt([]byte(eid+"\n"), 0); e != nil {
			lock.Close()
			return nil, ErrUnavailable
		}
		if e = lock.Sync(); e != nil {
			lock.Close()
			return nil, ErrUnavailable
		}
	} else {
		lock.Close()
		return nil, ErrUnavailable
	}
	boot, e := randomID("executor-boot-")
	if e != nil {
		lock.Close()
		return nil, ErrUnavailable
	}
	next := cloneState(r.state)
	next.BootID = boot
	next.Ready = false
	next.ReadyUntilMs = 0
	next.Reason = "starting"
	if next.Revoked {
		next.Reason = "revoked"
	}
	retire(&next, "restart")
	now, e := r.now()
	if e != nil {
		lock.Close()
		return nil, e
	}
	if now > next.ObservedAtMs {
		next.ObservedAtMs = now
	}
	if e = r.commit(next); e != nil {
		lock.Close()
		return nil, ErrUnavailable
	}
	return r, nil
}
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	r.state.Ready = false
	r.state.ReadyUntilMs = 0
	if r.lock != nil {
		return r.lock.Close()
	}
	return nil
}
func (r *Registry) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed && !r.poisoned {
		r.observe()
	}
	s := snapshot(r.state)
	if r.closed || r.poisoned {
		s.Ready = false
		s.ReadyUntilMs = 0
		s.Reason = "registry_unavailable"
	}
	return s
}
func snapshot(s registryState) Snapshot {
	return Snapshot{Pin: s.Pin, BootID: s.BootID, Ready: s.Ready, Reason: s.Reason, Revision: s.Revision, ReadyUntilMs: s.ReadyUntilMs}
}
func (r *Registry) MarkReady(p InstallPin, boot string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ready(p, boot, false)
}
func (r *Registry) Heartbeat(p InstallPin, boot string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ready(p, boot, true)
}
func (r *Registry) ready(p InstallPin, boot string, heartbeat bool) error {
	if err := r.active(p, boot); err != nil {
		return err
	}
	if err := r.observe(); err != nil {
		return err
	}
	if r.state.Revoked || heartbeat && !r.state.Ready {
		return ErrUnavailable
	}
	now, err := r.now()
	if err != nil || now > SafeInteger-HeartbeatMilliseconds {
		return ErrUnavailable
	}
	next := cloneState(r.state)
	next.Ready = true
	next.Reason = "ready"
	next.ReadyUntilMs = now + HeartbeatMilliseconds
	next.ObservedAtMs = now
	return r.commit(next)
}
func (r *Registry) MarkUnavailable(p InstallPin, boot, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reason != "host_disconnected" && reason != "host_unavailable" && reason != "shutdown" {
		return ErrInvalid
	}
	if e := r.active(p, boot); e != nil {
		return e
	}
	next := cloneState(r.state)
	next.Ready = false
	next.ReadyUntilMs = 0
	next.Reason = reason
	retire(&next, "unavailable")
	return r.commit(next)
}
func (r *Registry) Issue(p InstallPin, scope TaskScope) (AttemptLease, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !validScope(scope) {
		return AttemptLease{}, false, ErrInvalid
	}
	if e := r.active(p, r.state.BootID); e != nil {
		return AttemptLease{}, false, e
	}
	if e := r.observe(); e != nil {
		return AttemptLease{}, false, e
	}
	key := scope.RequestID + ":" + scope.AttemptID
	if saved, ok := r.state.Leases[key]; ok {
		if saved.Lease.TaskScope != scope || saved.Lease.ExecutorID != p.ExecutorID || saved.Lease.Generation != p.Generation {
			return AttemptLease{}, false, ErrConflict
		}
		if e := r.allowed(p, saved.Lease); e != nil {
			return saved.Lease, true, e
		}
		return saved.Lease, true, nil
	}
	if !r.state.Ready || r.state.Revoked {
		return AttemptLease{}, false, ErrUnavailable
	}
	if len(r.state.Leases) >= maxLeases || r.state.Fence >= SafeInteger {
		return AttemptLease{}, false, ErrUnavailable
	}
	now, e := r.now()
	if e != nil || now > SafeInteger-MaxLeaseMilliseconds {
		return AttemptLease{}, false, ErrUnavailable
	}
	lid, e := randomID("executor-lease-")
	if e != nil {
		return AttemptLease{}, false, ErrUnavailable
	}
	next := cloneState(r.state)
	next.Fence++
	l := AttemptLease{Version: 1, ID: lid, ExecutorID: p.ExecutorID, Generation: p.Generation, BootID: next.BootID, TaskScope: scope, Fence: next.Fence, IssuedAtMs: now, ExpiresAtMs: now + MaxLeaseMilliseconds}
	next.Leases[key] = leaseRecord{Lease: l}
	next.ObservedAtMs = now
	if e = r.commit(next); e != nil {
		return AttemptLease{}, false, e
	}
	return l, false, nil
}
func (r *Registry) WithLease(p InstallPin, l AttemptLease, fn func() error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if fn == nil || !ValidLease(l) {
		return ErrInvalid
	}
	if e := r.active(p, l.BootID); e != nil {
		return e
	}
	if e := r.observe(); e != nil {
		return e
	}
	if e := r.allowed(p, l); e != nil {
		return e
	}
	return fn()
}

// WithInstall linearizes immutable install selection and a bounded journal
// commit. Readiness is separate: same-install queued obligations may persist
// while offline. The callback must not reenter the registry or spawn workers.
func (r *Registry) WithInstall(p InstallPin, fn func() error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if fn == nil || !ValidInstallPin(p) {
		return ErrInvalid
	}
	if e := r.active(p, r.state.BootID); e != nil {
		return e
	}
	if e := r.observe(); e != nil {
		return e
	}
	if r.state.Revoked {
		return ErrUnavailable
	}
	return fn()
}
func (r *Registry) allowed(p InstallPin, l AttemptLease) error {
	if r.closed || r.poisoned || !r.state.Ready || r.state.Revoked {
		return ErrUnavailable
	}
	if l.ExecutorID != p.ExecutorID || l.Generation != p.Generation || l.BootID != r.state.BootID {
		return ErrConflict
	}
	saved, ok := r.state.Leases[l.RequestID+":"+l.AttemptID]
	if !ok || saved.Lease != l {
		return ErrConflict
	}
	if saved.Retired != "" {
		if saved.Retired == "expired" {
			return ErrExpired
		}
		return ErrUnavailable
	}
	return nil
}
func (r *Registry) Revoke(p InstallPin, nonce string) (Snapshot, bool, error) {
	return r.mutate(p, Descriptor{}, nonce, "revoke")
}
func (r *Registry) Configure(p InstallPin, d Descriptor, nonce string) (Snapshot, bool, error) {
	if !ValidDescriptor(d) {
		return Snapshot{}, false, ErrInvalid
	}
	return r.mutate(p, d, nonce, "configure")
}
func (r *Registry) mutate(p InstallPin, d Descriptor, nonce, kind string) (Snapshot, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !validNonce(nonce) || !ValidInstallPin(p) {
		return Snapshot{}, false, ErrInvalid
	}
	if r.closed || r.poisoned {
		return Snapshot{}, false, ErrUnavailable
	}
	if r.checkIdentity() != nil {
		return Snapshot{}, false, ErrUnavailable
	}
	if receipt, ok := r.state.Mutations[nonce]; ok {
		if receipt.Kind != kind || receipt.Expected != p || receipt.Descriptor != d {
			return Snapshot{}, false, ErrConflict
		}
		return receipt.Result, true, nil
	}
	if p != r.state.Pin {
		return Snapshot{}, false, ErrConflict
	}
	if r.state.Pin.Generation >= SafeInteger || len(r.state.Mutations) >= maxMutations {
		return Snapshot{}, false, ErrUnavailable
	}
	next := cloneState(r.state)
	next.Pin.Generation++
	next.Revoked = kind == "revoke"
	next.Ready = false
	next.ReadyUntilMs = 0
	next.Reason = "revoked"
	retire(&next, "revoked")
	if kind == "configure" {
		next.Pin.Descriptor = d
		next.Reason = "configured"
	}
	if next.Revision >= SafeInteger {
		return Snapshot{}, false, ErrUnavailable
	}
	result := snapshot(next)
	result.Revision++
	next.Mutations[nonce] = mutationReceipt{Kind: kind, Expected: p, Descriptor: d, Result: result}
	if e := r.commit(next); e != nil {
		return Snapshot{}, false, e
	}
	return result, false, nil
}
func (r *Registry) active(p InstallPin, boot string) error {
	if r.closed || r.poisoned {
		return ErrUnavailable
	}
	if !ValidInstallPin(p) || !id(boot, "executor-boot-") {
		return ErrInvalid
	}
	if p != r.state.Pin || boot != r.state.BootID {
		return ErrConflict
	}
	return nil
}
func (r *Registry) observe() error {
	if r.closed || r.poisoned {
		return ErrUnavailable
	}
	if r.checkIdentity() != nil {
		return ErrUnavailable
	}
	now, e := r.now()
	if e != nil {
		r.poisoned = true
		return ErrUnavailable
	}
	next := cloneState(r.state)
	changed := false
	rewind := now < next.ObservedAtMs
	if rewind {
		if next.Reason != "clock_rewind" || next.Ready {
			next.Ready = false
			next.ReadyUntilMs = 0
			next.Reason = "clock_rewind"
			retire(&next, "clock_rewind")
			changed = true
		}
	}
	if !rewind && next.Ready && now >= next.ReadyUntilMs {
		next.Ready = false
		next.ReadyUntilMs = 0
		next.Reason = "heartbeat_expired"
		retire(&next, "unavailable")
		changed = true
	}
	for key, record := range next.Leases {
		if record.Retired == "" && now >= record.Lease.ExpiresAtMs {
			record.Retired = "expired"
			next.Leases[key] = record
			changed = true
		}
	}
	if changed {
		if now > next.ObservedAtMs {
			next.ObservedAtMs = now
		}
		if e = r.commit(next); e != nil {
			return e
		}
	}
	if rewind {
		return ErrUnavailable
	}
	return nil
}
func (r *Registry) now() (uint64, error) {
	n := r.clock().UnixMilli()
	if n <= 0 || uint64(n) > SafeInteger {
		return 0, ErrUnavailable
	}
	return uint64(n), nil
}
func retire(s *registryState, reason string) {
	for key, record := range s.Leases {
		if record.Retired == "" {
			record.Retired = reason
			s.Leases[key] = record
		}
	}
}
func cloneState(s registryState) registryState {
	n := s
	n.Leases = make(map[string]leaseRecord, len(s.Leases))
	for k, v := range s.Leases {
		n.Leases[k] = v
	}
	n.Mutations = make(map[string]mutationReceipt, len(s.Mutations))
	for k, v := range s.Mutations {
		n.Mutations[k] = v
	}
	return n
}
func (r *Registry) commit(next registryState) error {
	if r.closed || r.poisoned || next.Revision >= SafeInteger {
		return ErrUnavailable
	}
	next.Revision++
	if !validState(next) {
		return ErrInvalid
	}
	b, e := json.Marshal(next)
	if e != nil || len(b) > maxStateBytes {
		return ErrUnavailable
	}
	if e = r.write(b); e != nil {
		r.poisoned = true
		r.state.Ready = false
		return ErrUnavailable
	}
	r.state = next
	return nil
}
func randomID(prefix string) (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return prefix + hexenc.EncodeToString(b), nil
}
func validNonce(s string) bool {
	if len(s) < 1 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-_:.@", c)) {
			return false
		}
	}
	return true
}
func validReason(s string) bool {
	switch s {
	case "starting", "ready", "revoked", "configured", "host_disconnected", "host_unavailable", "shutdown", "heartbeat_expired", "clock_rewind":
		return true
	}
	return false
}
func validRetirement(s string) bool {
	switch s {
	case "", "restart", "unavailable", "revoked", "expired", "clock_rewind":
		return true
	}
	return false
}
func validState(s registryState) bool {
	if s.Version != 1 || !ValidInstallPin(s.Pin) || !id(s.BootID, "executor-boot-") || !positive(s.Revision) || s.Fence > SafeInteger || s.ObservedAtMs > SafeInteger || s.ReadyUntilMs > SafeInteger || !validReason(s.Reason) || s.Leases == nil || s.Mutations == nil || len(s.Leases) > maxLeases || len(s.Mutations) > maxMutations {
		return false
	}
	if s.Ready && (s.Revoked || s.Reason != "ready" || s.ReadyUntilMs <= s.ObservedAtMs) || !s.Ready && s.ReadyUntilMs != 0 {
		return false
	}
	ids := map[string]bool{}
	fences := map[uint64]bool{}
	for key, rec := range s.Leases {
		l := rec.Lease
		if !ValidLease(l) || key != l.RequestID+":"+l.AttemptID || l.ExecutorID != s.Pin.ExecutorID || l.Generation > s.Pin.Generation || l.Fence > s.Fence || ids[l.ID] || fences[l.Fence] || !validRetirement(rec.Retired) {
			return false
		}
		if rec.Retired == "" && (l.Generation != s.Pin.Generation || l.BootID != s.BootID || s.Revoked || !s.Ready) {
			return false
		}
		ids[l.ID] = true
		fences[l.Fence] = true
	}
	for nonce, rec := range s.Mutations {
		n := rec.Result
		if !validNonce(nonce) || !ValidInstallPin(rec.Expected) || !ValidInstallPin(n.Pin) || n.Pin.ExecutorID != s.Pin.ExecutorID || rec.Expected.ExecutorID != s.Pin.ExecutorID || n.Pin.Generation != rec.Expected.Generation+1 || n.Pin.Generation > s.Pin.Generation || !positive(n.Revision) || n.Revision > s.Revision || n.Ready || n.ReadyUntilMs != 0 || !id(n.BootID, "executor-boot-") {
			return false
		}
		if rec.Kind == "revoke" {
			if rec.Descriptor != (Descriptor{}) || n.Pin.Descriptor != rec.Expected.Descriptor || n.Reason != "revoked" {
				return false
			}
		} else if rec.Kind == "configure" {
			if !ValidDescriptor(rec.Descriptor) || n.Pin.Descriptor != rec.Descriptor || n.Reason != "configured" {
				return false
			}
		} else {
			return false
		}
	}
	return true
}
func strictState(b []byte, out *registryState) error {
	if len(b) == 0 || len(b) > maxStateBytes || !utf8.Valid(b) {
		return ErrInvalid
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if e := checkTokens(dec, 0); e != nil {
		return ErrInvalid
	}
	if _, e := dec.Token(); e != io.EOF {
		return ErrInvalid
	}
	typed := json.NewDecoder(bytes.NewReader(b))
	typed.DisallowUnknownFields()
	if typed.Decode(out) != nil {
		return ErrInvalid
	}
	rendered, e := json.Marshal(out)
	if e != nil {
		return ErrInvalid
	}
	var left, right any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if d.Decode(&left) != nil {
		return ErrInvalid
	}
	d = json.NewDecoder(bytes.NewReader(rendered))
	d.UseNumber()
	if d.Decode(&right) != nil || !reflect.DeepEqual(left, right) {
		return ErrInvalid
	}
	return nil
}
func checkTokens(dec *json.Decoder, depth int) error {
	if depth > 32 {
		return ErrInvalid
	}
	token, e := dec.Token()
	if e != nil {
		return ErrInvalid
	}
	switch v := token.(type) {
	case json.Delim:
		if v == '{' {
			seen := map[string]bool{}
			for dec.More() {
				key, e := dec.Token()
				s, ok := key.(string)
				if e != nil || !ok || seen[s] || strings.ContainsRune(s, '\uFFFD') {
					return ErrInvalid
				}
				seen[s] = true
				if e = checkTokens(dec, depth+1); e != nil {
					return e
				}
			}
			end, e := dec.Token()
			if e != nil || end != json.Delim('}') {
				return ErrInvalid
			}
		} else if v == '[' {
			for dec.More() {
				if e = checkTokens(dec, depth+1); e != nil {
					return e
				}
			}
			end, e := dec.Token()
			if e != nil || end != json.Delim(']') {
				return ErrInvalid
			}
		} else {
			return ErrInvalid
		}
	case json.Number:
		if strings.ContainsAny(string(v), ".eE") || string(v) == "-0" {
			return ErrInvalid
		}
		n, e := strconv.ParseInt(string(v), 10, 64)
		if e != nil || n < 0 || uint64(n) > SafeInteger {
			return ErrInvalid
		}
	case string:
		if strings.ContainsRune(v, '\uFFFD') {
			return ErrInvalid
		}
	case nil, bool:
	default:
		return ErrInvalid
	}
	return nil
}
func safeDirectory(dir string) bool {
	for {
		info, e := os.Lstat(dir)
		if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return true
		}
		dir = parent
	}
}
func readSingle(path string, max int) ([]byte, error) {
	info, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !info.Mode().IsRegular() {
		return nil, ErrUnavailable
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	if !singleFile(f) {
		return nil, ErrUnavailable
	}
	b, e := io.ReadAll(io.LimitReader(f, int64(max+1)))
	if e != nil || len(b) > max {
		return nil, ErrUnavailable
	}
	return b, nil
}
func (r *Registry) atomicWrite(b []byte) error {
	path := filepath.Join(r.dir, stateName)
	old, e := readSingle(path, maxStateBytes)
	if r.exists {
		if e != nil || sha256.Sum256(old) != r.diskSHA {
			return ErrUnavailable
		}
	} else if !os.IsNotExist(e) {
		return ErrUnavailable
	}
	name, e := randomID("executor-tmp-")
	if e != nil {
		return e
	}
	tmp := filepath.Join(r.dir, name)
	f, e := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer os.Remove(tmp)
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if e = replaceFile(tmp, path); e != nil {
		return e
	}
	r.diskSHA = sha256.Sum256(b)
	r.exists = true
	return nil
}

func readMarker(f *os.File) (string, error) {
	if !singleFile(f) {
		return "", ErrUnavailable
	}
	b := make([]byte, 256)
	n, e := f.ReadAt(b, 0)
	if e != nil && e != io.EOF {
		return "", ErrUnavailable
	}
	if n == 0 {
		return "", nil
	}
	if n != 42 || b[n-1] != '\n' || !id(string(b[:n-1]), "executor-") {
		return "", ErrUnavailable
	}
	return string(b[:n-1]), nil
}
func (r *Registry) checkIdentity() error {
	marker, e := readMarker(r.lock)
	if e != nil || marker != r.state.Pin.ExecutorID {
		r.poisoned = true
		return ErrUnavailable
	}
	b, e := readSingle(filepath.Join(r.dir, stateName), maxStateBytes)
	if e != nil || sha256.Sum256(b) != r.diskSHA {
		r.poisoned = true
		return ErrUnavailable
	}
	return nil
}

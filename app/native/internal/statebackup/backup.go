package statebackup

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"nekobot.local/native/internal/ledger"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalid     = errors.New("backup_invalid")
	ErrUnsafe      = errors.New("backup_unsafe_path")
	ErrBusy        = errors.New("backup_state_busy")
	ErrLive        = errors.New("backup_process_live_or_unknown")
	ErrProtection  = errors.New("backup_protection_failed")
	ErrUnsupported = errors.New("backup_platform_unsupported")
)

const maxFile = int64(64 << 20)
const maxTotal = int64(512 << 20)
const maxManifest = int64(2 << 20)
const maxFiles = 5000

type File struct {
	Path         string `json:"path"`
	Bytes        int64  `json:"bytes"`
	ModifiedAt   string `json:"modifiedAt"`
	SHA256       string `json:"sha256"`
	Blob         string `json:"blob"`
	CipherBytes  int64  `json:"cipherBytes"`
	CipherSHA256 string `json:"cipherSha256"`
}
type Manifest struct {
	Version             int    `json:"version"`
	SnapshotID          string `json:"snapshotId"`
	ApplicationRevision string `json:"applicationRevision"`
	CreatedAt           string `json:"createdAt"`
	Protection          string `json:"protection"`
	ExecutionDisabled   bool   `json:"executionDisabled"`
	Sequence            uint64 `json:"sequence"`
	Files               []File `json:"files"`
}
type envelope struct {
	Version      int    `json:"version"`
	SnapshotID   string `json:"snapshotId"`
	Protection   string `json:"protection"`
	CipherBytes  int64  `json:"cipherBytes"`
	CipherSHA256 string `json:"cipherSha256"`
}

func manifestEntropy(id string) []byte {
	h := sha256.Sum256([]byte("neko-local-backup-manifest-v1:" + id))
	return h[:]
}

type pinned struct {
	files    map[string]*os.File
	manifest Manifest
	digest   string
}

func (p *pinned) close() {
	for _, f := range p.files {
		f.Close()
	}
}
func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func validHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func validRevision(s string) bool { return s == "working-tree" || validHex(s, 40) }
func safePath(name string) bool {
	if name == "" || len(name) > 512 || strings.Contains(name, "\\") || strings.Contains(name, ":") || strings.HasPrefix(name, "/") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || len(part) > 255 || strings.HasSuffix(part, ".") {
			return false
		}
		for _, c := range part {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
				return false
			}
		}
		stem := strings.ToUpper(strings.Split(part, ".")[0])
		if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9' {
			return false
		}
	}
	return true
}
func safeDirectory(path string, exists bool) (string, error) {
	if !filepath.IsAbs(path) || strings.HasPrefix(path, `\\`) || strings.Contains(strings.TrimPrefix(path, filepath.VolumeName(path)), ":") {
		return "", ErrUnsafe
	}
	for _, part := range strings.Split(filepath.ToSlash(strings.TrimPrefix(path, filepath.VolumeName(path))), "/") {
		if part != "" && (strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.Contains(part, "~")) {
			return "", ErrUnsafe
		}
	}
	path = filepath.Clean(path)
	cursor := path
	for {
		info, e := os.Lstat(cursor)
		if e == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return "", ErrUnsafe
			}
			if e = directorySafe(cursor); e != nil {
				return "", e
			}
		}
		if e != nil && !os.IsNotExist(e) {
			return "", ErrUnsafe
		}
		if cursor == path && exists && e != nil {
			return "", ErrInvalid
		}
		parent := filepath.Dir(cursor)
		if parent == cursor {
			break
		}
		cursor = parent
	}
	return path, nil
}
func inside(a, b string) bool {
	a = strings.ToLower(filepath.Clean(a))
	b = strings.ToLower(filepath.Clean(b))
	return a == b || strings.HasPrefix(a, b+string(filepath.Separator))
}
func newDestination(target, other string) (string, error) {
	p, e := safeDirectory(target, false)
	if e != nil {
		return "", e
	}
	if inside(p, other) || inside(other, p) {
		return "", ErrUnsafe
	}
	if _, e = os.Lstat(p); !os.IsNotExist(e) {
		return "", ErrUnsafe
	}
	otherInfo, e := os.Stat(other)
	if e != nil || !otherInfo.IsDir() {
		return "", ErrUnsafe
	}
	for cursor := filepath.Dir(p); ; cursor = filepath.Dir(cursor) {
		info, err := os.Stat(cursor)
		if err != nil || !info.IsDir() || os.SameFile(otherInfo, info) {
			return "", ErrUnsafe
		}
		if _, e = os.Lstat(filepath.Join(cursor, "transactions.jsonl")); e == nil {
			return "", ErrUnsafe
		}
		if filepath.Dir(cursor) == cursor {
			break
		}
	}
	if info, e := os.Stat(filepath.Dir(p)); e != nil || !info.IsDir() {
		return "", ErrUnsafe
	}
	return p, nil
}
func inventory(root string, skipWriter bool) ([]string, error) {
	var names []string
	e := filepath.WalkDir(root, func(path string, entry os.DirEntry, e error) error {
		if e != nil {
			return ErrUnsafe
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return ErrUnsafe
		}
		if entry.IsDir() {
			return directorySafe(path)
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return ErrUnsafe
		}
		rel = filepath.ToSlash(rel)
		if rel == "writer.lock" && skipWriter {
			return nil
		}
		if !safePath(rel) || strings.HasSuffix(strings.ToLower(rel), ".tmp") || len(names) >= maxFiles+3 {
			return ErrUnsafe
		}
		info, e := entry.Info()
		if e != nil || !info.Mode().IsRegular() {
			return ErrUnsafe
		}
		names = append(names, rel)
		return nil
	})
	sort.Strings(names)
	return names, e
}
func pin(root string, names []string) (*pinned, error) {
	p := &pinned{files: map[string]*os.File{}}
	for _, name := range names {
		f, e := openPinned(filepath.Join(root, filepath.FromSlash(name)))
		if e != nil {
			p.close()
			return nil, e
		}
		p.files[name] = f
	}
	return p, nil
}
func readFile(f *os.File, limit int64) ([]byte, error) {
	if f == nil {
		return nil, ErrInvalid
	}
	info, e := f.Stat()
	if e != nil || info.Size() < 0 || info.Size() > limit {
		return nil, ErrInvalid
	}
	if _, e = f.Seek(0, io.SeekStart); e != nil {
		return nil, ErrInvalid
	}
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil || int64(len(b)) != info.Size() {
		clear(b)
		return nil, ErrInvalid
	}
	return b, nil
}
func writeNew(path string, b []byte) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return ErrUnsafe
	}
	n, e := f.Write(b)
	if e == nil && n != len(b) {
		e = io.ErrShortWrite
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil || closeErr != nil {
		return ErrInvalid
	}
	return nil
}
func entropy(m Manifest, f File) []byte {
	b, _ := json.Marshal([]any{"neko-local-backup-v1", m.SnapshotID, m.ApplicationRevision, m.CreatedAt, m.Protection, m.ExecutionDisabled, m.Sequence, f.Path, f.Bytes, f.ModifiedAt, f.SHA256})
	h := sha256.Sum256(b)
	return h[:]
}
func consistentReferences(snapshot ledger.Snapshot, files []File) bool {
	byPath := map[string]File{}
	for _, f := range files {
		byPath[f.Path] = f
	}
	check := func(path, hash string, size int) bool {
		f, ok := byPath[path]
		return ok && f.SHA256 == hash && f.Bytes == int64(size)
	}
	for _, w := range snapshot.Work {
		for _, a := range w.Artifacts {
			if !check("artifacts/"+a.ID, a.SHA256, a.Bytes) {
				return false
			}
		}
	}
	for _, f := range snapshot.Files {
		if !check("uploads/"+f.ID, f.SHA256, f.Bytes) {
			return false
		}
	}
	return true
}
func Create(source, destination, application string) (Manifest, error) {
	root, e := safeDirectory(source, true)
	if e != nil {
		return Manifest{}, e
	}
	target, e := newDestination(destination, root)
	if e != nil {
		return Manifest{}, e
	}
	if !validRevision(application) {
		return Manifest{}, ErrInvalid
	}
	names, e := inventory(root, true)
	if e != nil {
		return Manifest{}, e
	}
	if len(names) > maxFiles {
		return Manifest{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, name := range names {
		key := strings.ToLower(name)
		if seen[key] {
			return Manifest{}, ErrInvalid
		}
		seen[key] = true
	}
	if !seen["transactions.jsonl"] {
		return Manifest{}, ErrInvalid
	}
	if _, e = os.Lstat(filepath.Join(root, "restore-readonly.json")); !os.IsNotExist(e) {
		return Manifest{}, ErrInvalid
	}
	store, e := ledger.OpenReadOnly(root)
	if e != nil {
		if errors.Is(e, ledger.ErrCorrupt) {
			return Manifest{}, ErrInvalid
		}
		return Manifest{}, ErrBusy
	}
	defer store.Close()
	snap := store.Snapshot()
	for _, w := range snap.Work {
		bindings := []*ledger.CoreBinding{w.Core, w.RuntimeAttemptBinding}
		for _, a := range w.ContextAttempts {
			bindings = append(bindings, a.Core)
		}
		for _, b := range bindings {
			if b != nil {
				if b.NativeProcessID == 0 && w.Status != "completed" && w.Status != "failed" && w.Status != "cancelled" {
					return Manifest{}, ErrLive
				}
				if e = processInactive(b.NativeProcessID); e != nil {
					return Manifest{}, e
				}
			}
		}
	}
	pinned, e := pin(root, names)
	if e != nil {
		return Manifest{}, e
	}
	defer pinned.close()
	again, e := inventory(root, true)
	if e != nil || !reflect.DeepEqual(names, again) {
		return Manifest{}, ErrBusy
	}
	var id [16]byte
	if _, e = rand.Read(id[:]); e != nil {
		return Manifest{}, ErrProtection
	}
	m := Manifest{Version: 1, SnapshotID: hex.EncodeToString(id[:]), ApplicationRevision: application, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Protection: "dpapi-current-user-v1", ExecutionDisabled: true, Sequence: snap.Sequence, Files: []File{}}
	if e = makePrivateDirectory(target); e != nil {
		return Manifest{}, ErrUnsafe
	}
	if e = os.Mkdir(filepath.Join(target, "data"), 0700); e != nil {
		return Manifest{}, ErrUnsafe
	}
	var total int64
	for i, name := range names {
		f := pinned.files[name]
		info, e := f.Stat()
		if e != nil {
			return Manifest{}, ErrInvalid
		}
		raw, e := readFile(f, maxFile)
		if e != nil {
			return Manifest{}, e
		}
		total += int64(len(raw))
		if total > maxTotal {
			clear(raw)
			return Manifest{}, ErrInvalid
		}
		entry := File{Path: name, Bytes: int64(len(raw)), ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano), SHA256: sum(raw), Blob: fmt.Sprintf("data/%06d.dpapi", i)}
		encrypted, e := protect(raw, entropy(m, entry))
		clear(raw)
		if e != nil {
			return Manifest{}, e
		}
		entry.CipherBytes = int64(len(encrypted))
		entry.CipherSHA256 = sum(encrypted)
		if e = writeNew(filepath.Join(target, filepath.FromSlash(entry.Blob)), encrypted); e != nil {
			clear(encrypted)
			return Manifest{}, e
		}
		clear(encrypted)
		m.Files = append(m.Files, entry)
	}
	again, e = inventory(root, true)
	if e != nil || !reflect.DeepEqual(names, again) {
		return Manifest{}, ErrBusy
	}
	if !consistentReferences(snap, m.Files) {
		return Manifest{}, ErrInvalid
	}
	encoded, e := json.Marshal(m)
	if e != nil || int64(len(encoded)) > maxManifest {
		return Manifest{}, ErrInvalid
	}
	protected, e := protect(encoded, manifestEntropy(m.SnapshotID))
	clear(encoded)
	if e != nil {
		return Manifest{}, e
	}
	header := envelope{Version: 1, SnapshotID: m.SnapshotID, Protection: m.Protection, CipherBytes: int64(len(protected)), CipherSHA256: sum(protected)}
	if e = writeNew(filepath.Join(target, "manifest.dpapi"), protected); e != nil {
		clear(protected)
		return Manifest{}, e
	}
	clear(protected)
	encoded, e = json.Marshal(header)
	if e != nil {
		return Manifest{}, ErrInvalid
	}
	if e = writeNew(filepath.Join(target, "manifest.json"), encoded); e != nil {
		return Manifest{}, e
	}
	if e = writeNew(filepath.Join(target, "manifest.sha256"), []byte(sum(encoded)+"\n")); e != nil {
		return Manifest{}, e
	}
	return m, nil
}
func loadProtected(root string) (*pinned, error) {
	root, e := safeDirectory(root, true)
	if e != nil {
		return nil, e
	}
	names, e := inventory(root, false)
	if e != nil || len(names) < 4 {
		return nil, ErrInvalid
	}
	p, e := pin(root, names)
	if e != nil {
		return nil, e
	}
	failed := true
	defer func() {
		if failed {
			p.close()
		}
	}()
	raw, e := readFile(p.files["manifest.json"], maxManifest)
	if e != nil {
		return nil, ErrInvalid
	}
	digest, e := readFile(p.files["manifest.sha256"], 65)
	if e != nil || string(digest) != sum(raw)+"\n" {
		return nil, ErrInvalid
	}
	var header envelope
	if decodeManifestJSON(raw, &header) != nil || header.Version != 1 || !validHex(header.SnapshotID, 32) || header.Protection != "dpapi-current-user-v1" || header.CipherBytes <= 0 || header.CipherBytes > maxManifest+16384 || !validHex(header.CipherSHA256, 64) {
		return nil, ErrInvalid
	}
	metadata, e := readFile(p.files["manifest.dpapi"], maxManifest+16384)
	if e != nil || int64(len(metadata)) != header.CipherBytes || sum(metadata) != header.CipherSHA256 {
		clear(metadata)
		return nil, ErrInvalid
	}
	decoded, e := unprotect(metadata, manifestEntropy(header.SnapshotID))
	clear(metadata)
	if e != nil {
		return nil, e
	}
	defer clear(decoded)
	if int64(len(decoded)) > maxManifest {
		return nil, ErrInvalid
	}
	if decodeManifestJSON(decoded, &p.manifest) != nil {
		return nil, ErrInvalid
	}
	m := p.manifest
	if m.SnapshotID != header.SnapshotID || m.Protection != header.Protection {
		return nil, ErrInvalid
	}
	if m.Version != 1 || !validHex(m.SnapshotID, 32) || m.Protection != "dpapi-current-user-v1" || !m.ExecutionDisabled || !validRevision(m.ApplicationRevision) || len(m.Files) == 0 || len(m.Files) > maxFiles {
		return nil, ErrInvalid
	}
	if _, e = time.Parse(time.RFC3339Nano, m.CreatedAt); e != nil {
		return nil, ErrInvalid
	}
	seen := map[string]bool{}
	expected := []string{"manifest.json", "manifest.sha256", "manifest.dpapi"}
	var total int64
	for i, f := range m.Files {
		rootName := strings.ToLower(strings.Split(f.Path, "/")[0])
		if !safePath(f.Path) || rootName == "writer.lock" || rootName == "restore-readonly.json" || seen[strings.ToLower(f.Path)] || !validHex(f.SHA256, 64) || !validHex(f.CipherSHA256, 64) || f.Blob != fmt.Sprintf("data/%06d.dpapi", i) || f.Bytes < 0 || f.Bytes > maxFile || f.CipherBytes <= 0 || f.CipherBytes > 65<<20 {
			return nil, ErrInvalid
		}
		if _, e = time.Parse(time.RFC3339Nano, f.ModifiedAt); e != nil {
			return nil, ErrInvalid
		}
		seen[strings.ToLower(f.Path)] = true
		total += f.Bytes
		if total > maxTotal {
			return nil, ErrInvalid
		}
		expected = append(expected, f.Blob)
		cipher, e := readFile(p.files[f.Blob], 65<<20)
		if e != nil || int64(len(cipher)) != f.CipherBytes || sum(cipher) != f.CipherSHA256 {
			clear(cipher)
			return nil, ErrInvalid
		}
		plain, e := unprotect(cipher, entropy(m, f))
		clear(cipher)
		if e != nil {
			return nil, e
		}
		valid := int64(len(plain)) == f.Bytes && sum(plain) == f.SHA256
		clear(plain)
		if !valid {
			return nil, ErrInvalid
		}
	}
	sort.Strings(expected)
	if !reflect.DeepEqual(names, expected) {
		return nil, ErrInvalid
	}
	if !seen["transactions.jsonl"] {
		return nil, ErrInvalid
	}
	for name := range seen {
		for parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(name))); parent != "."; parent = filepath.ToSlash(filepath.Dir(filepath.FromSlash(parent))) {
			if seen[parent] {
				return nil, ErrInvalid
			}
		}
	}
	p.digest = sum(raw)
	failed = false
	return p, nil
}
func Verify(backup string) (Manifest, error) {
	p, e := loadProtected(backup)
	if e != nil {
		return Manifest{}, e
	}
	defer p.close()
	return p.manifest, nil
}
func RestoreDryRun(backup, destination string) (Manifest, error) {
	p, e := loadProtected(backup)
	if e != nil {
		return Manifest{}, e
	}
	defer p.close()
	root, e := safeDirectory(backup, true)
	if e != nil {
		return Manifest{}, e
	}
	target, e := newDestination(destination, root)
	if e != nil {
		return Manifest{}, e
	}
	if e = makePrivateDirectory(target); e != nil {
		return Manifest{}, ErrUnsafe
	}
	marker, _ := json.Marshal(map[string]any{"version": 1, "executionDisabled": true, "manifestSHA256": p.digest})
	if e = writeNew(filepath.Join(target, "restore-readonly.json"), marker); e != nil {
		return Manifest{}, e
	}
	for _, entry := range p.manifest.Files {
		cipher, e := readFile(p.files[entry.Blob], 65<<20)
		if e != nil {
			return Manifest{}, e
		}
		plain, e := unprotect(cipher, entropy(p.manifest, entry))
		clear(cipher)
		if e != nil {
			return Manifest{}, e
		}
		path := filepath.Join(target, filepath.FromSlash(entry.Path))
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			clear(plain)
			return Manifest{}, ErrUnsafe
		}
		e = writeNew(path, plain)
		clear(plain)
		if e != nil {
			return Manifest{}, e
		}
		modified, _ := time.Parse(time.RFC3339Nano, entry.ModifiedAt)
		if e = os.Chtimes(path, modified, modified); e != nil {
			return Manifest{}, ErrInvalid
		}
		if e = os.Chmod(path, 0400); e != nil {
			return Manifest{}, ErrInvalid
		}
	}
	if e = writeNew(filepath.Join(target, "writer.lock"), nil); e != nil {
		return Manifest{}, e
	}
	s, e := ledger.OpenReadOnly(target)
	if e != nil {
		return Manifest{}, ErrInvalid
	}
	snapshot := s.Snapshot()
	sequence := snapshot.Sequence
	s.Close()
	if sequence != p.manifest.Sequence || !consistentReferences(snapshot, p.manifest.Files) {
		return Manifest{}, ErrInvalid
	}
	return p.manifest, nil
}

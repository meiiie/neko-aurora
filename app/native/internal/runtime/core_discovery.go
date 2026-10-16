package runtime

import (
	"context"
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"io"
	"nekobot.local/native/internal/protocol"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const discoveryMaxDirectories = 16
const discoveryMaxCandidates = 64
const discoveryMaxFileBytes int64 = 256 << 20
const discoveryMaxTotalBytes int64 = 512 << 20
const discoveryTimeout = 3 * time.Second

type CoreDiscoveryCandidate struct {
	ID                string `json:"id"`
	Origin            string `json:"origin"`
	Path              string `json:"path"`
	FileBytes         int64  `json:"fileBytes"`
	SHA256            string `json:"sha256,omitempty"`
	MetadataVersion   string `json:"metadataVersion,omitempty"`
	PinnedVersion     string `json:"pinnedVersion,omitempty"`
	RecognizedVersion string `json:"recognizedVersion,omitempty"`
	RecognitionSource string `json:"recognitionSource,omitempty"`
	SourceRevision    string `json:"sourceRevision,omitempty"`
	Status            string `json:"status"`
	Reason            string `json:"reason,omitempty"`
	Compatible        bool   `json:"compatible"`
	ContextProtocol   int    `json:"contextProtocol,omitempty"`
	ManifestVersions  []int  `json:"manifestVersions,omitempty"`
}
type CoreDiscovery struct {
	Version                int                      `json:"version"`
	Mode                   string                   `json:"mode"`
	Status                 string                   `json:"status"`
	Candidates             []CoreDiscoveryCandidate `json:"candidates"`
	RecommendedCandidateID string                   `json:"recommendedCandidateId,omitempty"`
	Usability              string                   `json:"usability"`
	ConfigInspection       string                   `json:"configInspection"`
	CredentialAccess       string                   `json:"credentialAccess"`
	Execution              string                   `json:"execution"`
	Limited                bool                     `json:"limited"`
}
type discoveryInputs struct {
	path, pathExt, localAppData, executable string
	pin                                     protocol.CoreDiscoveryPin
}
type discoveryPath struct{ path, origin string }

func DiscoverCore(ctx context.Context) CoreDiscovery {
	executable, _ := os.Executable()
	return discoverCore(ctx, discoveryInputs{os.Getenv("PATH"), os.Getenv("PATHEXT"), os.Getenv("LOCALAPPDATA"), executable, protocol.CurrentWindowsCoreDiscoveryPin()})
}
func discoveryPaths(in discoveryInputs) ([]discoveryPath, bool) {
	out := []discoveryPath{}
	seen := map[string]bool{}
	limited := false
	add := func(path, origin string) {
		if len(out) >= discoveryMaxCandidates {
			limited = true
			return
		}
		if len(path) > 4096 || !filepath.IsAbs(path) || strings.HasPrefix(path, `\\`) || strings.ContainsAny(path, "\x00\r\n") {
			return
		}
		path = filepath.Clean(path)
		key := strings.ToLower(path)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, discoveryPath{path, origin})
	}
	if filepath.IsAbs(in.localAppData) {
		add(filepath.Join(in.localAppData, "Programs", "neko", "neko.exe"), "installed")
	}
	if filepath.IsAbs(in.executable) {
		for _, name := range []string{"neko-core-native.exe", "neko-core.exe", "neko.exe"} {
			add(filepath.Join(filepath.Dir(in.executable), name), "bundled_candidate")
		}
	}
	canExe := in.pathExt == ""
	for i, extension := range strings.Split(in.pathExt, ";") {
		if i >= 16 {
			limited = true
			break
		}
		if strings.EqualFold(strings.TrimSpace(extension), ".exe") {
			canExe = true
		}
	}
	if canExe {
		for i, dir := range filepath.SplitList(in.path) {
			if i >= discoveryMaxDirectories {
				limited = true
				break
			}
			dir = strings.Trim(strings.TrimSpace(dir), `"`)
			for _, name := range []string{"neko.exe", "neko-core.exe"} {
				add(filepath.Join(dir, name), "path")
			}
		}
	}
	return out, limited
}
func discoveryRegularPath(path string) bool {
	if !discoveryLocalVolume(path) {
		return false
	}
	ancestors := []string{}
	for current := path; ; current = filepath.Dir(current) {
		if len(ancestors) >= 128 {
			return false
		}
		ancestors = append(ancestors, current)
		if filepath.Dir(current) == current {
			break
		}
	}
	for i := len(ancestors) - 1; i >= 0; i-- {
		info, err := os.Lstat(ancestors[i])
		if err != nil || discoveryReparse(info) {
			return false
		}
		if i == 0 {
			if !info.Mode().IsRegular() {
				return false
			}
		} else if !info.IsDir() {
			return false
		}
	}
	return true
}
func discoverCore(ctx context.Context, in discoveryInputs) CoreDiscovery {
	ctx, cancel := context.WithTimeout(ctx, discoveryTimeout)
	defer cancel()
	out := CoreDiscovery{Version: 1, Mode: "read_only_discovery", Status: "not_detected", Candidates: []CoreDiscoveryCandidate{}, Usability: "not_evaluated", ConfigInspection: "not_performed", CredentialAccess: "not_requested", Execution: "not_started"}
	paths, limited := discoveryPaths(in)
	out.Limited = limited
	budget := discoveryMaxTotalBytes
	for _, target := range paths {
		if ctx.Err() != nil {
			out.Limited = true
			break
		}
		if !discoveryRegularPath(target.path) {
			continue
		}
		before, err := os.Lstat(target.path)
		if err != nil {
			continue
		}
		h := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(target.path))))
		candidate := CoreDiscoveryCandidate{ID: "core-candidate-" + hex.EncodeToString(h[:16]), Origin: target.origin, Path: target.path, FileBytes: before.Size(), Status: "metadata_unavailable", Reason: "metadata_unavailable"}
		if before.Size() < 1 || before.Size() > discoveryMaxFileBytes || before.Size() > budget {
			candidate.Reason = "discovery_budget"
			out.Limited = true
			out.Candidates = append(out.Candidates, candidate)
			continue
		}
		file, e := os.Open(target.path)
		if e != nil {
			out.Candidates = append(out.Candidates, candidate)
			continue
		}
		stopClose := context.AfterFunc(ctx, func() { file.Close() })
		opened, e := file.Stat()
		if e != nil || !os.SameFile(before, opened) || !opened.Mode().IsRegular() {
			stopClose()
			file.Close()
			candidate.Reason = "file_changed"
			out.Candidates = append(out.Candidates, candidate)
			continue
		}
		if !discoveryPEPreflight(file, opened.Size()) {
			stopClose()
			file.Close()
			candidate.Reason = "not_pe_executable"
			out.Candidates = append(out.Candidates, candidate)
			continue
		}
		binary, e := pe.NewFile(file)
		if e != nil || binary.Machine != pe.IMAGE_FILE_MACHINE_AMD64 || binary.Characteristics&pe.IMAGE_FILE_EXECUTABLE_IMAGE == 0 || binary.Characteristics&pe.IMAGE_FILE_DLL != 0 {
			stopClose()
			if binary != nil {
				binary.Close()
			}
			file.Close()
			candidate.Reason = "not_pe_executable"
			out.Candidates = append(out.Candidates, candidate)
			continue
		}
		candidate.MetadataVersion = discoveryPEVersion(file, binary)
		digest, bytes, e := discoveryHash(ctx, file, opened.Size())
		budget -= bytes
		after, statErr := file.Stat()
		current, pathErr := os.Lstat(target.path)
		stopClose()
		binary.Close()
		file.Close()
		if e != nil {
			if ctx.Err() != nil {
				candidate.Reason = "discovery_budget"
				out.Limited = true
			} else if e == io.ErrUnexpectedEOF {
				candidate.Reason = "file_changed"
			} else {
				candidate.Reason = "metadata_unavailable"
			}
		} else if statErr != nil || pathErr != nil || !os.SameFile(opened, after) || !os.SameFile(after, current) || opened.Size() != after.Size() || after.Size() != current.Size() || !opened.ModTime().Equal(after.ModTime()) || !after.ModTime().Equal(current.ModTime()) || !discoveryRegularPath(target.path) {
			candidate.Reason = "file_changed"
		} else {
			candidate.SHA256 = digest
			candidate.Status = "unrecognized_runtime"
			candidate.Reason = "unrecognized_digest"
			if digest == in.pin.SHA256 && in.pin.ContextProtocol == 1 && !in.pin.HistoricalReplayOnly {
				candidate.Status = "compatible_pinned"
				candidate.Reason = ""
				candidate.Compatible = true
				candidate.PinnedVersion = in.pin.Version
				candidate.ContextProtocol = in.pin.ContextProtocol
				candidate.ManifestVersions = append([]int(nil), in.pin.ManifestVersions...)
				if out.RecommendedCandidateID == "" {
					out.RecommendedCandidateID = candidate.ID
				}
			} else {
				recognizeLegacyCore(&candidate)
			}
		}
		out.Candidates = append(out.Candidates, candidate)
	}
	if len(out.Candidates) > 0 {
		out.Status = "detected_unusable"
	}
	if out.RecommendedCandidateID != "" {
		out.Status = "compatible_core_found"
	}
	return out
}
func recognizeLegacyCore(candidate *CoreDiscoveryCandidate) {
	if candidate.SHA256 != "c760a31fad34a27215353e77f39dc654d0c566b1d7ad47ea811fe76eeb2f4990" || candidate.FileBytes != 93967360 {
		return
	}
	candidate.Status = "recognized_incompatible"
	candidate.Reason = "host_profile_unsupported"
	candidate.RecognizedVersion = "1.7.0"
	candidate.RecognitionSource = "verified_wiii_bundlelock"
	candidate.SourceRevision = "16860b031197b0df5ecf3613b72df7e10de4981e"
}
func discoveryHash(ctx context.Context, file *os.File, size int64) (string, int64, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", 0, err
	}
	hash := sha256.New()
	buffer := make([]byte, 64<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return "", total, err
		}
		remaining := size + 1 - total
		if remaining <= 0 {
			return "", total, io.ErrUnexpectedEOF
		}
		part := buffer
		if int64(len(part)) > remaining {
			part = part[:remaining]
		}
		n, err := file.Read(part)
		if n > 0 {
			total += int64(n)
			if total > size {
				return "", total, io.ErrUnexpectedEOF
			}
			hash.Write(part[:n])
		}
		if err == io.EOF {
			if total != size {
				return "", total, io.ErrUnexpectedEOF
			}
			return hex.EncodeToString(hash.Sum(nil)), total, nil
		}
		if err != nil {
			return "", total, err
		}
		if n == 0 {
			return "", total, io.ErrNoProgress
		}
	}
}

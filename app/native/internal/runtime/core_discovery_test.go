package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/pe"
	"encoding/binary"
	"encoding/hex"
	"io"
	"nekoaurora.local/native/internal/protocol"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func discoveryFixturePE(t *testing.T) []byte {
	t.Helper()
	out := make([]byte, 1024)
	copy(out, "MZ")
	binary.LittleEndian.PutUint32(out[60:], 128)
	var header bytes.Buffer
	header.Write([]byte{'P', 'E', 0, 0})
	binary.Write(&header, binary.LittleEndian, pe.FileHeader{Machine: pe.IMAGE_FILE_MACHINE_AMD64, NumberOfSections: 1, SizeOfOptionalHeader: 240, Characteristics: pe.IMAGE_FILE_EXECUTABLE_IMAGE})
	optional := pe.OptionalHeader64{Magic: 0x20b, ImageBase: 0x140000000, SectionAlignment: 0x1000, FileAlignment: 0x200, SizeOfImage: 0x2000, SizeOfHeaders: 0x200, NumberOfRvaAndSizes: 16}
	optional.DataDirectory[2] = pe.DataDirectory{VirtualAddress: 0x1000, Size: 512}
	binary.Write(&header, binary.LittleEndian, optional)
	section := pe.SectionHeader32{VirtualSize: 512, VirtualAddress: 0x1000, SizeOfRawData: 512, PointerToRawData: 512, Characteristics: 0x40000040}
	copy(section.Name[:], ".rsrc")
	binary.Write(&header, binary.LittleEndian, section)
	copy(out[128:], header.Bytes())
	resource := out[512:]
	for _, offset := range []int{0, 32, 64} {
		binary.LittleEndian.PutUint16(resource[offset+14:], 1)
	}
	binary.LittleEndian.PutUint32(resource[16:], 16)
	binary.LittleEndian.PutUint32(resource[20:], 0x80000020)
	binary.LittleEndian.PutUint32(resource[48:], 1)
	binary.LittleEndian.PutUint32(resource[52:], 0x80000040)
	binary.LittleEndian.PutUint32(resource[80:], 1033)
	binary.LittleEndian.PutUint32(resource[84:], 96)
	binary.LittleEndian.PutUint32(resource[96:], 0x1080)
	binary.LittleEndian.PutUint32(resource[100:], 92)
	version := resource[128:]
	binary.LittleEndian.PutUint16(version, 92)
	binary.LittleEndian.PutUint16(version[2:], 52)
	for i, r := range "VS_VERSION_INFO" {
		binary.LittleEndian.PutUint16(version[6+i*2:], uint16(r))
	}
	binary.LittleEndian.PutUint32(version[40:], 0xfeef04bd)
	binary.LittleEndian.PutUint32(version[44:], 0x10000)
	binary.LittleEndian.PutUint32(version[48:], 1<<16|8)
	return out
}
func discoveryWritePE(t *testing.T, path string, raw []byte) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
}
func TestCoreDiscoveryKnownHashUnknownVersionAndNoInstalled(t *testing.T) {
	dir := t.TempDir()
	raw := discoveryFixturePE(t)
	hash := sha256.Sum256(raw)
	digest := hex.EncodeToString(hash[:])
	installed := filepath.Join(dir, "Programs", "neko", "neko.exe")
	discoveryWritePE(t, installed, raw)
	pin := protocol.CoreDiscoveryPin{Version: "1.8.0", SHA256: digest, ContextProtocol: 1, ManifestVersions: []int{1, 2}}
	in := discoveryInputs{localAppData: dir, pin: pin}
	got := discoverCore(context.Background(), in)
	if got.Status != "compatible_core_found" || got.Usability != "not_evaluated" || got.Execution != "not_started" || got.CredentialAccess != "not_requested" || len(got.Candidates) != 1 || got.RecommendedCandidateID != got.Candidates[0].ID || got.Candidates[0].MetadataVersion != "1.8.0.0" || got.Candidates[0].SHA256 != digest || !got.Candidates[0].Compatible {
		t.Fatal("pinned metadata not discovered", got.Status)
	}
	in.pin.SHA256 = strings.Repeat("a", 64)
	unknown := discoverCore(context.Background(), in)
	if unknown.Status != "detected_unusable" || unknown.Candidates[0].Status != "unrecognized_runtime" || unknown.Candidates[0].Compatible || unknown.RecommendedCandidateID != "" || unknown.Candidates[0].PinnedVersion != "" {
		t.Fatal("version metadata acquired runtime authority")
	}
	in.pin = pin
	in.pin.HistoricalReplayOnly = true
	historical := discoverCore(context.Background(), in)
	if historical.Candidates[0].Compatible {
		t.Fatal("historical pin became fresh compatible")
	}
	empty := discoverCore(context.Background(), discoveryInputs{localAppData: t.TempDir(), pin: pin})
	if empty.Status != "not_detected" || len(empty.Candidates) != 0 || empty.RecommendedCandidateID != "" {
		t.Fatal("fabricated installation")
	}
	after, e := os.ReadFile(installed)
	if e != nil || !bytes.Equal(after, raw) {
		t.Fatal("discovery changed executable")
	}
}
func TestCoreDiscoveryLegacyFingerprintNeverRuntimeGrant(t *testing.T) {
	const legacy = "c760a31fad34a27215353e77f39dc654d0c566b1d7ad47ea811fe76eeb2f4990"
	candidate := CoreDiscoveryCandidate{SHA256: legacy, FileBytes: 93967360, Status: "unrecognized_runtime"}
	recognizeLegacyCore(&candidate)
	if candidate.Status != "recognized_incompatible" || candidate.Reason != "host_profile_unsupported" || candidate.RecognizedVersion != "1.7.0" || candidate.RecognitionSource != "verified_wiii_bundlelock" || candidate.Compatible || protocol.CoreRuntimeAllowed(legacy) || protocol.CurrentWindowsCoreDiscoveryPin().SHA256 == legacy {
		t.Fatal("legacy recognition changed execution allowlist")
	}
	wrong := CoreDiscoveryCandidate{SHA256: legacy, FileBytes: 93967361, Status: "unrecognized_runtime"}
	recognizeLegacyCore(&wrong)
	if wrong.Status != "unrecognized_runtime" {
		t.Fatal("legacy recognition ignored immutable bytes")
	}
}
func TestCoreDiscoveryBoundsMalformedPEAndGrowth(t *testing.T) {
	dir := t.TempDir()
	raw := discoveryFixturePE(t)
	path := filepath.Join(dir, "neko.exe")
	discoveryWritePE(t, path, raw)
	file, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer file.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, n, e := discoveryHash(ctx, file, int64(len(raw))); e == nil || n != 0 {
		t.Fatal("cancelled discovery read bytes")
	}
	if _, n, e := discoveryHash(context.Background(), file, int64(len(raw)-1)); e != io.ErrUnexpectedEOF || n > int64(len(raw)) {
		t.Fatal("file growth read escaped pinned size")
	}
	malformed := append([]byte{}, raw...)
	binary.LittleEndian.PutUint32(malformed[128+4+12:], 0xffffffff)
	if discoveryPEPreflight(bytes.NewReader(malformed), int64(len(malformed))) {
		t.Fatal("unbounded COFF allocation accepted")
	}
	dirs := []string{}
	for i := 0; i < 20; i++ {
		dirs = append(dirs, filepath.Join(dir, strings.Repeat("a", i+1)))
	}
	paths, limited := discoveryPaths(discoveryInputs{path: strings.Join(dirs, string(os.PathListSeparator)), pathExt: ".EXE;.CMD;.BAT", localAppData: dir, executable: filepath.Join(dir, "control.exe")})
	if !limited || len(paths) > discoveryMaxCandidates {
		t.Fatal("PATH directory budget escaped")
	}
	for _, p := range paths {
		if !strings.HasSuffix(p.path, ".exe") {
			t.Fatal("discovery admitted scripts")
		}
	}
	large := filepath.Join(dir, "Programs", "neko", "neko.exe")
	discoveryWritePE(t, large, raw)
	g, e := os.OpenFile(large, os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	if e = g.Truncate(discoveryMaxFileBytes + 1); e != nil {
		g.Close()
		t.Fatal(e)
	}
	g.Close()
	got := discoverCore(context.Background(), discoveryInputs{localAppData: dir})
	if !got.Limited || len(got.Candidates) != 1 || got.Candidates[0].SHA256 != "" || got.Candidates[0].Reason != "discovery_budget" {
		t.Fatal("large file hashed or budget hidden")
	}
}
func TestCoreDiscoveryFinitePathsAndSymlinkAncestor(t *testing.T) {
	dir := t.TempDir()
	raw := discoveryFixturePE(t)
	real := filepath.Join(dir, "real")
	target := filepath.Join(real, "neko.exe")
	discoveryWritePE(t, target, raw)
	paths, _ := discoveryPaths(discoveryInputs{path: strings.Join([]string{dir, dir, "relative", `\\server\share`}, string(os.PathListSeparator)), pathExt: ".EXE"})
	seen := map[string]bool{}
	for _, p := range paths {
		if seen[p.path] || !filepath.IsAbs(p.path) || strings.HasPrefix(p.path, `\\`) {
			t.Fatal("duplicate/relative/network path admitted")
		}
		seen[p.path] = true
	}
	link := filepath.Join(dir, "linked")
	if e := os.Symlink(real, link); e != nil {
		t.Skip("directory symlink unavailable; path and budget checks passed")
	}
	if discoveryRegularPath(filepath.Join(link, "neko.exe")) {
		t.Fatal("reparse ancestor accepted")
	}
}

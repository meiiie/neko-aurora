//go:build windows

package statebackup

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"unsafe"
)

func TestWindowsPlatformDPAPIRoundTripKeepsCallerBuffers(t *testing.T) {
	plain := []byte("owned synthetic backup fixture 猫")
	entropy := []byte("owned backup fixture domain")
	original := append([]byte(nil), plain...)
	originalEntropy := append([]byte(nil), entropy...)
	cipher, err := protect(plain, entropy)
	if err != nil {
		t.Fatal("genuine current-user DPAPI protect failed", err)
	}
	if len(cipher) == 0 || bytes.Equal(cipher, plain) || bytes.Contains(cipher, plain) {
		t.Fatal("DPAPI did not protect the fixture")
	}
	decoded, err := unprotect(cipher, entropy)
	if err != nil {
		t.Fatal("genuine current-user DPAPI unprotect failed", err)
	}
	defer platformScrubBytes(decoded)
	if !bytes.Equal(decoded, original) || !bytes.Equal(plain, original) || !bytes.Equal(entropy, originalEntropy) {
		t.Fatal("round trip changed caller buffers or returned different data")
	}
}

func TestWindowsPlatformDPAPIRejectsWrongEntropyAndTamper(t *testing.T) {
	cipher, err := protect([]byte("synthetic backup data only"), []byte("fixture entropy one"))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := unprotect(cipher, []byte("fixture entropy two"))
	if !errors.Is(err, ErrProtection) || decoded != nil {
		platformScrubBytes(decoded)
		t.Fatal("wrong entropy must reject without plaintext")
	}
	for _, index := range []int{0, len(cipher) / 2, len(cipher) - 1} {
		changed := append([]byte(nil), cipher...)
		changed[index] ^= 1
		decoded, err = unprotect(changed, []byte("fixture entropy one"))
		if !errors.Is(err, ErrProtection) || decoded != nil {
			platformScrubBytes(decoded)
			t.Fatal("tampered ciphertext must reject without plaintext")
		}
	}
}

func TestWindowsPlatformDPAPIEmptyPlaintextAndEntropy(t *testing.T) {
	cipher, err := protect(nil, nil)
	if err != nil || len(cipher) == 0 {
		t.Fatal("DPAPI must protect empty plaintext without optional entropy", err)
	}
	decoded, err := unprotect(cipher, nil)
	if err != nil || len(decoded) != 0 {
		platformScrubBytes(decoded)
		t.Fatal("empty plaintext round trip failed", err)
	}
	platformScrubBytes(decoded)
	decoded, err = unprotect(nil, nil)
	if !errors.Is(err, ErrProtection) || decoded != nil {
		t.Fatal("empty ciphertext must reject")
	}
}

func TestWindowsPlatformProtectionBudgetAndScrubbing(t *testing.T) {
	tooLarge := make([]byte, 65<<20+1)
	for _, encode := range []func([]byte, []byte) ([]byte, error){protect, unprotect} {
		result, err := encode(tooLarge, nil)
		if !errors.Is(err, ErrProtection) || result != nil {
			t.Fatal("oversized input must reject without output")
		}
	}
	platformScrubBytes(tooLarge)
	plain := []byte{1, 2, 3, 4}
	platformScrubBytes(plain)
	if !bytes.Equal(plain, make([]byte, 4)) {
		t.Fatal("owned plaintext buffer was not scrubbed")
	}
}

func TestWindowsPlatformPinnedFileExcludesWriteDeleteAndRename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owned-fixture.bin")
	if err := os.WriteFile(path, []byte("immutable owned fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	pinned, err := openPinned(path)
	if err != nil {
		t.Fatal("safe regular fixture was not pinned", err)
	}
	defer pinned.Close()
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal("read sharing must remain allowed", err)
	}
	reader.Close()
	writer, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0600)
	if err == nil {
		writer.Close()
		t.Fatal("pinned handle allowed write/truncate")
	}
	if err := os.Remove(path); err == nil {
		t.Fatal("pinned handle allowed deletion")
	}
	if err := os.Rename(path, path+".renamed"); err == nil {
		t.Fatal("pinned handle allowed rename")
	}
	data, err := io.ReadAll(pinned)
	if err != nil || string(data) != "immutable owned fixture" {
		t.Fatal("pinned fixture changed")
	}
	if err := pinned.Close(); err != nil {
		t.Fatal(err)
	}
	writer, err = os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("closing pinned handle did not release write exclusion", err)
	}
	writer.Close()
}

func TestWindowsPlatformPinnedFileRejectsExistingWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "writer.bin")
	writer, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	file, err := openPinned(path)
	if file != nil {
		file.Close()
	}
	if !errors.Is(err, ErrUnsafe) || file != nil {
		t.Fatal("pin must fail while a writer already owns the file")
	}
}

func TestWindowsPlatformPinnedFileRejectsHardLinksAndDirectories(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "one.bin")
	if err := os.WriteFile(path, []byte("owned hardlink fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "two.bin")
	if err := os.Link(path, link); err != nil {
		t.Fatal("genuine owned hardlink fixture unavailable", err)
	}
	for _, path := range []string{path, link, directory} {
		file, err := openPinned(path)
		if file != nil {
			file.Close()
		}
		if !errors.Is(err, ErrUnsafe) || file != nil {
			t.Fatal("unsafe file type or hardlink was accepted")
		}
	}
}

func TestWindowsPlatformPinnedFileRejectsReparsePoint(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "target.bin")
	if err := os.WriteFile(path, []byte("owned link fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "link.bin")
	if err := os.Symlink(path, link); err != nil {
		t.Skip("genuine symlink fixture unavailable; reparse-point acceptance remains unverified")
	}
	file, err := openPinned(link)
	if file != nil {
		file.Close()
	}
	if !errors.Is(err, ErrUnsafe) || file != nil {
		t.Fatal("reparse point was accepted")
	}
}

func TestWindowsPlatformPinnedFileRejectsRemoteDeviceAndADSPaths(t *testing.T) {
	for _, path := range []string{`\\server.invalid\share\file`, `\\.\pipe\owned-fixture`, "relative.bin", `C:\owned-fixture:stream`, "C:\\nul\x00.bin"} {
		file, err := openPinned(path)
		if file != nil {
			file.Close()
		}
		if !errors.Is(err, ErrUnsafe) || file != nil {
			t.Fatal("unsafe path form was accepted")
		}
	}
}

func TestWindowsPlatformProcessInactiveIsFailClosed(t *testing.T) {
	if err := processInactive(0); err != nil {
		t.Fatal("zero PID must be inactive", err)
	}
	if err := processInactive(^uint32(0)); err != nil {
		t.Fatal("invalid nonexistent PID must be inactive", err)
	}
	if !errors.Is(processInactive(uint32(os.Getpid())), ErrLive) {
		t.Fatal("genuine owned current process must be live")
	}
	if !errors.Is(processInactive(4), ErrLive) {
		t.Fatal("live or access-denied system process must be treated as live")
	}
}

func TestWindowsPlatformCurrentUserEvidence(t *testing.T) {
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal("current-user token evidence unavailable", err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal("current-user token evidence unavailable", err)
	}
	sid, err := user.User.Sid.String()
	if err != nil {
		t.Fatal("current-user token evidence unavailable", err)
	}
	identityHash := sha256.Sum256([]byte(sid))
	t.Logf("current Windows user SID SHA-256: %x; no cross-user or cross-machine acceptance claimed", identityHash)
}

func TestWindowsPlatformDirectorySafeChecksTypeAndReparse(t *testing.T) {
	directory := t.TempDir()
	if err := directorySafe(directory); err != nil {
		t.Fatal("owned ordinary directory rejected", err)
	}
	file := filepath.Join(directory, "owned-file.bin")
	if err := os.WriteFile(file, []byte("owned fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(directorySafe(file), ErrUnsafe) {
		t.Fatal("ordinary file accepted as safe directory")
	}
	if !errors.Is(directorySafe("relative-dir"), ErrUnsafe) {
		t.Fatal("relative directory accepted")
	}
	link := filepath.Join(directory, "directory-link")
	if err := os.Symlink(directory, link); err != nil {
		t.Skip("genuine directory reparse fixture unavailable; directory reparse case remains unverified")
	}
	if !errors.Is(directorySafe(link), ErrUnsafe) {
		t.Fatal("directory reparse point accepted")
	}
}

type platformTestACLObservation struct {
	control        uint16
	aceCount       uint16
	aceType        byte
	aceFlags       byte
	mask           uint32
	currentOwner   bool
	currentTrustee bool
	descriptorHash [32]byte
}

func platformObserveACL(t *testing.T, path string) platformTestACLObservation {
	t.Helper()
	advapi := syscall.NewLazyDLL("advapi32.dll")
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal("ACL fixture path invalid")
	}
	var owner *syscall.SID
	var acl, descriptor *byte
	result, _, _ := advapi.NewProc("GetNamedSecurityInfoW").Call(uintptr(unsafe.Pointer(name)), 1, 5, uintptr(unsafe.Pointer(&owner)), 0, uintptr(unsafe.Pointer(&acl)), 0, uintptr(unsafe.Pointer(&descriptor)))
	if result != 0 || descriptor == nil || owner == nil || acl == nil {
		t.Fatal("cannot read actual fixture security descriptor")
	}
	size, _, _ := advapi.NewProc("GetSecurityDescriptorLength").Call(uintptr(unsafe.Pointer(descriptor)))
	if size == 0 || size > 1<<20 {
		syscall.LocalFree(syscall.Handle(uintptr(unsafe.Pointer(descriptor))))
		t.Fatal("fixture security descriptor size invalid")
	}
	defer func() {
		platformScrubBytes(unsafe.Slice(descriptor, int(size)))
		syscall.LocalFree(syscall.Handle(uintptr(unsafe.Pointer(descriptor))))
	}()
	var observation platformTestACLObservation
	observation.descriptorHash = sha256.Sum256(unsafe.Slice(descriptor, int(size)))
	var revision uint32
	result, _, _ = advapi.NewProc("GetSecurityDescriptorControl").Call(uintptr(unsafe.Pointer(descriptor)), uintptr(unsafe.Pointer(&observation.control)), uintptr(unsafe.Pointer(&revision)))
	if result == 0 {
		t.Fatal("cannot read actual descriptor control flags")
	}
	aclHeader := unsafe.Slice(acl, 8)
	observation.aceCount = binary.LittleEndian.Uint16(aclHeader[4:6])
	if observation.aceCount == 0 {
		t.Fatal("fixture has no discretionary ACE")
	}
	var ace *byte
	result, _, _ = advapi.NewProc("GetAce").Call(uintptr(unsafe.Pointer(acl)), 0, uintptr(unsafe.Pointer(&ace)))
	if result == 0 || ace == nil {
		t.Fatal("cannot read actual access ACE")
	}
	header := unsafe.Slice(ace, 8)
	if binary.LittleEndian.Uint16(header[2:4]) < 16 {
		t.Fatal("fixture access ACE size invalid")
	}
	observation.aceType, observation.aceFlags = header[0], header[1]
	observation.mask = binary.LittleEndian.Uint32(header[4:8])
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal("cannot query existing current-user token")
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		t.Fatal("cannot query existing current-user SID")
	}
	result, _, _ = advapi.NewProc("EqualSid").Call(uintptr(unsafe.Pointer(owner)), uintptr(unsafe.Pointer(user.User.Sid)))
	observation.currentOwner = result != 0
	result, _, _ = advapi.NewProc("EqualSid").Call(uintptr(unsafe.Add(unsafe.Pointer(ace), 8)), uintptr(unsafe.Pointer(user.User.Sid)))
	observation.currentTrustee = result != 0
	return observation
}

func TestWindowsPlatformPrivateDirectoryUsesProtectedCurrentUserDACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new-private-root")
	if err := makePrivateDirectory(path); err != nil {
		t.Fatal("private destination creation failed", err)
	}
	observed := platformObserveACL(t, path)
	if observed.control&0x1000 == 0 || !observed.currentOwner || observed.aceCount != 1 || observed.aceType != 0 || observed.aceFlags&0x03 != 0x03 || observed.aceFlags&0x10 != 0 || observed.mask != 0x001f01ff || !observed.currentTrustee {
		t.Fatal("new root must have protected DACL, current-user ownership, and exactly one inheritable current-user full-access ACE")
	}
	subdir := filepath.Join(path, "data")
	if err := os.Mkdir(subdir, 0700); err != nil {
		t.Fatal("current user cannot create an inheriting subdirectory")
	}
	file := filepath.Join(subdir, "fixture.txt")
	if err := os.WriteFile(file, []byte("private owned fixture"), 0600); err != nil {
		t.Fatal("current user cannot write an inheriting file")
	}
	for _, item := range []string{subdir, file} {
		child := platformObserveACL(t, item)
		if child.aceCount != 1 || child.aceType != 0 || child.aceFlags&0x10 == 0 || child.mask != 0x001f01ff || !child.currentTrustee {
			t.Fatal("new descendants must inherit only the current-user access ACE")
		}
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "private owned fixture" {
		t.Fatal("current user cannot read private descendant fixture")
	}
}

func TestWindowsPlatformPrivateDirectoryRefusesExistingWithoutACLChanges(t *testing.T) {
	path := t.TempDir()
	file := filepath.Join(path, "unchanged.txt")
	if err := os.WriteFile(file, []byte("unchanged owned fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	before := platformObserveACL(t, path)
	if !errors.Is(makePrivateDirectory(path), ErrUnsafe) {
		t.Fatal("existing destination must be refused")
	}
	after := platformObserveACL(t, path)
	if before.descriptorHash != after.descriptorHash {
		t.Fatal("existing directory security descriptor changed")
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "unchanged owned fixture" {
		t.Fatal("existing directory fixture changed")
	}
}

//go:build windows

package statebackup

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

const platformMaxProtectionBytes = 65 << 20
const platformMaxEntropyBytes = 1 << 20

var platformKernel32 = syscall.NewLazyDLL("kernel32.dll")
var platformLoadLibraryEx = platformKernel32.NewProc("LoadLibraryExW")

type platformDataBlob struct {
	Size uint32
	Data *byte
}

func protect(data, entropy []byte) ([]byte, error) {
	return platformCrypt(data, entropy, true)
}

func unprotect(data, entropy []byte) ([]byte, error) {
	return platformCrypt(data, entropy, false)
}

func platformCrypt(data, entropy []byte, seal bool) (result []byte, err error) {
	if len(data) > platformMaxProtectionBytes || len(entropy) > platformMaxEntropyBytes || !seal && len(data) == 0 {
		return nil, ErrProtection
	}
	if platformLoadLibraryEx.Find() != nil {
		return nil, ErrProtection
	}
	libraryName, _ := syscall.UTF16PtrFromString("crypt32.dll")
	module, _, _ := platformLoadLibraryEx.Call(uintptr(unsafe.Pointer(libraryName)), 0, 0x00000800)
	if module == 0 {
		return nil, ErrProtection
	}
	defer syscall.FreeLibrary(syscall.Handle(module))
	name := "CryptUnprotectData"
	if seal {
		name = "CryptProtectData"
	}
	procedure, lookupErr := syscall.GetProcAddress(syscall.Handle(module), name)
	if lookupErr != nil {
		return nil, ErrProtection
	}
	inputCopy := make([]byte, max(1, len(data)))
	copy(inputCopy, data)
	entropyCopy := append([]byte(nil), entropy...)
	defer platformScrubBytes(inputCopy)
	defer platformScrubBytes(entropyCopy)
	input := platformDataBlob{Size: uint32(len(data)), Data: &inputCopy[0]}
	optionalEntropy := platformDataBlob{Size: uint32(len(entropyCopy))}
	var entropyPointer *platformDataBlob
	if len(entropyCopy) != 0 {
		optionalEntropy.Data = &entropyCopy[0]
		entropyPointer = &optionalEntropy
	}
	var output platformDataBlob
	defer func() {
		if output.Data != nil {
			platformScrubBytes(unsafe.Slice(output.Data, int(output.Size)))
			freed, freeErr := syscall.LocalFree(syscall.Handle(uintptr(unsafe.Pointer(output.Data))))
			if freeErr != nil || freed != 0 {
				platformScrubBytes(result)
				result, err = nil, ErrProtection
			}
			output.Data, output.Size = nil, 0
		}
	}()
	success, _, _ := syscall.SyscallN(procedure,
		uintptr(unsafe.Pointer(&input)), 0, uintptr(unsafe.Pointer(entropyPointer)), 0, 0, 1,
		uintptr(unsafe.Pointer(&output)))
	runtime.KeepAlive(inputCopy)
	runtime.KeepAlive(entropyCopy)
	runtime.KeepAlive(optionalEntropy)
	if success == 0 || output.Size > platformMaxProtectionBytes || output.Data == nil && output.Size != 0 || seal && output.Size == 0 {
		return nil, ErrProtection
	}
	result = make([]byte, int(output.Size))
	if len(result) != 0 {
		copy(result, unsafe.Slice(output.Data, len(result)))
	}
	return result, nil
}

func platformLocalPath(path string) bool {
	if len(path) == 0 || len(path) > 32767 || !filepath.IsAbs(path) || strings.ContainsRune(path, '\x00') {
		return false
	}
	volume := filepath.VolumeName(path)
	return len(volume) == 2 && volume[1] == ':' && !strings.Contains(path[len(volume):], ":")
}

func openPinned(path string) (*os.File, error) {
	if !platformLocalPath(path) {
		return nil, ErrUnsafe
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, ErrUnsafe
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, ErrUnsafe
	}
	var info syscall.ByHandleFileInformation
	kind, typeErr := syscall.GetFileType(handle)
	infoErr := syscall.GetFileInformationByHandle(handle, &info)
	if typeErr != nil || infoErr != nil || kind != syscall.FILE_TYPE_DISK || info.FileAttributes&(syscall.FILE_ATTRIBUTE_DIRECTORY|syscall.FILE_ATTRIBUTE_REPARSE_POINT|0x00000040) != 0 || info.NumberOfLinks != 1 {
		syscall.CloseHandle(handle)
		return nil, ErrUnsafe
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		syscall.CloseHandle(handle)
		return nil, ErrUnsafe
	}
	return file, nil
}

func processInactive(pid uint32) error {
	if pid == 0 {
		return nil
	}
	handle, err := syscall.OpenProcess(syscall.SYNCHRONIZE|0x00001000, false, pid)
	if err == syscall.Errno(87) {
		return nil
	}
	if err != nil {
		return ErrLive
	}
	defer syscall.CloseHandle(handle)
	status, err := syscall.WaitForSingleObject(handle, 0)
	if err != nil || status != syscall.WAIT_OBJECT_0 {
		return ErrLive
	}
	return nil
}

func platformScrubBytes(data []byte) {
	clear(data)
	runtime.KeepAlive(data)
}

func directorySafe(path string) error {
	if !platformLocalPath(path) {
		return ErrUnsafe
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return ErrUnsafe
	}
	attributes, err := syscall.GetFileAttributes(name)
	if err != nil || attributes&syscall.FILE_ATTRIBUTE_DIRECTORY == 0 || attributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return ErrUnsafe
	}
	return nil
}

func makePrivateDirectory(path string) (err error) {
	if !platformLocalPath(path) {
		return ErrUnsafe
	}
	root, _ := syscall.UTF16PtrFromString(filepath.VolumeName(path) + `\`)
	var serial, maximumComponent, volumeFlags uint32
	volumeInformation := platformKernel32.NewProc("GetVolumeInformationW")
	if volumeInformation.Find() != nil {
		return ErrProtection
	}
	success, _, _ := volumeInformation.Call(uintptr(unsafe.Pointer(root)), 0, 0,
		uintptr(unsafe.Pointer(&serial)), uintptr(unsafe.Pointer(&maximumComponent)),
		uintptr(unsafe.Pointer(&volumeFlags)), 0, 0)
	if success == 0 {
		return ErrProtection
	}
	if volumeFlags&0x00000008 == 0 {
		return ErrUnsupported
	}
	token, tokenErr := syscall.OpenCurrentProcessToken()
	if tokenErr != nil {
		return ErrProtection
	}
	defer func() {
		if token.Close() != nil {
			err = ErrProtection
		}
	}()
	user, userErr := token.GetTokenUser()
	if userErr != nil {
		return ErrProtection
	}
	sid, sidErr := user.User.Sid.String()
	if sidErr != nil || len(sid) > 184 {
		return ErrProtection
	}
	sddl, sddlErr := syscall.UTF16FromString("O:" + sid + "D:P(A;OICI;FA;;;" + sid + ")")
	if sddlErr != nil {
		return ErrProtection
	}
	defer func() {
		clear(sddl)
		runtime.KeepAlive(sddl)
	}()
	libraryName, _ := syscall.UTF16PtrFromString("advapi32.dll")
	module, _, _ := platformLoadLibraryEx.Call(uintptr(unsafe.Pointer(libraryName)), 0, 0x00000800)
	if module == 0 {
		return ErrProtection
	}
	defer syscall.FreeLibrary(syscall.Handle(module))
	convert, lookupErr := syscall.GetProcAddress(syscall.Handle(module), "ConvertStringSecurityDescriptorToSecurityDescriptorW")
	if lookupErr != nil {
		return ErrProtection
	}
	var descriptor *byte
	var descriptorSize uint32
	defer func() {
		if descriptor != nil {
			platformScrubBytes(unsafe.Slice(descriptor, int(descriptorSize)))
			freed, freeErr := syscall.LocalFree(syscall.Handle(uintptr(unsafe.Pointer(descriptor))))
			if freeErr != nil || freed != 0 {
				err = ErrProtection
			}
		}
	}()
	success, _, _ = syscall.SyscallN(convert, uintptr(unsafe.Pointer(&sddl[0])), 1,
		uintptr(unsafe.Pointer(&descriptor)), uintptr(unsafe.Pointer(&descriptorSize)))
	runtime.KeepAlive(sddl)
	if success == 0 || descriptor == nil || descriptorSize == 0 || descriptorSize > 4096 {
		return ErrProtection
	}
	name, nameErr := syscall.UTF16PtrFromString(path)
	if nameErr != nil {
		return ErrUnsafe
	}
	attributes := syscall.SecurityAttributes{Length: uint32(unsafe.Sizeof(syscall.SecurityAttributes{})), SecurityDescriptor: uintptr(unsafe.Pointer(descriptor))}
	if syscall.CreateDirectory(name, &attributes) != nil {
		return ErrUnsafe
	}
	runtime.KeepAlive(descriptor)
	return nil
}

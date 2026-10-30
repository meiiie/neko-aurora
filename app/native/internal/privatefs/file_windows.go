//go:build windows

package privatefs

import (
	"encoding/binary"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

var advapi = syscall.NewLazyDLL("advapi32.dll")

func currentSID() (*syscall.Tokenuser, error) {
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return nil, err
	}
	defer token.Close()
	return token.GetTokenUser()
}

func descriptor() (*byte, error) {
	u, err := currentSID()
	if err != nil {
		return nil, err
	}
	sid, err := u.User.Sid.String()
	if err != nil {
		return nil, err
	}
	text, err := syscall.UTF16PtrFromString("O:" + sid + "D:P(A;;FA;;;" + sid + ")")
	if err != nil {
		return nil, err
	}
	var sd *byte
	ok, _, err := advapi.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW").Call(uintptr(unsafe.Pointer(text)), 1, uintptr(unsafe.Pointer(&sd)), 0)
	runtime.KeepAlive(text)
	if ok == 0 {
		return nil, err
	}
	return sd, nil
}

func CreateNew(path string) (*os.File, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	sd, err := descriptor()
	if err != nil {
		return nil, err
	}
	defer syscall.LocalFree(syscall.Handle(uintptr(unsafe.Pointer(sd))))
	sa := syscall.SecurityAttributes{Length: uint32(unsafe.Sizeof(syscall.SecurityAttributes{})), SecurityDescriptor: uintptr(unsafe.Pointer(sd))}
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, syscall.FILE_SHARE_READ, &sa, syscall.CREATE_NEW, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	runtime.KeepAlive(sd)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), path)
	if err := checkHandle(h); err != nil {
		f.Close()
		os.Remove(path)
		return nil, err
	}
	return f, nil
}

func openFile(path string, access uint32) (syscall.Handle, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	h, err := syscall.CreateFile(name, access, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return 0, err
	}
	var info syscall.ByHandleFileInformation
	kind, typeErr := syscall.GetFileType(h)
	if syscall.GetFileInformationByHandle(h, &info) != nil || typeErr != nil || kind != syscall.FILE_TYPE_DISK || info.FileAttributes&(syscall.FILE_ATTRIBUTE_DIRECTORY|syscall.FILE_ATTRIBUTE_REPARSE_POINT) != 0 || info.NumberOfLinks != 1 {
		syscall.CloseHandle(h)
		return 0, ErrPrivate
	}
	return h, nil
}

func Protect(path string) error {
	h, err := openFile(path, syscall.GENERIC_READ|0x00040000|0x00080000)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(h)
	sd, err := descriptor()
	if err != nil {
		return err
	}
	defer syscall.LocalFree(syscall.Handle(uintptr(unsafe.Pointer(sd))))
	var acl, owner *byte
	var present, defaulted int32
	if ok, _, _ := advapi.NewProc("GetSecurityDescriptorDacl").Call(uintptr(unsafe.Pointer(sd)), uintptr(unsafe.Pointer(&present)), uintptr(unsafe.Pointer(&acl)), uintptr(unsafe.Pointer(&defaulted))); ok == 0 || present == 0 || acl == nil {
		return ErrPrivate
	}
	if ok, _, _ := advapi.NewProc("GetSecurityDescriptorOwner").Call(uintptr(unsafe.Pointer(sd)), uintptr(unsafe.Pointer(&owner)), uintptr(unsafe.Pointer(&defaulted))); ok == 0 || owner == nil {
		return ErrPrivate
	}
	code, _, _ := advapi.NewProc("SetSecurityInfo").Call(uintptr(h), 1, 0x80000005, uintptr(unsafe.Pointer(owner)), 0, uintptr(unsafe.Pointer(acl)), 0)
	runtime.KeepAlive(sd)
	if code != 0 {
		return syscall.Errno(code)
	}
	return checkHandle(h)
}

func Check(path string) error {
	h, err := openFile(path, syscall.GENERIC_READ)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(h)
	return checkHandle(h)
}

func checkHandle(h syscall.Handle) error {
	var owner *syscall.SID
	var acl, sd *byte
	code, _, _ := advapi.NewProc("GetSecurityInfo").Call(uintptr(h), 1, 5, uintptr(unsafe.Pointer(&owner)), 0, uintptr(unsafe.Pointer(&acl)), 0, uintptr(unsafe.Pointer(&sd)))
	if code != 0 {
		return syscall.Errno(code)
	}
	defer syscall.LocalFree(syscall.Handle(uintptr(unsafe.Pointer(sd))))
	if owner == nil || acl == nil || sd == nil {
		return ErrPrivate
	}
	var control uint16
	var revision uint32
	ok, _, _ := advapi.NewProc("GetSecurityDescriptorControl").Call(uintptr(unsafe.Pointer(sd)), uintptr(unsafe.Pointer(&control)), uintptr(unsafe.Pointer(&revision)))
	if ok == 0 || control&0x1000 == 0 || binary.LittleEndian.Uint16(unsafe.Slice(acl, 8)[4:6]) != 1 {
		return ErrPrivate
	}
	var ace *byte
	ok, _, _ = advapi.NewProc("GetAce").Call(uintptr(unsafe.Pointer(acl)), 0, uintptr(unsafe.Pointer(&ace)))
	if ok == 0 || ace == nil {
		return ErrPrivate
	}
	header := unsafe.Slice(ace, 8)
	if header[0] != 0 || header[1] != 0 || binary.LittleEndian.Uint16(header[2:4]) < 16 || binary.LittleEndian.Uint32(header[4:8]) != 0x001f01ff {
		return ErrPrivate
	}
	u, err := currentSID()
	if err != nil {
		return err
	}
	ownerOK, _, _ := advapi.NewProc("EqualSid").Call(uintptr(unsafe.Pointer(owner)), uintptr(unsafe.Pointer(u.User.Sid)))
	trusteeOK, _, _ := advapi.NewProc("EqualSid").Call(uintptr(unsafe.Add(unsafe.Pointer(ace), 8)), uintptr(unsafe.Pointer(u.User.Sid)))
	runtime.KeepAlive(u)
	if ownerOK == 0 || trusteeOK == 0 {
		return ErrPrivate
	}
	return nil
}

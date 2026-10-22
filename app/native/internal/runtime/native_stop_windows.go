package runtime

import (
	"nekoaurora.local/native/internal/ledger"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

func openNativeStop(path string) (*os.File, error) {
	name, e := syscall.UTF16PtrFromString(path)
	if e != nil {
		return nil, e
	}
	h, e := syscall.CreateFile(name, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL|0x00200000, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(h), path)
	invalid := func() (*os.File, error) { f.Close(); return nil, ledger.ErrInvalid }
	var data syscall.ByHandleFileInformation
	if syscall.GetFileInformationByHandle(h, &data) != nil || data.NumberOfLinks != 1 || data.FileAttributes&(0x400|syscall.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		return invalid()
	}
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() > ledger.NativeStopMaxBytes {
		return invalid()
	}
	buf := make([]uint16, 32768)
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetFinalPathNameByHandleW")
	n, _, _ := proc.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	if n == 0 || n >= uintptr(len(buf)) {
		return invalid()
	}
	actual := strings.TrimPrefix(syscall.UTF16ToString(buf[:n]), `\\?\`)
	if !strings.EqualFold(filepath.Clean(actual), filepath.Clean(path)) {
		return invalid()
	}
	return f, nil
}

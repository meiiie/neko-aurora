//go:build windows

package runtime

import (
	"io/fs"
	"path/filepath"
	"syscall"
	"unsafe"
)

var discoveryKernel = syscall.NewLazyDLL("kernel32.dll")
var discoveryDriveType = discoveryKernel.NewProc("GetDriveTypeW")

func discoveryReparse(info fs.FileInfo) bool {
	attributes, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return info.Mode()&fs.ModeSymlink != 0 || ok && attributes.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
func discoveryLocalVolume(path string) bool {
	root := filepath.VolumeName(path) + `\`
	raw, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		return false
	}
	kind, _, _ := discoveryDriveType.Call(uintptr(unsafe.Pointer(raw)))
	return kind == 2 || kind == 3 || kind == 6
}

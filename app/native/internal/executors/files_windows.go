package executors

import (
	"os"
	"syscall"
	"unsafe"
)

func lockWriter(path string) (*os.File, error) {
	name, e := syscall.UTF16PtrFromString(path)
	if e != nil {
		return nil, e
	}
	h, e := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL|0x00200000, 0)
	if e != nil {
		return nil, e
	}
	return os.NewFile(uintptr(h), path), nil
}
func singleFile(f *os.File) bool {
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() {
		return false
	}
	var data syscall.ByHandleFileInformation
	if e = syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &data); e != nil {
		return false
	}
	return data.NumberOfLinks == 1 && data.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT == 0
}
func replaceFile(from, to string) error {
	src, e := syscall.UTF16PtrFromString(from)
	if e != nil {
		return e
	}
	dst, e := syscall.UTF16PtrFromString(to)
	if e != nil {
		return e
	}
	move := syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")
	ok, _, e := move.Call(uintptr(unsafe.Pointer(src)), uintptr(unsafe.Pointer(dst)), 0x1|0x8)
	if ok == 0 {
		return e
	}
	return nil
}

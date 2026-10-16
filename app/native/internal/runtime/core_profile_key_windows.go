//go:build windows

package runtime

import (
	"runtime"
	"syscall"
	"unsafe"
)

type profileKeyBlob struct {
	Size uint32
	Data *byte
}

func profileKeyCrypt(data, entropy []byte, seal bool) (result []byte, err error) {
	if len(data) == 0 || len(data) > 8192 || len(entropy) == 0 || len(entropy) > 2048 {
		return nil, errProfileKey
	}
	libraryName, _ := syscall.UTF16PtrFromString("crypt32.dll")
	load := syscall.NewLazyDLL("kernel32.dll").NewProc("LoadLibraryExW")
	module, _, _ := load.Call(uintptr(unsafe.Pointer(libraryName)), 0, 0x00000800)
	if module == 0 {
		return nil, errProfileKey
	}
	defer syscall.FreeLibrary(syscall.Handle(module))
	name := "CryptUnprotectData"
	if seal {
		name = "CryptProtectData"
	}
	procedure, e := syscall.GetProcAddress(syscall.Handle(module), name)
	if e != nil {
		return nil, errProfileKey
	}
	inputCopy, entropyCopy := append([]byte(nil), data...), append([]byte(nil), entropy...)
	defer clear(inputCopy)
	defer clear(entropyCopy)
	input := profileKeyBlob{uint32(len(inputCopy)), &inputCopy[0]}
	extra := profileKeyBlob{uint32(len(entropyCopy)), &entropyCopy[0]}
	var output profileKeyBlob
	defer func() {
		if output.Data != nil {
			clear(unsafe.Slice(output.Data, int(output.Size)))
			freed, freeErr := syscall.LocalFree(syscall.Handle(uintptr(unsafe.Pointer(output.Data))))
			if freed != 0 || freeErr != nil {
				clear(result)
				result, err = nil, errProfileKey
			}
		}
	}()
	success, _, _ := syscall.SyscallN(procedure, uintptr(unsafe.Pointer(&input)), 0, uintptr(unsafe.Pointer(&extra)), 0, 0, 1, uintptr(unsafe.Pointer(&output)))
	runtime.KeepAlive(inputCopy)
	runtime.KeepAlive(entropyCopy)
	if success == 0 || output.Data == nil || output.Size > 8192 {
		return nil, errProfileKey
	}
	return append([]byte(nil), unsafe.Slice(output.Data, int(output.Size))...), nil
}

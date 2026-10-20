//go:build windows

package api

import (
	"runtime"
	"syscall"
	"unsafe"
)

var webCrypt32 = syscall.NewLazyDLL("crypt32.dll")

func sealWebSecret(data []byte, entropy string) ([]byte, error) {
	return webCrypt(data, entropy, true)
}

func unsealWebSecret(data []byte, entropy string) ([]byte, error) {
	return webCrypt(data, entropy, false)
}

func webCrypt(data []byte, entropy string, seal bool) (result []byte, err error) {
	procName := "CryptUnprotectData"
	if seal {
		procName = "CryptProtectData"
	}
	proc := webCrypt32.NewProc(procName)
	inputCopy := append([]byte(nil), data...)
	entropyCopy := append([]byte(nil), []byte(entropy)...)
	defer clear(inputCopy)
	defer clear(entropyCopy)
	if len(inputCopy) == 0 {
		inputCopy = []byte{0}
	}
	type blob struct {
		Size uint32
		Data *byte
	}
	input := blob{Size: uint32(len(data)), Data: &inputCopy[0]}
	optional := blob{Size: uint32(len(entropyCopy))}
	var entropyPointer *blob
	if len(entropyCopy) != 0 {
		optional.Data = &entropyCopy[0]
		entropyPointer = &optional
	}
	var output blob
	defer func() {
		if output.Data != nil {
			clear(unsafe.Slice(output.Data, int(output.Size)))
			syscall.LocalFree(syscall.Handle(uintptr(unsafe.Pointer(output.Data))))
			output.Data, output.Size = nil, 0
		}
	}()
	success, _, _ := syscall.SyscallN(proc.Addr(),
		uintptr(unsafe.Pointer(&input)), 0, uintptr(unsafe.Pointer(entropyPointer)), 0, 0, 1,
		uintptr(unsafe.Pointer(&output)))
	runtime.KeepAlive(inputCopy)
	runtime.KeepAlive(entropyCopy)
	runtime.KeepAlive(optional)
	if success == 0 || output.Data == nil || output.Size == 0 || output.Size > 1<<20 {
		return nil, errWebConnectionSeal
	}
	result = make([]byte, int(output.Size))
	copy(result, unsafe.Slice(output.Data, len(result)))
	return result, nil
}

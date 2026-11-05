//go:build windows

package privatefs

import "syscall"

func openDirectory(path string, access uint32) (syscall.Handle, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	h, err := syscall.CreateFile(name, access, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return 0, err
	}
	var info syscall.ByHandleFileInformation
	if syscall.GetFileInformationByHandle(h, &info) != nil || info.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		syscall.CloseHandle(h)
		return 0, ErrPrivate
	}
	return h, nil
}

// ProtectDirectory restricts an existing directory and inherited child ACLs to its owner.
func ProtectDirectory(path string) error {
	h, err := openDirectory(path, syscall.GENERIC_READ|0x00040000)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(h)
	return protectHandle(h, "OICI", 3, false)
}

func CheckDirectory(path string) error {
	h, err := openDirectory(path, syscall.GENERIC_READ)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(h)
	return checkHandleSecurity(h, 3, true)
}

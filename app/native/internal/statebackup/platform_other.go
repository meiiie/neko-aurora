//go:build !windows

package statebackup

import "os"

func protect(data, entropy []byte) ([]byte, error)   { return nil, ErrUnsupported }
func unprotect(data, entropy []byte) ([]byte, error) { return nil, ErrUnsupported }
func openPinned(path string) (*os.File, error)       { return nil, ErrUnsupported }
func processInactive(pid uint32) error               { return ErrUnsupported }
func directorySafe(path string) error                { return ErrUnsupported }
func makePrivateDirectory(path string) error         { return ErrUnsupported }

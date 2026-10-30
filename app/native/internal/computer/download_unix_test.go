//go:build !windows

package computer

import "os"

func openDownloadTestSource(path string) (*os.File, error) { return os.Open(path) }

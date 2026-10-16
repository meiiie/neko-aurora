//go:build !windows

package runtime

import "io/fs"

func discoveryReparse(info fs.FileInfo) bool { return info.Mode()&fs.ModeSymlink != 0 }
func discoveryLocalVolume(path string) bool  { return true }

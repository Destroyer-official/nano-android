//go:build !windows

package mcp

import (
	"fmt"
	"syscall"
)

// getDiskFreeSpace returns the free and total bytes for the volume containing path.
func getDiskFreeSpace(path string) (freeBytes uint64, totalBytes uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, fmt.Errorf("statfs error: %w", err)
	}
	return stat.Bavail * uint64(stat.Bsize), stat.Blocks * uint64(stat.Bsize), nil
}

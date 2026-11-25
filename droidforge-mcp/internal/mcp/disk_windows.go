//go:build windows

package mcp

import (
	"fmt"
	"syscall"
	"unsafe"
)

// getDiskFreeSpace returns the free and total bytes for the volume containing path.
func getDiskFreeSpace(path string) (freeBytes uint64, totalBytes uint64, err error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetDiskFreeSpaceExW")

	var free, total, totalFree int64
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, fmt.Errorf("utf16 conversion failed: %w", err)
	}

	r1, _, callErr := proc.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&free)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&totalFree)),
	)
	if r1 == 0 {
		return 0, 0, fmt.Errorf("GetDiskFreeSpaceExW error: %v", callErr)
	}

	return uint64(free), uint64(total), nil
}

//go:build !windows

package subtree

import (
	"os"
	"syscall"
)

// mmapFile creates a shared, read-write mmap mapping for the given file.
// The mapping is MAP_SHARED so that writes are flushed back to the file,
// allowing the OS to page cold data to disk and reclaim RAM.
func mmapFile(f *os.File, size int) ([]byte, error) {
	return syscall.Mmap(
		int(f.Fd()),
		0,
		size,
		syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED,
	)
}

// munmap releases an mmap mapping.
func munmap(data []byte) error {
	return syscall.Munmap(data)
}

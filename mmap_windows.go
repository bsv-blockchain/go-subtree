//go:build windows

package subtree

import (
	"os"
)

// mmapFile is not supported on Windows; returns ErrMmapUnsupported so callers fall back gracefully.
func mmapFile(_ *os.File, _ int) ([]byte, error) {
	return nil, ErrMmapUnsupported
}

// munmap is a no-op on Windows.
func munmap(_ []byte) error {
	return nil
}

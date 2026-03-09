//go:build windows

package subtree

import (
	"errors"
	"os"
)

// mmapFile is not supported on Windows; returns an error so callers fall back gracefully.
func mmapFile(_ *os.File, _ int) ([]byte, error) {
	return nil, errors.New("mmap is not supported on Windows")
}

// munmap is a no-op on Windows.
func munmap(_ []byte) error {
	return nil
}

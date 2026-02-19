package subtree

import (
	"fmt"
	"io"
	"os"
	"sync"
	"unsafe"
)

// nodeSize is the size of a Node struct in bytes.
// Node has no pointer fields ([32]byte + uint64 + uint64 = 48 bytes),
// which makes it safe to store in mmap'd memory outside the GC's reach.
const nodeSize = int(unsafe.Sizeof(Node{}))

// mmapNodeStore manages a file-backed mmap region that stores Node data.
// When closed, it unmaps the region and removes the backing file.
type mmapNodeStore struct {
	data     []byte // raw mmap region
	filePath string // backing file path (for cleanup)
	once     sync.Once
}

// Close unmaps the mmap region and removes the backing file.
// Safe to call multiple times.
func (m *mmapNodeStore) Close() error {
	var err error
	m.once.Do(func() {
		if m.data != nil {
			if munmapErr := munmap(m.data); munmapErr != nil {
				err = fmt.Errorf("munmap failed: %w", munmapErr)
			}
			m.data = nil
		}
		if m.filePath != "" {
			_ = os.Remove(m.filePath)
		}
	})
	return err
}

// newFileBackedMmapNodes creates a file-backed mmap region sized for the given
// capacity of Nodes. The file is created in dir using os.CreateTemp. The file
// descriptor is closed after mmap (the kernel keeps the mapping alive via the
// inode reference), so this holds zero persistent file descriptors.
//
// Returns a []Node slice backed by the mmap'd region and an io.Closer for cleanup.
// The returned slice has len=0, cap=capacity.
func newFileBackedMmapNodes(capacity int, dir string) ([]Node, io.Closer, error) {
	if capacity <= 0 {
		return nil, nil, fmt.Errorf("%w: got %d", ErrCapacityNotPositive, capacity)
	}

	size := capacity * nodeSize

	// Create temp file
	f, err := os.CreateTemp(dir, "subtree-nodes-*")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	filePath := f.Name()

	// Truncate to required size
	if err = f.Truncate(int64(size)); err != nil {
		_ = f.Close()
		_ = os.Remove(filePath)
		return nil, nil, fmt.Errorf("failed to truncate file to %d bytes: %w", size, err)
	}

	// mmap the file with MAP_SHARED so writes go back to the file for OS paging
	data, err := mmapFile(f, size)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(filePath)
		return nil, nil, fmt.Errorf("mmap failed: %w", err)
	}

	// Close the fd — the kernel keeps the mapping alive via the inode.
	// This saves file descriptors (important at 1000+ subtrees).
	_ = f.Close()

	// Create a []Node view backed by the mmap'd memory.
	// This is safe because Node has no pointer fields, so the GC won't scan this region.
	nodes := unsafe.Slice((*Node)(unsafe.Pointer(&data[0])), capacity)[:0:capacity]

	store := &mmapNodeStore{
		data:     data,
		filePath: filePath,
	}

	return nodes, store, nil
}

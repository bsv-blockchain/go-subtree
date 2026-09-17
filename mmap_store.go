package subtree

import (
	"fmt"
	"io"
	"math"
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
//
// capacity may originate from an untrusted persisted count, so it is validated
// before it drives any arithmetic: capacity <= 0 is rejected, and a capacity that
// would overflow either the mapped byte size (capacity * nodeSize) or the
// unsafe.Slice length is rejected with ErrCapacityTooLarge. Without that bound a
// hostile count wraps `size` to a small value, the tiny region maps successfully,
// and the subsequent unsafe.Slice panics with "unsafe.Slice: len out of range" —
// crashing the process. A deferred recover converts any residual unsafe panic
// into ErrMmapPanic as a belt-and-braces measure.
func newFileBackedMmapNodes(capacity int, dir string) (nodes []Node, closer io.Closer, err error) {
	// store is declared here so the deferred recover can release an already-mapped
	// region if the unsafe.Slice below ever panics despite the bound. The bound
	// makes that unreachable in practice, but unsafe is involved, so we neither
	// let a panic escape nor leak the mapping and backing file behind it.
	var store *mmapNodeStore

	defer func() {
		if r := recover(); r != nil {
			if store != nil {
				_ = store.Close()
			}
			nodes, closer, err = nil, nil, fmt.Errorf("%w: %v", ErrMmapPanic, r)
		}
	}()

	if capacity <= 0 {
		return nil, nil, fmt.Errorf("%w: got %d", ErrCapacityNotPositive, capacity)
	}

	// capacity * nodeSize must not overflow int, and the []Node view built with
	// unsafe.Slice (length = capacity) must not overflow uintptr in len*elemsize.
	// math.MaxInt/nodeSize is the largest capacity that satisfies both.
	if capacity > math.MaxInt/nodeSize {
		return nil, nil, fmt.Errorf("%w: %d exceeds max %d", ErrCapacityTooLarge, capacity, math.MaxInt/nodeSize)
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

	// Build the cleanup handle before the unsafe.Slice so the deferred recover can
	// unmap and remove the backing file if that call ever faults.
	store = &mmapNodeStore{
		data:     data,
		filePath: filePath,
	}

	// Create a []Node view backed by the mmap'd memory.
	// This is safe because Node has no pointer fields, so the GC won't scan this region.
	nodes = unsafe.Slice((*Node)(unsafe.Pointer(&data[0])), capacity)[:0:capacity] //nolint:gosec // G103: intentional unsafe for mmap-backed Node slice

	return nodes, store, nil
}

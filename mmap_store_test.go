package subtree

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// numLeavesOffset is the byte offset of the 8-byte little-endian node count in a
// serialized subtree header: rootHash(32) | fees(8) | sizeInBytes(8) | numNodes(8).
const numLeavesOffset = rootHashPlusFeesPlusSizeLen

// auditHostileCount is the exact node count from the keys-to-the-kingdom audit
// (S-1). count * nodeSize wraps (mod 2^64) to 16 bytes, so the scratch file maps
// successfully and the subsequent unsafe.Slice(ptr, count) overflows uintptr and
// panics with "unsafe.Slice: len out of range" unless the count is bounded first.
const auditHostileCount uint64 = 768614336404564651

// mmapTempFiles returns the scratch files newFileBackedMmapNodes leaves in dir.
// A rejected or failed load must leave none behind.
func mmapTempFiles(t *testing.T, dir string) []string {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(dir, "subtree-nodes-*"))
	require.NoError(t, err)

	return files
}

// TestNewFileBackedMmapNodes_RejectsHostileCapacity pins that a capacity which
// cannot be mapped is rejected as an ordinary error — never as a panic, and never
// via the belt-and-braces recover (asserting the specific sentinel proves the
// bound fired before unsafe.Slice, not that a panic was caught after it). Before
// the bound, capacity*nodeSize overflowed int and unsafe.Slice panicked on the
// oversized length: the S-1 process-crash primitive.
func TestNewFileBackedMmapNodes_RejectsHostileCapacity(t *testing.T) {
	tests := []struct {
		name    string
		cap     int
		wantErr error
	}{
		{name: "zero", cap: 0, wantErr: ErrCapacityNotPositive},
		{name: "negative", cap: -1, wantErr: ErrCapacityNotPositive},
		{name: "max int", cap: math.MaxInt, wantErr: ErrCapacityTooLarge},
		{name: "one past the bound", cap: math.MaxInt/nodeSize + 1, wantErr: ErrCapacityTooLarge},
		{name: "audit count", cap: int(auditHostileCount), wantErr: ErrCapacityTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			var (
				nodes  []Node
				closer io.Closer
				err    error
			)

			require.NotPanics(t, func() {
				nodes, closer, err = newFileBackedMmapNodes(tt.cap, dir)
			})

			require.ErrorIs(t, err, tt.wantErr)
			require.Nil(t, nodes)
			require.Nil(t, closer)
			require.Empty(t, mmapTempFiles(t, dir), "a rejected capacity must not leave a scratch file")
		})
	}
}

// TestNewFileBackedMmapNodes_ValidCapacity pins the happy path is unchanged: a
// small capacity yields a zero-length, capacity-capped slice over a scratch file
// that Close removes idempotently.
func TestNewFileBackedMmapNodes_ValidCapacity(t *testing.T) {
	dir := t.TempDir()

	nodes, closer, err := newFileBackedMmapNodes(4, dir)
	require.NoError(t, err)
	require.NotNil(t, closer)
	require.Empty(t, nodes)
	require.Equal(t, 4, cap(nodes))
	require.Len(t, mmapTempFiles(t, dir), 1)

	require.NoError(t, closer.Close())
	require.Empty(t, mmapTempFiles(t, dir), "Close must remove the scratch file")
	require.NoError(t, closer.Close(), "Close must be idempotent")
}

// TestNewSubtreeFromReaderMmap_HostileCount pins that a hostile leaf count in the
// serialized header is rejected up front by the range bound (ErrNumLeavesOutOfRange)
// with no panic and no leftover scratch file. This is the S-1 delivery shape: the
// count arrives in bytes and, unbounded, crashed the process via unsafe.Slice.
func TestNewSubtreeFromReaderMmap_HostileCount(t *testing.T) {
	valid, _ := buildSubtreeWithConflicting(t, 8, 0)

	tests := []struct {
		name  string
		count uint64
	}{
		{name: "audit count wraps size to 16 bytes", count: auditHostileCount},
		{name: "max uint64", count: math.MaxUint64},
		{name: "one past the int bound", count: uint64(math.MaxInt)/uint64(nodeSize) + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			corrupted := make([]byte, len(valid))
			copy(corrupted, valid)
			binary.LittleEndian.PutUint64(corrupted[numLeavesOffset:], tt.count)

			var (
				st  *Subtree
				err error
			)

			require.NotPanics(t, func() {
				st, err = NewSubtreeFromReaderMmap(bytes.NewReader(corrupted), dir)
			})

			require.ErrorIs(t, err, ErrNumLeavesOutOfRange)
			require.Nil(t, st)
			require.Empty(t, mmapTempFiles(t, dir), "a rejected load must not leave a scratch file")
		})
	}
}

// TestNewSubtreeFromReaderMmap_AuditHeaderOnly reproduces the audit input exactly:
// a 56-byte header (rootHash | fees | sizeInBytes | numNodes) whose count is the
// value that made the real v1.4.6 loader panic, with no node bodies behind it.
// The loader must return an error, not crash the process.
func TestNewSubtreeFromReaderMmap_AuditHeaderOnly(t *testing.T) {
	dir := t.TempDir()

	header := make([]byte, rootHashPlusFeesPlusSizeLen+8) // 56-byte header, no bodies
	binary.LittleEndian.PutUint64(header[numLeavesOffset:], auditHostileCount)

	var (
		st  *Subtree
		err error
	)

	require.NotPanics(t, func() {
		st, err = NewSubtreeFromReaderMmap(bytes.NewReader(header), dir)
	})

	require.ErrorIs(t, err, ErrNumLeavesOutOfRange)
	require.Nil(t, st)
	require.Empty(t, mmapTempFiles(t, dir))
}

// TestNewSubtreeFromReaderMmap_ShortBody pins that a declared count larger than the
// bytes present — a file shorter than its declared layout — fails cleanly and
// removes its scratch file, rather than reading past the mapped region. The count
// is small enough to map, so this exercises the read loop's EOF path, not the bound.
func TestNewSubtreeFromReaderMmap_ShortBody(t *testing.T) {
	dir := t.TempDir()

	valid, _ := buildSubtreeWithConflicting(t, 8, 0)

	// Declare 1000 nodes but keep only the original 8-node body behind it.
	corrupted := make([]byte, len(valid))
	copy(corrupted, valid)
	binary.LittleEndian.PutUint64(corrupted[numLeavesOffset:], 1000)

	var (
		st  *Subtree
		err error
	)

	require.NotPanics(t, func() {
		st, err = NewSubtreeFromReaderMmap(bytes.NewReader(corrupted), dir)
	})

	require.Error(t, err)
	require.Nil(t, st)
	require.Empty(t, mmapTempFiles(t, dir), "a short body must not leave a scratch file")
}

// TestNewSubtreeFromReaderMmap_ValidRoundTrip pins that valid subtrees — including
// one carrying a conflicting-node trailer — still load and re-serialize byte for
// byte through the mmap path after the hardening (zero regression).
func TestNewSubtreeFromReaderMmap_ValidRoundTrip(t *testing.T) {
	tests := []struct {
		name           string
		numNodes       int
		numConflicting int
	}{
		{name: "no conflicting nodes", numNodes: 8, numConflicting: 0},
		{name: "with conflicting trailer", numNodes: 16, numConflicting: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			serialized, _ := buildSubtreeWithConflicting(t, tt.numNodes, tt.numConflicting)

			st, err := NewSubtreeFromReaderMmap(bytes.NewReader(serialized), dir)
			require.NoError(t, err)
			defer func() { require.NoError(t, st.Close()) }()

			require.True(t, st.IsMmapBacked())
			require.Equal(t, tt.numNodes, st.Length())

			reSerialized, err := st.Serialize()
			require.NoError(t, err)
			require.Equal(t, serialized, reSerialized)
		})
	}
}

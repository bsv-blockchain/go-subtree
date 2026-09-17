package subtree

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"path/filepath"
	"runtime"
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

// skipIfMmapUnsupported skips tests that require a working mmap backend. Windows
// implements mmapFile as an unsupported operation (mmap_windows.go), so the
// success paths cannot run there; the rejection paths, which fail before mapping,
// still run everywhere.
func skipIfMmapUnsupported(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("mmap is not supported on Windows")
	}
}

// unrecoverableSeeker satisfies io.Seeker but, after answering the initial
// SeekCurrent probe, fails every subsequent seek — modelling a reader whose
// position cannot be restored once a size probe has disturbed it. The loader must
// treat this as a hard error rather than parsing from an unknown offset.
type unrecoverableSeeker struct{ r io.Reader }

func (s unrecoverableSeeker) Read(p []byte) (int, error) { return s.r.Read(p) }

func (s unrecoverableSeeker) Seek(_ int64, whence int) (int64, error) {
	if whence == io.SeekCurrent {
		return 0, nil // the initial position query succeeds
	}

	return 0, io.ErrUnexpectedEOF // SeekEnd, and the restore that follows, both fail
}

// zeroEndSeeker is an io.ReadSeeker whose SeekEnd reports length 0 without moving
// the underlying reader — the shape of a streaming / object-store wrapper whose
// length is not known until the body is read. maxNodesInReader must treat this as
// "size unknown" and fall back to the practical ceiling, not as an authoritative
// zero-node bound that would reject every well-formed subtree.
type zeroEndSeeker struct{ r *bytes.Reader }

func (s zeroEndSeeker) Read(p []byte) (int, error) { return s.r.Read(p) }

func (s zeroEndSeeker) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekEnd {
		return 0, nil // report empty without disturbing the read position
	}

	return s.r.Seek(offset, whence)
}

// TestNewFileBackedMmapNodes_RejectsHostileCapacity pins that a capacity which
// cannot be mapped is rejected as an ordinary error — never as a panic, and never
// via the belt-and-braces recover (asserting the specific sentinel proves the
// bound fired before unsafe.Slice, not that a panic was caught after it). Before
// the bound, capacity*nodeSize overflowed int and unsafe.Slice panicked on the
// oversized length: the S-1 process-crash primitive.
//
// The int boundary values below are representable on every architecture, so this
// test compiles on 32-bit too. The audit value 768614336404564651 exceeds MaxInt
// on 32-bit and so cannot be a compile-time int constant here; it is exercised as
// a uint64 through the reader path in TestNewSubtreeFromReaderMmap_HostileCount.
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
	skipIfMmapUnsupported(t)

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

// TestNewSubtreeFromReaderMmap_CountExceedsInput pins the input-size bound: a
// count that clears the overflow bound but whose node array cannot fit in the
// reader's remaining bytes (e.g. 1<<40 behind a tiny body) is rejected before any
// scratch file is mapped. Without it, that count would drive a ~48 TiB
// truncate+mmap from a few hundred bytes. bytes.Reader is seekable, so the size
// is knowable.
func TestNewSubtreeFromReaderMmap_CountExceedsInput(t *testing.T) {
	dir := t.TempDir()

	valid, _ := buildSubtreeWithConflicting(t, 8, 0)

	corrupted := make([]byte, len(valid))
	copy(corrupted, valid)
	binary.LittleEndian.PutUint64(corrupted[numLeavesOffset:], 1<<40)

	var (
		st  *Subtree
		err error
	)

	require.NotPanics(t, func() {
		st, err = NewSubtreeFromReaderMmap(bytes.NewReader(corrupted), dir)
	})

	require.ErrorIs(t, err, ErrNodeCountExceedsInput)
	require.Nil(t, st)
	require.Empty(t, mmapTempFiles(t, dir), "an impossible count must not map a scratch file")
}

// TestNewSubtreeFromReaderMmap_ShortBodyNonSeekable pins the fallback for readers
// whose size is not knowable: the input-size bound is skipped, so a declared count
// larger than the body present is caught by the per-node read loop hitting EOF.
// The mapping is created first, so this also asserts the scratch file is cleaned
// up on the read failure. The count stays small enough to map cheaply.
func TestNewSubtreeFromReaderMmap_ShortBodyNonSeekable(t *testing.T) {
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

	// nonSeekableReader hides the Seeker bytes.Reader provides, so maxNodesInReader
	// reports "unknown" and the read loop is what fails.
	require.NotPanics(t, func() {
		st, err = NewSubtreeFromReaderMmap(nonSeekableReader{r: bytes.NewReader(corrupted)}, dir)
	})

	require.Error(t, err)
	require.Nil(t, st)
	require.Empty(t, mmapTempFiles(t, dir), "a short body must not leave a scratch file")
}

// TestNewSubtreeFromReaderMmap_NonSeekableCountExceedsLimit pins the practical
// ceiling for readers whose length cannot be probed. A non-seekable stream can
// declare a count (here maxMmapDeserializeNodes+1) that clears the overflow bound
// yet would map a multi-terabyte scratch file; it must be rejected before mapping.
// Only a header is supplied — the count is refused before any node is read.
func TestNewSubtreeFromReaderMmap_NonSeekableCountExceedsLimit(t *testing.T) {
	dir := t.TempDir()

	header := make([]byte, rootHashPlusFeesPlusSizeLen+8)
	binary.LittleEndian.PutUint64(header[numLeavesOffset:], uint64(maxMmapDeserializeNodes)+1)

	var (
		st  *Subtree
		err error
	)

	require.NotPanics(t, func() {
		st, err = NewSubtreeFromReaderMmap(nonSeekableReader{r: bytes.NewReader(header)}, dir)
	})

	require.ErrorIs(t, err, ErrNodeCountExceedsLimit)
	require.Nil(t, st)
	require.Empty(t, mmapTempFiles(t, dir), "a count over the non-seekable limit must not map a scratch file")
}

// TestNewSubtreeFromReaderMmap_SeekProbeUnrecoverable pins that a size probe which
// leaves the reader's position unrecoverable is a hard error, not a silent
// fallback. io.Seeker does not promise the position is unchanged after a failed
// Seek, so parsing from an unknown offset (and misreading trailing bytes as a
// header) must be refused outright.
func TestNewSubtreeFromReaderMmap_SeekProbeUnrecoverable(t *testing.T) {
	dir := t.TempDir()

	valid, _ := buildSubtreeWithConflicting(t, 8, 0)

	var (
		st  *Subtree
		err error
	)

	require.NotPanics(t, func() {
		st, err = NewSubtreeFromReaderMmap(unrecoverableSeeker{r: bytes.NewReader(valid)}, dir)
	})

	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Nil(t, st)
	require.Empty(t, mmapTempFiles(t, dir), "a failed probe must not map a scratch file")
}

// TestNewSubtreeFromReaderMmap_ValidRoundTrip pins that valid subtrees — including
// one carrying a conflicting-node trailer — still load and re-serialize byte for
// byte through the mmap path after the hardening (zero regression).
func TestNewSubtreeFromReaderMmap_ValidRoundTrip(t *testing.T) {
	skipIfMmapUnsupported(t)

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

// TestNewSubtreeFromReaderMmap_SeekableReportsZeroLength pins that a seekable
// wrapper whose SeekEnd reports 0 (streaming / object-store shapes whose length is
// not known until the body is read) does not have its well-formed subtree rejected.
// Treating that probe as an authoritative zero-node bound would fail every load with
// ErrNodeCountExceedsInput; instead the loader must fall back to the practical
// ceiling and parse normally, still re-serializing byte for byte.
func TestNewSubtreeFromReaderMmap_SeekableReportsZeroLength(t *testing.T) {
	skipIfMmapUnsupported(t)

	dir := t.TempDir()

	valid, _ := buildSubtreeWithConflicting(t, 8, 2)

	var (
		st  *Subtree
		err error
	)

	require.NotPanics(t, func() {
		st, err = NewSubtreeFromReaderMmap(zeroEndSeeker{r: bytes.NewReader(valid)}, dir)
	})

	require.NoError(t, err, "a wrapper reporting zero length must not trip the input bound")
	require.NotNil(t, st)

	defer func() { require.NoError(t, st.Close()) }()

	require.Equal(t, 8, st.Length())

	reSerialized, err := st.Serialize()
	require.NoError(t, err)
	require.Equal(t, valid, reSerialized)
}

// TestNewSubtreeFromReaderMmap_HostileConflictingCount pins that a hostile
// conflicting-node trailer count is rejected as an ordinary error, never as a panic
// or an OOM. The node array is well-formed, so the loader maps and reads it, then
// reaches the trailer: a count above the leaf total must fail with
// ErrConflictingCountExceedsLeaves before any allocation is sized by it. Before the
// bound, 1<<62 panicked in makeslice (surfacing only as the recover's ErrMmapPanic)
// and an allocation-sized count could OOM-kill the worker.
func TestNewSubtreeFromReaderMmap_HostileConflictingCount(t *testing.T) {
	skipIfMmapUnsupported(t)

	const numNodes = 16

	valid, _ := buildSubtreeWithConflicting(t, numNodes, 2)

	tests := []struct {
		name  string
		count uint64
	}{
		{name: "one over the leaf count", count: numNodes + 1},
		{name: "makeslice-panic sized", count: 1 << 62},
		{name: "allocation sized", count: 1 << 31},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			corrupted := make([]byte, len(valid))
			copy(corrupted, valid)
			binary.LittleEndian.PutUint64(corrupted[conflictingCountOffset(numNodes):], tt.count)

			var (
				st  *Subtree
				err error
			)

			require.NotPanics(t, func() {
				st, err = NewSubtreeFromReaderMmap(bytes.NewReader(corrupted), dir)
			})

			require.ErrorIs(t, err, ErrConflictingCountExceedsLeaves)
			require.Nil(t, st)
			require.Empty(t, mmapTempFiles(t, dir), "a rejected trailer must not leave a scratch file")
		})
	}
}

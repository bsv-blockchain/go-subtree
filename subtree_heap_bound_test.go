package subtree

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"runtime"
	"testing"

	"github.com/bsv-blockchain/go-bt/v2/chainhash"
	"github.com/stretchr/testify/require"
)

// hostileHeader returns a 56-byte serialized subtree header that declares count
// nodes and carries none of them: rootHash(32) | fees(8) | sizeInBytes(8) | count(8).
func hostileHeader(count uint64) []byte {
	header := make([]byte, rootHashPlusFeesPlusSizeLen+8)
	binary.LittleEndian.PutUint64(header[numLeavesOffset:], count)

	return header
}

// TestNewSubtreeFromBytes_CountExceedsInput pins the input-length bound on the heap
// path. A byte slice's length is known, so a header declaring more nodes than the
// slice holds is rejected before the node array is allocated, the same way the
// mmap path rejects it for a seekable reader.
func TestNewSubtreeFromBytes_CountExceedsInput(t *testing.T) {
	var (
		st  *Subtree
		err error
	)

	require.NotPanics(t, func() {
		st, err = NewSubtreeFromBytes(hostileHeader(2))
	})

	require.ErrorIs(t, err, ErrNodeCountExceedsInput)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Nil(t, st)
}

// TestNewSubtreeFromReader_HostileCounts pins that a count which cannot describe
// a real subtree is rejected with a sentinel before any allocation, for both
// reader kinds. Before the bound, these counts reached make([]Node, count): the
// overflowing ones panicked (recovered without a sentinel), and the ones in
// between asked the runtime for terabytes, which is a fatal, unrecoverable throw.
func TestNewSubtreeFromReader_HostileCounts(t *testing.T) {
	// A seekable 56-byte reader cannot hold any node, so for counts that clear the
	// overflow bound its length check fires before the ceiling does.
	tests := []struct {
		name         string
		count        uint64
		wantStream   error
		wantSeekable error
	}{
		{name: "audit count", count: auditHostileCount, wantStream: ErrNumLeavesOutOfRange, wantSeekable: ErrNumLeavesOutOfRange},
		{name: "max uint64", count: math.MaxUint64, wantStream: ErrNumLeavesOutOfRange, wantSeekable: ErrNumLeavesOutOfRange},
		{name: "terabytes", count: 1 << 36, wantStream: ErrNodeCountExceedsLimit, wantSeekable: ErrNodeCountExceedsInput},
		{name: "one past the ceiling", count: maxDeserializeNodes + 1, wantStream: ErrNodeCountExceedsLimit, wantSeekable: ErrNodeCountExceedsInput},
	}

	for _, tt := range tests {
		readers := []struct {
			name    string
			reader  io.Reader
			wantErr error
		}{
			{name: "non-seekable", reader: nonSeekableReader{r: bytes.NewReader(hostileHeader(tt.count))}, wantErr: tt.wantStream},
			{name: "seekable", reader: bytes.NewReader(hostileHeader(tt.count)), wantErr: tt.wantSeekable},
		}

		for _, rd := range readers {
			t.Run(tt.name+"/"+rd.name, func(t *testing.T) {
				var (
					st  *Subtree
					err error
				)

				require.NotPanics(t, func() {
					st, err = NewSubtreeFromReader(rd.reader)
				})

				require.ErrorIs(t, err, rd.wantErr)
				require.Nil(t, st)
			})
		}
	}
}

// TestDeserializeFromReaderWithAllocator_HostileCountSkipsAllocator pins that a
// caller-supplied allocator is never asked to back a count that was rejected. The
// allocator contract is cap >= numLeaves, so calling it with a hostile count would
// hand the allocation straight to the caller's pool.
func TestDeserializeFromReaderWithAllocator_HostileCountSkipsAllocator(t *testing.T) {
	allocCalls := 0
	alloc := func(_ int) []Node {
		allocCalls++
		return make([]Node, 0, 1)
	}

	st := &Subtree{}

	var err error

	require.NotPanics(t, func() {
		err = st.DeserializeFromReaderWithAllocator(nonSeekableReader{r: bytes.NewReader(hostileHeader(auditHostileCount))}, alloc)
	})

	require.ErrorIs(t, err, ErrNumLeavesOutOfRange)
	require.Zero(t, allocCalls, "a rejected count must not reach the allocator")
}

// TestDeserializeFromReader_UnverifiedCountBoundsPrealloc pins the up-front
// allocation for a count that passes every bound but whose length cannot be
// checked (a non-seekable stream). Only maxNodePrealloc nodes are allocated before
// the stream proves it holds more, so a header declaring 64x the cap costs at most
// the cap, not a multi-GiB anonymous allocation.
func TestDeserializeFromReader_UnverifiedCountBoundsPrealloc(t *testing.T) {
	const count = uint64(maxNodePrealloc) * 64

	// Two real nodes behind the header, then EOF.
	input := append(hostileHeader(count), make([]byte, 2*nodeSerializedLen)...)

	var before, after runtime.MemStats

	runtime.ReadMemStats(&before)

	st, err := NewSubtreeFromReader(nonSeekableReader{r: bytes.NewReader(input)})

	runtime.ReadMemStats(&after)

	// The stream ends exactly after the second node, so the third read sees EOF.
	require.ErrorIs(t, err, io.EOF)
	require.Nil(t, st)

	const slack = 1 << 20

	require.LessOrEqual(t, after.TotalAlloc-before.TotalAlloc, uint64(maxNodePrealloc*nodeSize+slack),
		"an unverified count must not allocate more than maxNodePrealloc nodes up front")
}

// TestDeserializeFromReader_GrowPathRoundTrip pins that a genuine subtree with more
// leaves than maxNodePrealloc still decodes exactly through a non-seekable reader,
// where the node array starts at the cap and grows as nodes are read. A pooled
// allocator is not consulted for such a count.
func TestDeserializeFromReader_GrowPathRoundTrip(t *testing.T) {
	const numNodes = maxNodePrealloc + 3

	st, err := NewTreeByLeafCount(maxNodePrealloc * 2)
	require.NoError(t, err)

	for i := 0; i < numNodes; i++ {
		var h chainhash.Hash

		binary.LittleEndian.PutUint64(h[:], uint64(i)+1)
		require.NoError(t, st.AddNode(h, uint64(i), uint64(i)*2))
	}

	conflicting := st.Nodes[numNodes-1].Hash
	require.NoError(t, st.AddConflictingNode(conflicting))

	serialized, err := st.Serialize()
	require.NoError(t, err)

	allocCalls := 0
	alloc := func(n int) []Node {
		allocCalls++
		return make([]Node, n)
	}

	got := &Subtree{}
	require.NoError(t, got.DeserializeFromReaderWithAllocator(nonSeekableReader{r: bytes.NewReader(serialized)}, alloc))

	require.Zero(t, allocCalls, "a count above maxNodePrealloc must not reach the allocator")
	require.Len(t, got.Nodes, numNodes)
	require.Equal(t, st.Nodes, got.Nodes)
	require.Equal(t, []chainhash.Hash{conflicting}, got.ConflictingNodes)
	require.Equal(t, st.RootHash(), got.RootHash())
}

package subtree

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"testing"

	"github.com/bsv-blockchain/go-bt/v2/chainhash"
	"github.com/stretchr/testify/require"
)

// nonSeekableReader hides the io.Seeker that bytes.Reader provides, so the
// streaming path can be exercised against the same bytes.
type nonSeekableReader struct{ r io.Reader }

func (n nonSeekableReader) Read(p []byte) (int, error) { return n.r.Read(p) }

// refusingSeeker satisfies io.Seeker but fails at runtime, mimicking a wrapper
// whose underlying reader is a network stream. The trailer reader must detect
// this on the initial probe and fall back without having consumed anything.
type refusingSeeker struct{ r io.Reader }

func (f refusingSeeker) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f refusingSeeker) Seek(int64, int) (int64, error) {
	return 0, io.ErrUnexpectedEOF
}

func buildSubtreeWithConflicting(t *testing.T, numNodes, numConflicting int) ([]byte, []chainhash.Hash) {
	t.Helper()

	st, err := NewIncompleteTreeByLeafCount(numNodes)
	require.NoError(t, err)

	require.NoError(t, st.AddCoinbaseNode())

	for i := 1; i < numNodes; i++ {
		var h chainhash.Hash
		h[0] = byte(i)
		h[1] = byte(i >> 8)
		require.NoError(t, st.AddNode(h, uint64(i), uint64(i*2)))
	}

	// AddConflictingNode requires the hash to already be a node of this subtree,
	// so the conflicting set is drawn from the nodes just added (skipping the
	// coinbase placeholder at index 0).
	require.Less(t, numConflicting, numNodes, "need one node per conflicting entry, excluding the coinbase")

	expected := make([]chainhash.Hash, 0, numConflicting)

	for i := 1; i <= numConflicting; i++ {
		h := st.Nodes[i].Hash
		require.NoError(t, st.AddConflictingNode(h))
		expected = append(expected, h)
	}

	b, err := st.Serialize()
	require.NoError(t, err)

	return b, expected
}

// TestDeserializeSubtreeConflictingFromReader_SeekAndStreamAgree is the property
// that matters: the seeking fast path and the streaming fallback must return
// identical results for identical bytes. If they ever diverge, a node's view of
// which transactions are conflicting would depend on its blob backend.
func TestDeserializeSubtreeConflictingFromReader_SeekAndStreamAgree(t *testing.T) {
	tests := []struct {
		name           string
		numNodes       int
		numConflicting int
	}{
		{name: "no conflicting nodes", numNodes: 8, numConflicting: 0},
		{name: "single conflicting node", numNodes: 8, numConflicting: 1},
		{name: "several conflicting nodes", numNodes: 16, numConflicting: 5},
		{name: "more conflicting than prealloc", numNodes: conflictingNodesPrealloc + 8, numConflicting: conflictingNodesPrealloc + 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serialized, expected := buildSubtreeWithConflicting(t, tt.numNodes, tt.numConflicting)

			// bytes.Reader implements io.Seeker, so this takes the fast path.
			seeked, err := DeserializeSubtreeConflictingFromReader(bytes.NewReader(serialized))
			require.NoError(t, err)

			streamed, err := DeserializeSubtreeConflictingFromReader(nonSeekableReader{r: bytes.NewReader(serialized)})
			require.NoError(t, err)

			require.Equal(t, expected, seeked, "seeking path returned the wrong conflicting nodes")
			require.Len(t, streamed, len(expected))
			require.Equal(t, seeked, streamed, "seeking and streaming paths must agree")
		})
	}
}

// TestDeserializeSubtreeConflictingFromReader_SeekerThatRefuses pins the probe:
// a reader that claims io.Seeker but fails must fall back cleanly rather than
// consuming bytes and then discovering it cannot seek.
func TestDeserializeSubtreeConflictingFromReader_SeekerThatRefuses(t *testing.T) {
	serialized, expected := buildSubtreeWithConflicting(t, 8, 3)

	got, err := DeserializeSubtreeConflictingFromReader(refusingSeeker{r: bytes.NewReader(serialized)})
	require.NoError(t, err)
	require.Equal(t, expected, got)
}

// TestDeserializeSubtreeConflictingFromReader_SeekStartsFromCurrentOffset pins
// the relative-seek requirement. Blob stores hand back a reader already advanced
// past their own file header, so absolute offsets would read the wrong bytes.
func TestDeserializeSubtreeConflictingFromReader_SeekStartsFromCurrentOffset(t *testing.T) {
	serialized, expected := buildSubtreeWithConflicting(t, 8, 2)

	const headerLen = 16

	withHeader := append(bytes.Repeat([]byte{0xAB}, headerLen), serialized...)

	reader := bytes.NewReader(withHeader)
	_, err := reader.Seek(headerLen, io.SeekStart)
	require.NoError(t, err)

	got, err := DeserializeSubtreeConflictingFromReader(reader)
	require.NoError(t, err)
	require.Equal(t, expected, got)
}

// TestDeserializeSubtreeConflictingFromReader_TruncatedTrailer ensures an
// overstated conflicting count surfaces as an error rather than silently
// returning a short list, since seeking past EOF succeeds on a file.
func TestDeserializeSubtreeConflictingFromReader_TruncatedTrailer(t *testing.T) {
	serialized, _ := buildSubtreeWithConflicting(t, 8, 4)

	truncated := serialized[:len(serialized)-chainhash.HashSize-1]

	_, err := DeserializeSubtreeConflictingFromReader(bytes.NewReader(truncated))
	require.Error(t, err)
}

// TestDeserializeSubtreeConflictingFromReader_HostileCountsAgree pins the
// equivalence property on inputs an attacker controls, which the benign-input
// test above does not reach.
//
// Subtree bytes arrive from peers, and which of the two paths runs depends on
// whether the blob backend hands back a seekable reader. If the paths disagreed
// on a malformed file, a node's view of which transactions are conflicting would
// depend on its storage backend — the exact divergence this parser must not
// introduce.
func TestDeserializeSubtreeConflictingFromReader_HostileCountsAgree(t *testing.T) {
	serialized, _ := buildSubtreeWithConflicting(t, 8, 2)

	// Offsets within the header: rootHash(32) | fees(8) | sizeInBytes(8) | numNodes(8)
	const numNodesOffset = rootHashPlusFeesPlusSizeLen

	tests := []struct {
		name   string
		mutate func([]byte)
	}{
		{
			name: "node count overflows the byte-length computation",
			mutate: func(b []byte) {
				binary.LittleEndian.PutUint64(b[numNodesOffset:], math.MaxUint64)
			},
		},
		{
			name: "node count is large but below the overflow bound",
			mutate: func(b []byte) {
				binary.LittleEndian.PutUint64(b[numNodesOffset:], 1<<40)
			},
		},
		{
			name: "conflicting count demands a huge allocation",
			mutate: func(b []byte) {
				// trailer count sits after the node array
				off := numNodesOffset + 8 + 8*nodeSerializedLen
				binary.LittleEndian.PutUint64(b[off:], 1<<35)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			corrupted := make([]byte, len(serialized))
			copy(corrupted, serialized)
			tt.mutate(corrupted)

			seeked, seekErr := DeserializeSubtreeConflictingFromReader(bytes.NewReader(corrupted))
			streamed, streamErr := DeserializeSubtreeConflictingFromReader(nonSeekableReader{r: bytes.NewReader(corrupted)})

			// Neither path may return a plausible-looking result for bytes the
			// other rejects, and neither may be driven into a huge allocation.
			require.Error(t, seekErr, "seeking path must reject malformed input")
			require.Error(t, streamErr, "streaming path must reject malformed input")
			require.Nil(t, seeked)
			require.Nil(t, streamed)
		})
	}
}

// conflictingCountOffset returns the byte offset of the conflicting-count word
// for a subtree serialized with numNodes leaves: it sits immediately after the
// header, the leaf-count word, and the node array.
func conflictingCountOffset(numNodes int) int {
	return rootHashPlusFeesPlusSizeLen + 8 + numNodes*nodeSerializedLen
}

// TestDeserializeSubtreeConflictingFromReader_ConflictingExceedsLeaves pins the
// invariant that a trailer may not claim more conflicting nodes than the subtree
// has leaves. AddConflictingNode only accepts a hash already present in the
// subtree and deduplicates it, so a well-formed subtree always satisfies
// len(ConflictingNodes) <= len(Nodes). A larger count is malformed; both paths
// must reject it up front by the invariant rather than reading a bogus array or,
// in the seeking case, being driven into work sized by the attacker's count.
func TestDeserializeSubtreeConflictingFromReader_ConflictingExceedsLeaves(t *testing.T) {
	const numNodes = 8

	serialized, _ := buildSubtreeWithConflicting(t, numNodes, 2)

	tests := []struct {
		name  string
		count uint64
	}{
		{name: "one over the leaf count", count: numNodes + 1},
		{name: "an allocation-sized count", count: 1 << 35},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			corrupted := make([]byte, len(serialized))
			copy(corrupted, serialized)
			binary.LittleEndian.PutUint64(corrupted[conflictingCountOffset(numNodes):], tt.count)

			_, seekErr := DeserializeSubtreeConflictingFromReader(bytes.NewReader(corrupted))
			_, streamErr := DeserializeSubtreeConflictingFromReader(nonSeekableReader{r: bytes.NewReader(corrupted)})

			require.ErrorIs(t, seekErr, ErrConflictingCountExceedsLeaves, "seeking path must reject a count above the leaf count")
			require.ErrorIs(t, streamErr, ErrConflictingCountExceedsLeaves, "streaming path must reject a count above the leaf count")
		})
	}
}

// TestDeserializeSubtreeConflictingFromReader_ConflictingEqualsLeaves pins the
// boundary: a count equal to the leaf count is permitted by the invariant (the
// check is strictly greater-than, not >=), so a file that claims it must fail
// only because the promised hashes are not present — never with the invariant
// error. This guards against tightening the comparison to >= and rejecting a
// theoretically valid subtree.
func TestDeserializeSubtreeConflictingFromReader_ConflictingEqualsLeaves(t *testing.T) {
	const numNodes = 8

	// The helper caps conflicting at numNodes-1, so the physical trailer holds
	// fewer hashes than the mutated count promises; the read runs short.
	serialized, _ := buildSubtreeWithConflicting(t, numNodes, numNodes-1)

	corrupted := make([]byte, len(serialized))
	copy(corrupted, serialized)
	binary.LittleEndian.PutUint64(corrupted[conflictingCountOffset(numNodes):], numNodes)

	_, seekErr := DeserializeSubtreeConflictingFromReader(bytes.NewReader(corrupted))
	_, streamErr := DeserializeSubtreeConflictingFromReader(nonSeekableReader{r: bytes.NewReader(corrupted)})

	require.Error(t, seekErr)
	require.Error(t, streamErr)
	require.NotErrorIs(t, seekErr, ErrConflictingCountExceedsLeaves, "count == leaves must not trip the invariant")
	require.NotErrorIs(t, streamErr, ErrConflictingCountExceedsLeaves, "count == leaves must not trip the invariant")
}

// TestDeserializeSubtreeConflictingFromReader_LeafCountOverflowsIntMultiply pins
// the streaming path's overflow guard. That path skips the node array with
// bufio.Discard(nodeSerializedLen * numLeavesInt), a product computed in
// platform-sized int, so the guard must reject at MaxInt/nodeSerializedLen — not
// MaxInt64. On a 32-bit build the two differ by orders of magnitude: a count
// just past this boundary passes safe.Uint64ToInt yet overflows the
// multiplication, which a MaxInt64 bound would have waved through. The value is
// well below MaxInt64/nodeSerializedLen, so a regression to that looser bound
// fails this test on every architecture.
func TestDeserializeSubtreeConflictingFromReader_LeafCountOverflowsIntMultiply(t *testing.T) {
	serialized, _ := buildSubtreeWithConflicting(t, 8, 2)

	corrupted := make([]byte, len(serialized))
	copy(corrupted, serialized)

	overflowing := uint64(math.MaxInt)/nodeSerializedLen + 1
	binary.LittleEndian.PutUint64(corrupted[rootHashPlusFeesPlusSizeLen:], overflowing)

	_, streamErr := DeserializeSubtreeConflictingFromReader(nonSeekableReader{r: bytes.NewReader(corrupted)})
	require.ErrorIs(t, streamErr, ErrNumLeavesOutOfRange, "streaming path must reject a leaf count that overflows the int byte-length computation")
}

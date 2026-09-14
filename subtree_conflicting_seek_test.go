package subtree

import (
	"bytes"
	"io"
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

func buildSubtreeWithConflicting(t *testing.T, numNodes int, numConflicting int) ([]byte, []chainhash.Hash) {
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
			require.Equal(t, len(expected), len(streamed))
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

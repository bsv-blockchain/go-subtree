package subtree_test

import (
	"crypto/rand"
	"encoding/binary"
	"testing"

	"github.com/bsv-blockchain/go-bt/v2/chainhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bsv-blockchain/go-subtree"
)

func BenchmarkSubtreeAddNode(b *testing.B) {
	st, err := subtree.NewIncompleteTreeByLeafCount(b.N)
	require.NoError(b, err)

	// create a slice of random hashes
	hashes := make([]chainhash.Hash, b.N)

	b32 := make([]byte, 32)

	for i := 0; i < b.N; i++ {
		// create random 32 bytes
		_, _ = rand.Read(b32)
		hashes[i] = chainhash.Hash(b32)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = st.AddNode(hashes[i], 111, 0)
	}
}

func BenchmarkSubtreeSerialize(b *testing.B) {
	st, err := subtree.NewIncompleteTreeByLeafCount(b.N)
	require.NoError(b, err)

	for i := 0; i < b.N; i++ {
		// int to bytes
		var bb [32]byte

		binary.LittleEndian.PutUint32(bb[:], uint32(i)) //nolint:gosec // G115: integer overflow conversion int -> uint32
		_ = st.AddNode(*(*chainhash.Hash)(&bb), 111, 234)
	}

	b.ResetTimer()

	ser, err := st.Serialize()
	require.NoError(b, err)
	assert.GreaterOrEqual(b, len(ser), 48*b.N)
}

func BenchmarkSubtreeSerializeNodes(b *testing.B) {
	st, err := subtree.NewIncompleteTreeByLeafCount(b.N)
	require.NoError(b, err)

	for i := 0; i < b.N; i++ {
		// int to bytes
		var bb [32]byte

		binary.LittleEndian.PutUint32(bb[:], uint32(i)) //nolint:gosec // G115: integer overflow conversion int -> uint32
		_ = st.AddNode(*(*chainhash.Hash)(&bb), 111, 234)
	}

	b.ResetTimer()

	ser, err := st.SerializeNodes()
	require.NoError(b, err)
	assert.GreaterOrEqual(b, len(ser), 32*b.N)
}

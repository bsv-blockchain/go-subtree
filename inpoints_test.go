package subtree

import (
	"bytes"
	"testing"

	"github.com/bsv-blockchain/go-bt/v2/chainhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// txInpointsFromParentVouts builds a TxInpoints with a single parent hash and
// the given vouts. Used by tests to replace the previous struct-literal
// construction with Idxs: [][]uint32{{...}}.
func txInpointsFromParentVouts(parent chainhash.Hash, vouts ...uint32) TxInpoints {
	p := NewTxInpoints()
	for _, v := range vouts {
		p.appendInput(parent, v)
	}

	return p
}

func TestTxInpoints(t *testing.T) {
	t.Run("TestTxInpoints", func(t *testing.T) {
		p, err := NewTxInpointsFromTx(tx)
		require.NoError(t, err)

		require.Len(t, p.ParentTxHashes, 1)

		vouts, err := p.GetParentVoutsAtIndex(0)
		require.NoError(t, err)
		require.Len(t, vouts, 1)
	})

	t.Run("serialize", func(t *testing.T) {
		p, err := NewTxInpointsFromTx(tx)
		require.NoError(t, err)

		b, err := p.Serialize()
		require.NoError(t, err)
		require.Len(t, b, 44)

		p2, err := NewTxInpointsFromBytes(b)
		require.NoError(t, err)

		require.Len(t, p2.ParentTxHashes, 1)

		vouts1, err := p.GetParentVoutsAtIndex(0)
		require.NoError(t, err)

		vouts2, err := p2.GetParentVoutsAtIndex(0)
		require.NoError(t, err)

		require.Len(t, vouts2, 1)
		assert.Equal(t, p.ParentTxHashes[0], p2.ParentTxHashes[0])
		assert.Equal(t, vouts1[0], vouts2[0])
	})

	t.Run("serialize with mismatched parent/vout state", func(t *testing.T) {
		// Construct an inconsistent state: one parent hash but no count word
		// in voutIdxs. With the field unexported this state can only be
		// fabricated from within the package — Serialize must still detect it.
		p := NewTxInpoints()
		p.ParentTxHashes = []chainhash.Hash{chainhash.HashH([]byte("test"))}
		p.voutIdxs = nil

		_, err := p.Serialize()
		require.Error(t, err)
	})

	t.Run("from inputs", func(t *testing.T) {
		p, err := NewTxInpointsFromTx(tx)
		require.NoError(t, err)

		p2, err := NewTxInpointsFromInputs(tx.Inputs)
		require.NoError(t, err)

		// make sure they are the same
		require.Len(t, p2.ParentTxHashes, len(p.ParentTxHashes))

		v1, err := p.GetParentVoutsAtIndex(0)
		require.NoError(t, err)

		v2, err := p2.GetParentVoutsAtIndex(0)
		require.NoError(t, err)

		assert.Equal(t, p.ParentTxHashes[0], p2.ParentTxHashes[0])
		assert.Equal(t, v1, v2)
	})
}

func TestGetTxInpoints(t *testing.T) {
	p, err := NewTxInpointsFromTx(tx)
	require.NoError(t, err)

	// Test getting inpoints
	inpoints := p.GetTxInpoints()
	assert.Len(t, inpoints, 1)
	assert.Equal(t, uint32(5), inpoints[0].Index)
	assert.Equal(t, *tx.Inputs[0].PreviousTxIDChainHash(), inpoints[0].Hash)
}

func TestGetParentTxHashAtIndex(t *testing.T) {
	t.Run("TestGetParentTxHashAtIndex", func(t *testing.T) {
		p, err := NewTxInpointsFromTx(tx)
		require.NoError(t, err)

		// Test getting parent tx hash at index
		hash, err := p.GetParentTxHashAtIndex(0)
		require.NoError(t, err)

		assert.Equal(t, *tx.Inputs[0].PreviousTxIDChainHash(), hash)
	})

	t.Run("out of range", func(t *testing.T) {
		p, err := NewTxInpointsFromTx(tx)
		require.NoError(t, err)

		// Test getting parent tx hash at index
		hash, err := p.GetParentTxHashAtIndex(1)
		require.Error(t, err)

		assert.Equal(t, chainhash.Hash{}, hash)
	})
}

func TestGetParentVoutsAtIndex(t *testing.T) {
	t.Run("TestGetParentVoutsAtIndex", func(t *testing.T) {
		p, err := NewTxInpointsFromTx(tx)
		require.NoError(t, err)

		// Test getting parent vouts at index
		vouts, err := p.GetParentVoutsAtIndex(0)
		require.NoError(t, err)

		require.Len(t, vouts, 1)
		assert.Equal(t, uint32(5), vouts[0])
	})

	t.Run("out of range", func(t *testing.T) {
		p, err := NewTxInpointsFromTx(tx)
		require.NoError(t, err)

		// Test getting parent vouts at index
		vouts, err := p.GetParentVoutsAtIndex(1)
		require.Error(t, err)

		assert.Nil(t, vouts)
	})
}

func TestTxInpoints_DedupAndRoundTrip(t *testing.T) {
	// Build inputs where two inputs share parent A (vouts 7 then 9) and
	// one input pulls from parent B (vout 3). Order matters: A then B then A.
	a := chainhash.HashH([]byte("parent-a"))
	b := chainhash.HashH([]byte("parent-b"))

	p := NewTxInpoints()
	p.appendInput(a, 7)
	p.appendInput(b, 3)
	p.appendInput(a, 9)

	require.Equal(t, []chainhash.Hash{a, b}, p.ParentTxHashes)

	voutsA, err := p.GetParentVoutsAtIndex(0)
	require.NoError(t, err)
	require.Equal(t, []uint32{7, 9}, voutsA)

	voutsB, err := p.GetParentVoutsAtIndex(1)
	require.NoError(t, err)
	require.Equal(t, []uint32{3}, voutsB)

	require.Equal(t, 3, p.nrInputs())

	// Round-trip through wire format.
	raw, err := p.Serialize()
	require.NoError(t, err)

	q, err := NewTxInpointsFromBytes(raw)
	require.NoError(t, err)

	require.Equal(t, p.ParentTxHashes, q.ParentTxHashes)
	require.Equal(t, p.voutIdxs, q.voutIdxs)

	// GetTxInpoints flattens in parent-then-vout order.
	flat := q.GetTxInpoints()
	require.Equal(t, []Inpoint{{a, 7}, {a, 9}, {b, 3}}, flat)
}

func TestString(t *testing.T) {
	p, err := NewTxInpointsFromTx(tx)
	require.NoError(t, err)

	// Test String method — format kept compatible with the pre-packed version.
	str := p.String()
	assert.NotEmpty(t, str)
	assert.Contains(t, str, "TxInpoints")
	assert.Contains(t, str, "ParentTxHashes")
	assert.Contains(t, str, "Idxs")
}

func TestLen32(t *testing.T) {
	t.Run("nil slice", func(t *testing.T) {
		var nilSlice []int
		result := len32(nilSlice)
		assert.Equal(t, uint32(0), result)
	})

	t.Run("normal slice", func(t *testing.T) {
		normalSlice := []int{1, 2, 3, 4, 5}
		result := len32(normalSlice)
		assert.Equal(t, uint32(5), result)
	})

	t.Run("empty slice", func(t *testing.T) {
		emptySlice := make([]int, 0)
		result := len32(emptySlice)
		assert.Equal(t, uint32(0), result)
	})
}

func TestNewTxInpointsFromBytesError(t *testing.T) {
	t.Run("invalid bytes", func(t *testing.T) {
		invalidBytes := []byte{0x01, 0x02, 0x03}
		_, err := NewTxInpointsFromBytes(invalidBytes)
		require.Error(t, err)
	})

	t.Run("empty bytes", func(t *testing.T) {
		emptyBytes := make([]byte, 4)
		// This creates bytes representing 0 parent inpoints
		p, err := NewTxInpointsFromBytes(emptyBytes)
		require.NoError(t, err)
		assert.Empty(t, p.ParentTxHashes)
	})
}

func TestNewTxInpointsFromReaderError(t *testing.T) {
	t.Run("invalid reader", func(t *testing.T) {
		invalidBytes := []byte{0x01, 0x02, 0x03}
		_, err := NewTxInpointsFromReader(bytes.NewReader(invalidBytes))
		require.Error(t, err)
	})
}

func TestNewTxInpointsFromPacked(t *testing.T) {
	a := chainhash.HashH([]byte("a"))
	b := chainhash.HashH([]byte("b"))

	t.Run("single parent single vout", func(t *testing.T) {
		// layout: [count=1, vout=7]
		p := NewTxInpointsFromPacked([]chainhash.Hash{a}, []uint32{1, 7})
		require.Equal(t, []chainhash.Hash{a}, p.ParentTxHashes)

		vouts, err := p.GetParentVoutsAtIndex(0)
		require.NoError(t, err)
		require.Equal(t, []uint32{7}, vouts)
	})

	t.Run("two parents multiple vouts", func(t *testing.T) {
		// layout: [count=2, 4, 5, count=1, 9]
		p := NewTxInpointsFromPacked(
			[]chainhash.Hash{a, b},
			[]uint32{2, 4, 5, 1, 9},
		)

		v0, err := p.GetParentVoutsAtIndex(0)
		require.NoError(t, err)
		require.Equal(t, []uint32{4, 5}, v0)

		v1, err := p.GetParentVoutsAtIndex(1)
		require.NoError(t, err)
		require.Equal(t, []uint32{9}, v1)

		require.Equal(t, 3, p.nrInputs())
	})

	t.Run("aliases input slices", func(t *testing.T) {
		parents := []chainhash.Hash{a}
		vouts := []uint32{1, 42}

		p := NewTxInpointsFromPacked(parents, vouts)

		// Mutate the caller-owned slices and observe TxInpoints sees it —
		// this is the contract of the function (caller must keep storage
		// stable for the lifetime of the TxInpoints).
		vouts[1] = 1234

		got, err := p.GetParentVoutsAtIndex(0)
		require.NoError(t, err)
		require.Equal(t, uint32(1234), got[0])
	})

	t.Run("round-trip with NewTxInpointsFromInputs", func(t *testing.T) {
		// Build with the slice-construction path, then re-wrap via Packed —
		// the inner fields must be byte-identical.
		original, err := NewTxInpointsFromTx(tx)
		require.NoError(t, err)

		wrapped := NewTxInpointsFromPacked(original.ParentTxHashes, original.voutIdxs)
		require.Equal(t, original.ParentTxHashes, wrapped.ParentTxHashes)
		require.Equal(t, original.voutIdxs, wrapped.voutIdxs)
	})
}

// BenchmarkNewTxInpointsFromPacked measures the hot path block-assembly takes
// after PR2 in teranode — pre-packed slices arrive over gRPC and TxInpoints
// aliases them with zero allocation.
func BenchmarkNewTxInpointsFromPacked(b *testing.B) {
	parents := []chainhash.Hash{chainhash.HashH([]byte("a"))}
	vouts := []uint32{1, 5}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewTxInpointsFromPacked(parents, vouts)
	}
}

func BenchmarkNewTxInpoints(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := NewTxInpointsFromTx(tx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

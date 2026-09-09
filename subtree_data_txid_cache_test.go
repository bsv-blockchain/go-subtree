package subtree

import (
	"bytes"
	"testing"

	"github.com/bsv-blockchain/go-bt/v2"
	"github.com/bsv-blockchain/go-bt/v2/chainhash"
	"github.com/stretchr/testify/require"
)

// txIDSink stops the compiler proving the id is unused and eliding the call it
// is there to measure.
var txIDSink *chainhash.Hash

// TestDeserializedTransactionsCarryTheirID pins the transaction id that
// deserialization already computes onto the transaction it belongs to.
//
// serializeFromReader hashes every transaction it reads, so it can check the id
// against the node hash the subtree already holds. That check is necessary. What
// was wasteful is that the computed id was then thrown away, because
// bt.Tx.TxIDChainHash reads the transaction's cache but never fills it; only
// SetTxHash does, which go-bt documents. Every later caller therefore serialized
// the whole transaction again and hashed it again.
//
// Measured on a Teranode node writing a mainnet block, the two halves cost
// almost exactly the same: 10.10 core-seconds inside this function, and 9.17
// core-seconds recomputing the identical values one stage later.
//
// The property is asserted by allocation count rather than by inspecting the
// cache, because go-bt exports no predicate for it and because allocations are
// what the caller feels. TxIDChainHash serializes the whole transaction and
// hashes it when the cache is empty, and returns a stored pointer when it is
// not, so a cached transaction answers for free.
func TestDeserializedTransactionsCarryTheirID(t *testing.T) {
	subtree, subtreeData := setupData(t)

	serialized, err := subtreeData.Serialize()
	require.NoError(t, err)

	read, err := NewSubtreeDataFromReader(subtree, bytes.NewReader(serialized))
	require.NoError(t, err)

	for i, got := range read.Txs {
		if got == nil {
			continue
		}

		readTx := got

		allocs := testing.AllocsPerRun(20, func() { txIDSink = readTx.TxIDChainHash() })
		require.Zero(t, allocs,
			"asking transaction %d for its id allocated %.0f time(s), so it came back uncached and every later caller re-hashes it",
			i, allocs)

		require.Equal(t, subtree.Nodes[i].Hash, *got.TxIDChainHash(),
			"transaction %d has the wrong id", i)
	}
}

// TestCachedIDSurvivesExtension is the correctness question behind caching it.
//
// Teranode extends these transactions after deserialization, filling in each
// input's parent satoshis and locking script. Caching an id would be a bug if
// extending changed it. It does not: the id is defined over the standard
// serialization, and go-bt's Bytes() always writes that form even for an
// extended transaction. This holds go-bt to that, since the cache would
// otherwise go stale silently and surface much later as a hash mismatch.
func TestCachedIDSurvivesExtension(t *testing.T) {
	subtree, subtreeData := setupData(t)

	serialized, err := subtreeData.Serialize()
	require.NoError(t, err)

	read, err := NewSubtreeDataFromReader(subtree, bytes.NewReader(serialized))
	require.NoError(t, err)

	for i, got := range read.Txs {
		if got == nil || got.IsCoinbase() {
			continue
		}

		before := *got.TxIDChainHash()

		for _, in := range got.Inputs {
			in.PreviousTxSatoshis = 1234
			in.PreviousTxScript = got.Outputs[0].LockingScript
		}

		// Recompute from scratch rather than reading the cache, or this checks
		// nothing at all.
		fresh := &bt.Tx{}
		_, err = fresh.ReadFrom(bytes.NewReader(got.Bytes()))
		require.NoError(t, err)

		require.Equal(t, before, *fresh.TxIDChainHash(),
			"extending transaction %d changed its id, so caching it at deserialization would be wrong", i)
	}
}

// TestDeserializedCoinbaseCarriesItsID covers the branch the test above cannot
// reach.
//
// The stream is hand-built rather than produced by Serialize, because Serialize
// deliberately omits the coinbase: it starts at index one whenever node zero is
// the coinbase placeholder. The reading side still handles a coinbase arriving
// first, for data written by a producer that includes it, and that branch has
// no node hash to check against, so it was the one path that returned a
// transaction with no id.
//
// One transaction in four thousand does not pay for itself. A uniform contract
// does: "every transaction this returns knows its id" is something a caller can
// rely on, where "every transaction except the coinbase" is a footnote nobody
// reads.
func TestDeserializedCoinbaseCarriesItsID(t *testing.T) {
	coinbaseTx, err := bt.NewTxFromString("02000000010000000000000000000000000000000000000000000000000000000000000000ffffffff03510101ffffffff0100f2052a01000000232103656065e6886ca1e947de3471c9e723673ab6ba34724476417fa9fcef8bafa604ac00000000")
	require.NoError(t, err)
	require.True(t, coinbaseTx.IsCoinbase(), "the fixture must be a coinbase")

	other := tx.Clone()
	other.Version = 7

	subtree, err := NewTree(2)
	require.NoError(t, err)

	require.NoError(t, subtree.AddCoinbaseNode())
	require.NoError(t, subtree.AddNode(*other.TxIDChainHash(), 111, 0))

	var stream bytes.Buffer

	stream.Write(coinbaseTx.SerializeBytes())
	stream.Write(other.SerializeBytes())

	read, err := NewSubtreeDataFromReader(subtree, bytes.NewReader(stream.Bytes()))
	require.NoError(t, err)

	require.NotNil(t, read.Txs[0], "the coinbase should have been placed first")
	require.True(t, read.Txs[0].IsCoinbase())

	got := read.Txs[0]

	allocs := testing.AllocsPerRun(20, func() { txIDSink = got.TxIDChainHash() })
	require.Zero(t, allocs, "the coinbase came back uncached, so every later caller re-hashes it")

	require.Equal(t, *coinbaseTx.TxIDChainHash(), *got.TxIDChainHash(), "the coinbase id is wrong")
}

// BenchmarkDeserializeThenIdentify measures what a caller actually does: read a
// subtree's transactions back from its data file, then ask each of them for its
// id, which is what the next stage of block validation needs.
//
// The second half used to repeat the whole of the first half's hashing.
func BenchmarkDeserializeThenIdentify(b *testing.B) {
	const count = 4096

	subtree, err := NewTreeByLeafCount(count)
	require.NoError(b, err)

	txs := make([]*bt.Tx, count)

	for i := 0; i < count; i++ {
		t := tx.Clone()
		t.Version = uint32(i + 1)
		txs[i] = t

		require.NoError(b, subtree.AddNode(*t.TxIDChainHash(), 111, uint64(i)))
	}

	subtreeData := NewSubtreeData(subtree)
	for i, t := range txs {
		require.NoError(b, subtreeData.AddTx(t, i))
	}

	serialized, err := subtreeData.Serialize()
	require.NoError(b, err)

	b.SetBytes(int64(len(serialized)))
	b.ReportAllocs()
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		read, rErr := NewSubtreeDataFromReader(subtree, bytes.NewReader(serialized))
		if rErr != nil {
			b.Fatal(rErr)
		}

		for _, t := range read.Txs {
			if t != nil {
				txIDSink = t.TxIDChainHash()
			}
		}
	}
}

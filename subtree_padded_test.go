package subtree

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/bsv-blockchain/go-bt/v2/chainhash"
	"github.com/stretchr/testify/require"
)

func TestRootHashPadded_LiftsTwoLeafSubtreeToHeightEight(t *testing.T) {
	st, err := NewTreeByLeafCount(256)
	require.NoError(t, err)

	h1, _ := chainhash.NewHashFromStr("97af9ad3583e2f83fc1e44e475e3a3ee31ec032449cc88b491479ef7d187c115")
	h2, _ := chainhash.NewHashFromStr("7ce05dda56bc523048186c0f0474eb21c92fe35de6d014bd016834637a3ed08d")

	require.NoError(t, st.AddNode(*h1, 0, 0))
	require.NoError(t, st.AddNode(*h2, 0, 0))

	lifted, err := st.RootHashPadded(8)
	require.NoError(t, err)
	require.NotNil(t, lifted)

	expected := *st.RootHash()
	for range 7 {
		var buf [64]byte
		copy(buf[0:32], expected[:])
		copy(buf[32:64], expected[:])
		first := sha256.Sum256(buf[:])
		expected = chainhash.Hash(sha256.Sum256(first[:]))
	}

	require.Equal(t, expected.String(), lifted.String())
}

func TestRootHashPadded_NoLiftWhenAlreadyAtTargetHeight(t *testing.T) {
	st, err := NewTreeByLeafCount(4)
	require.NoError(t, err)

	for i := range 4 {
		h := chainhash.HashH([]byte{byte(i)})
		require.NoError(t, st.AddNode(h, 0, 0))
	}

	padded, err := st.RootHashPadded(2)
	require.NoError(t, err)
	require.Equal(t, st.RootHash().String(), padded.String())
}

func TestRootHashPadded_EmptySubtreeReturnsNil(t *testing.T) {
	st, err := NewTreeByLeafCount(4)
	require.NoError(t, err)

	padded, err := st.RootHashPadded(2)
	require.NoError(t, err)
	require.Nil(t, padded)
}

// TestRootHashPadded_AcceptsNonPowerOfTwoLeafCount verifies that RootHashPadded
// accepts subtrees whose leaf count is not a power of two and returns a lifted
// root that matches the canonical flat merkle tree at the same height. The
// duplicate-when-odd rule applied inside BuildMerkleTreeStoreFromBytes already
// pads the natural root to height ceil(log2(length)), so no special handling
// is needed at the caller.
func TestRootHashPadded_AcceptsNonPowerOfTwoLeafCount(t *testing.T) {
	st, err := NewIncompleteTreeByLeafCount(3)
	require.NoError(t, err)

	h := chainhash.HashH([]byte{0x01})
	require.NoError(t, st.AddNode(h, 0, 0))
	h2 := chainhash.HashH([]byte{0x02})
	require.NoError(t, st.AddNode(h2, 0, 0))
	h3 := chainhash.HashH([]byte{0x03})
	require.NoError(t, st.AddNode(h3, 0, 0))

	// ceil(log2(3)) == 2, so the natural root already sits at height 2.
	padded, err := st.RootHashPadded(2)
	require.NoError(t, err)
	require.Equal(t, st.RootHash().String(), padded.String())

	// Lifting one level above the natural height should self-hash once.
	lifted, err := st.RootHashPadded(3)
	require.NoError(t, err)

	expected := *st.RootHash()
	var buf [64]byte
	copy(buf[0:32], expected[:])
	copy(buf[32:64], expected[:])
	first := sha256.Sum256(buf[:])
	expected = chainhash.Hash(sha256.Sum256(first[:]))

	require.Equal(t, expected.String(), lifted.String())
}

func TestRootHashPadded_TargetHeightTooSmallErrors(t *testing.T) {
	st, err := NewTreeByLeafCount(4)
	require.NoError(t, err)

	for i := range 4 {
		h := chainhash.HashH([]byte{byte(i)})
		require.NoError(t, st.AddNode(h, 0, 0))
	}

	_, err = st.RootHashPadded(1)
	require.ErrorIs(t, err, ErrTargetHeightTooSmall)
}

func TestRootHashPadded_MatchesBigTreeRoot(t *testing.T) {
	bigTree, err := NewTreeByLeafCount(16)
	require.NoError(t, err)

	hashes := make([]chainhash.Hash, 10)
	for i := range hashes {
		hashes[i] = chainhash.HashH([]byte{byte(i + 1)})
		require.NoError(t, bigTree.AddNode(hashes[i], 0, 0))
	}

	left, err := NewTreeByLeafCount(8)
	require.NoError(t, err)
	for i := range 8 {
		require.NoError(t, left.AddNode(hashes[i], 0, 0))
	}

	right, err := NewTreeByLeafCount(8)
	require.NoError(t, err)
	for i := 8; i < 10; i++ {
		require.NoError(t, right.AddNode(hashes[i], 0, 0))
	}

	leftRoot, err := left.RootHashPadded(3)
	require.NoError(t, err)
	rightRoot, err := right.RootHashPadded(3)
	require.NoError(t, err)

	top, err := NewTreeByLeafCount(2)
	require.NoError(t, err)
	require.NoError(t, top.AddNode(*leftRoot, 0, 0))
	require.NoError(t, top.AddNode(*rightRoot, 0, 0))

	require.Equal(t, bigTree.RootHash().String(), top.RootHash().String())
}

// TestRootHashPadded_MatchesBitcoinRootAt258Txs proves at block scale that the
// bitcoin merkle root of N transactions equals the root produced by composing
// two fixed-height subtree roots — a complete left subtree and a partially-
// filled right subtree lifted to the same height via RootHashPadded.
//
// Tree 1 holds all 258 leaves directly. NewIncompleteTreeByLeafCount(258)
// rounds the capacity up to 512 (the next power of two), and the merkle
// builder pads the unused slots with the zero hash; calcMerkle's "duplicate
// the left child when the right is empty" rule then collapses those zeros
// exactly the way bitcoin's "duplicate-last-when-odd" rule does, level by
// level, so tree 1's RootHash() is the bitcoin-correct merkle root.
func TestRootHashPadded_MatchesBitcoinRootAt258Txs(t *testing.T) {
	const (
		totalTxs        = 258
		subtreeCapacity = 256
	)

	txHashes := make([]chainhash.Hash, totalTxs)
	for i := range txHashes {
		txHashes[i] = chainhash.HashH([]byte{byte(i), byte(i >> 8)})
	}

	tree1, err := NewIncompleteTreeByLeafCount(totalTxs)
	require.NoError(t, err)
	for _, h := range txHashes {
		require.NoError(t, tree1.AddNode(h, 0, 0))
	}
	bitcoinRoot := tree1.RootHash()
	require.NotNil(t, bitcoinRoot)

	tree2, err := NewTreeByLeafCount(subtreeCapacity)
	require.NoError(t, err)
	for i := range subtreeCapacity {
		require.NoError(t, tree2.AddNode(txHashes[i], 0, 0))
	}

	tree3, err := NewTreeByLeafCount(subtreeCapacity)
	require.NoError(t, err)
	require.NoError(t, tree3.AddNode(txHashes[256], 0, 0))
	require.NoError(t, tree3.AddNode(txHashes[257], 0, 0))
	tree3Root, err := tree3.RootHashPadded(tree2.Height)
	require.NoError(t, err)

	tree4, err := NewTreeByLeafCount(2)
	require.NoError(t, err)
	require.NoError(t, tree4.AddNode(*tree2.RootHash(), 0, 0))
	require.NoError(t, tree4.AddNode(*tree3Root, 0, 0))

	require.Equal(t, bitcoinRoot.String(), tree4.RootHash().String())
}

// TestRootHashPadded_MatchesFlatTreeForNonPowerOfTwoFinal sweeps a range of
// non-power-of-two leaf counts in the final subtree and verifies that the
// composed root (complete left subtree + lifted right subtree) equals the
// canonical flat merkle root for each case. The chosen `r` values cover
// different duplicate-when-odd cascade depths within a capacity-32 subtree.
func TestRootHashPadded_MatchesFlatTreeForNonPowerOfTwoFinal(t *testing.T) {
	const subtreeSize = 32

	rValues := []int{1, 2, 3, 5, 6, 7, 9, 11, 13, 15, 17, 21, 23, 27, 31}

	for _, r := range rValues {
		t.Run(fmt.Sprintf("final_subtree_has_%d_leaves", r), func(t *testing.T) {
			totalTxs := subtreeSize + r

			txHashes := make([]chainhash.Hash, totalTxs)
			for i := range txHashes {
				txHashes[i] = chainhash.HashH([]byte{byte(i), byte(i >> 8)})
			}

			reference, err := NewIncompleteTreeByLeafCount(totalTxs)
			require.NoError(t, err)
			for _, h := range txHashes {
				require.NoError(t, reference.AddNode(h, 0, 0))
			}
			referenceRoot := reference.RootHash()
			require.NotNil(t, referenceRoot)

			left, err := NewTreeByLeafCount(subtreeSize)
			require.NoError(t, err)
			for i := range subtreeSize {
				require.NoError(t, left.AddNode(txHashes[i], 0, 0))
			}

			right, err := NewIncompleteTreeByLeafCount(r)
			require.NoError(t, err)
			for i := subtreeSize; i < totalTxs; i++ {
				require.NoError(t, right.AddNode(txHashes[i], 0, 0))
			}
			require.Equal(t, r, right.Length())

			rightLifted, err := right.RootHashPadded(left.Height)
			require.NoError(t, err)

			top, err := NewTreeByLeafCount(2)
			require.NoError(t, err)
			require.NoError(t, top.AddNode(*left.RootHash(), 0, 0))
			require.NoError(t, top.AddNode(*rightLifted, 0, 0))

			require.Equal(t, referenceRoot.String(), top.RootHash().String(),
				"composed root must equal canonical flat root for r=%d", r)
		})
	}
}

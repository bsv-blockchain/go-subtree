package subtree

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/bsv-blockchain/go-bt/v2/chainhash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTree(t *testing.T) {
	t.Run("invalid size", func(t *testing.T) {
		_, err := NewTreeByLeafCount(123)
		require.Error(t, err)
	})

	t.Run("valid size", func(t *testing.T) {
		st, err := NewTree(20)
		require.NoError(t, err)

		if st.Size() != 1048576 {
			t.Errorf("expected size to be 1048576, got %d", st.Size())
		}
	})
}

func TestNewIncompleteTreeByLeafCount(t *testing.T) {
	t.Run("invalid size", func(t *testing.T) {
		_, err := NewIncompleteTreeByLeafCount(0)
		require.Error(t, err)
	})

	t.Run("valid size", func(t *testing.T) {
		st, err := NewIncompleteTreeByLeafCount(20)
		require.NoError(t, err)

		// should be the next power of 2
		assert.Equal(t, 32, st.Size())
	})
}

func TestRootHash(t *testing.T) {
	t.Run("root hash", func(t *testing.T) {
		st, err := NewTree(2)
		require.NoError(t, err)

		if st.Size() != 4 {
			t.Errorf("expected size to be 4, got %d", st.Size())
		}

		hash1, _ := chainhash.NewHashFromStr("97af9ad3583e2f83fc1e44e475e3a3ee31ec032449cc88b491479ef7d187c115")
		hash2, _ := chainhash.NewHashFromStr("7ce05dda56bc523048186c0f0474eb21c92fe35de6d014bd016834637a3ed08d")
		hash3, _ := chainhash.NewHashFromStr("3070fb937289e24720c827cbc24f3fce5c361cd7e174392a700a9f42051609e0")
		hash4, _ := chainhash.NewHashFromStr("d3cde0ab7142cc99acb31c5b5e1e941faed1c5cf5f8b63ed663972845d663487")
		_ = st.AddNode(*hash1, 111, 0)
		_ = st.AddNode(*hash2, 111, 0)
		_ = st.AddNode(*hash3, 111, 0)
		_ = st.AddNode(*hash4, 111, 0)

		rootHash := st.RootHash()
		assert.Equal(t, "b47df6aa4fe0a1d3841c635444be4e33eb8cdc2f2e929ced06d0a8454fb28225", rootHash.String())
	})
}

func Test_RootHashWithReplaceRootNode(t *testing.T) {
	t.Run("empty tree", func(t *testing.T) {
		st, err := NewTree(2)
		require.NoError(t, err)

		rootHash := st.RootHash()
		assert.Nil(t, rootHash)
	})

	t.Run("replace with 0 noded", func(t *testing.T) {
		st, err := NewTree(2)
		require.NoError(t, err)

		hash1, _ := chainhash.NewHashFromStr("97af9ad3583e2f83fc1e44e475e3a3ee31ec032449cc88b491479ef7d187c115")

		rootHash := st.RootHash()
		assert.Nil(t, rootHash)

		rootHash2, err := st.RootHashWithReplaceRootNode(hash1, 111, 0)
		require.NoError(t, err)
		assert.NotEqual(t, rootHash, rootHash2)
		assert.Equal(t, "97af9ad3583e2f83fc1e44e475e3a3ee31ec032449cc88b491479ef7d187c115", rootHash2.String())
	})

	t.Run("replace with only 1 node", func(t *testing.T) {
		st, err := NewTree(2)
		require.NoError(t, err)

		if st.Size() != 4 {
			t.Errorf("expected size to be 4, got %d", st.Size())
		}

		_ = st.AddCoinbaseNode()

		hash1, _ := chainhash.NewHashFromStr("97af9ad3583e2f83fc1e44e475e3a3ee31ec032449cc88b491479ef7d187c115")

		rootHash := st.RootHash()
		assert.Equal(t, "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", rootHash.String())

		rootHash2, err := st.RootHashWithReplaceRootNode(hash1, 111, 0)
		require.NoError(t, err)
		assert.NotEqual(t, rootHash, rootHash2)
		assert.Equal(t, "97af9ad3583e2f83fc1e44e475e3a3ee31ec032449cc88b491479ef7d187c115", rootHash2.String())
	})

	t.Run("root hash with replace root node", func(t *testing.T) {
		st, err := NewTree(2)
		require.NoError(t, err)

		if st.Size() != 4 {
			t.Errorf("expected size to be 4, got %d", st.Size())
		}

		hash1, _ := chainhash.NewHashFromStr("97af9ad3583e2f83fc1e44e475e3a3ee31ec032449cc88b491479ef7d187c115")
		hash2, _ := chainhash.NewHashFromStr("7ce05dda56bc523048186c0f0474eb21c92fe35de6d014bd016834637a3ed08d")
		hash3, _ := chainhash.NewHashFromStr("3070fb937289e24720c827cbc24f3fce5c361cd7e174392a700a9f42051609e0")
		hash4, _ := chainhash.NewHashFromStr("d3cde0ab7142cc99acb31c5b5e1e941faed1c5cf5f8b63ed663972845d663487")
		_ = st.AddNode(*hash1, 111, 0)
		_ = st.AddNode(*hash2, 111, 0)
		_ = st.AddNode(*hash3, 111, 0)
		_ = st.AddNode(*hash4, 111, 0)

		rootHash := st.RootHash()
		assert.Equal(t, "b47df6aa4fe0a1d3841c635444be4e33eb8cdc2f2e929ced06d0a8454fb28225", rootHash.String())

		rootHash2, err := st.RootHashWithReplaceRootNode(hash4, 111, 0)
		require.NoError(t, err)
		assert.NotEqual(t, rootHash, rootHash2)
		assert.Equal(t, "dfec71cf72403643187e9e02d7c436e87251fa098cffa54d182022153da3d09a", rootHash2.String())
	})
}

func TestGetMap(t *testing.T) {
	st, err := NewTree(2)
	require.NoError(t, err)

	if st.Size() != 4 {
		t.Errorf("expected size to be 4, got %d", st.Size())
	}

	hash1, _ := chainhash.NewHashFromStr("8c14f0db3df150123e6f3dbbf30f8b955a8249b62ac1d1ff16284aefa3d06d87")
	hash2, _ := chainhash.NewHashFromStr("fff2525b8931402dd09222c50775608f75787bd2b87e56995a7bdd30f79702c4")
	hash3, _ := chainhash.NewHashFromStr("6359f0868171b1d194cbee1af2f16ea598ae8fad666d9b012c8ed2b79a236ec4")
	hash4, _ := chainhash.NewHashFromStr("e9a66845e05d5abc0ad04ec80f774a7e585c6e8db975962d069a522137b80c1d")
	_ = st.AddNode(*hash1, 111, 101)
	_ = st.AddNode(*hash2, 112, 102)
	_ = st.AddNode(*hash3, 113, 103)
	_ = st.AddNode(*hash4, 114, 104)

	txMap, err := st.GetMap()
	require.NoError(t, err)

	assert.Equal(t, 4, txMap.Length())

	for _, node := range st.Nodes {
		txIdx, ok := txMap.Get(node.Hash)
		require.True(t, ok, fmt.Sprintf("expected to find hash %s in map", node.Hash.String()))

		// find node in original tree
		originalIdx := st.NodeIndex(node.Hash)
		assert.Equal(t, uint64(originalIdx), txIdx) //nolint:gosec // Ignore for now
	}
}

func TestHasNode(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		st, err := NewTree(2)
		require.NoError(t, err)

		if st.Size() != 4 {
			t.Errorf("expected size to be 4, got %d", st.Size())
		}

		hash1, _ := chainhash.NewHashFromStr("8c14f0db3df150123e6f3dbbf30f8b955a8249b62ac1d1ff16284aefa3d06d87")
		_ = st.AddNode(*hash1, 111, 0)

		exists := st.HasNode(*hash1)
		assert.True(t, exists)
	})

	t.Run("does not exist", func(t *testing.T) {
		st, err := NewTree(2)
		require.NoError(t, err)

		if st.Size() != 4 {
			t.Errorf("expected size to be 4, got %d", st.Size())
		}

		hash1, _ := chainhash.NewHashFromStr("8c14f0db3df150123e6f3dbbf30f8b955a8249b62ac1d1ff16284aefa3d06d87")
		exists := st.HasNode(*hash1)
		assert.False(t, exists)
	})
}

func TestGetNode(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		st, err := NewTree(2)
		require.NoError(t, err)

		if st.Size() != 4 {
			t.Errorf("expected size to be 4, got %d", st.Size())
		}

		hash1, _ := chainhash.NewHashFromStr("8c14f0db3df150123e6f3dbbf30f8b955a8249b62ac1d1ff16284aefa3d06d87")
		_ = st.AddNode(*hash1, 111, 0)

		node, err := st.GetNode(*hash1)
		assert.NoError(t, err)
		assert.Equal(t, *hash1, node.Hash)
		assert.Equal(t, uint64(111), node.Fee)
	})

	t.Run("does not exist", func(t *testing.T) {
		st, err := NewTree(2)
		require.NoError(t, err)

		if st.Size() != 4 {
			t.Errorf("expected size to be 4, got %d", st.Size())
		}

		hash1, _ := chainhash.NewHashFromStr("8c14f0db3df150123e6f3dbbf30f8b955a8249b62ac1d1ff16284aefa3d06d87")
		node, err := st.GetNode(*hash1)
		assert.Error(t, err)
		assert.Nil(t, node)
	})
}

func TestDifference(t *testing.T) {
	st1, err := NewTree(2)
	require.NoError(t, err)

	hash1, _ := chainhash.NewHashFromStr("8c14f0db3df150123e6f3dbbf30f8b955a8249b62ac1d1ff16284aefa3d06d87")
	hash2, _ := chainhash.NewHashFromStr("fff2525b8931402dd09222c50775608f75787bd2b87e56995a7bdd30f79702c4")
	hash3, _ := chainhash.NewHashFromStr("6359f0868171b1d194cbee1af2f16ea598ae8fad666d9b012c8ed2b79a236ec4")
	hash4, _ := chainhash.NewHashFromStr("e9a66845e05d5abc0ad04ec80f774a7e585c6e8db975962d069a522137b80c1d")
	_ = st1.AddNode(*hash1, 111, 0)
	_ = st1.AddNode(*hash2, 112, 0)
	_ = st1.AddNode(*hash3, 113, 0)
	_ = st1.AddNode(*hash4, 114, 0)

	st2, err := NewTree(2)
	require.NoError(t, err)

	_ = st2.AddNode(*hash3, 113, 0)
	_ = st2.AddNode(*hash4, 114, 0)

	st2Map, err := st2.GetMap()
	require.NoError(t, err)

	diff, err := st1.Difference(st2Map)
	require.NoError(t, err)

	assert.Equal(t, 2, len(diff))
	assert.Equal(t, *hash1, diff[0].Hash)
	assert.Equal(t, *hash2, diff[1].Hash)
}

func TestRootHashSimon(t *testing.T) {
	st, err := NewTree(2)
	require.NoError(t, err)

	if st.Size() != 4 {
		t.Errorf("expected size to be 4, got %d", st.Size())
	}

	hash1, _ := chainhash.NewHashFromStr("8c14f0db3df150123e6f3dbbf30f8b955a8249b62ac1d1ff16284aefa3d06d87")
	hash2, _ := chainhash.NewHashFromStr("fff2525b8931402dd09222c50775608f75787bd2b87e56995a7bdd30f79702c4")
	hash3, _ := chainhash.NewHashFromStr("6359f0868171b1d194cbee1af2f16ea598ae8fad666d9b012c8ed2b79a236ec4")
	hash4, _ := chainhash.NewHashFromStr("e9a66845e05d5abc0ad04ec80f774a7e585c6e8db975962d069a522137b80c1d")
	_ = st.AddNode(*hash1, 111, 0)
	_ = st.AddNode(*hash2, 111, 0)
	_ = st.AddNode(*hash3, 111, 0)
	_ = st.AddNode(*hash4, 111, 0)

	rootHash := st.RootHash()
	assert.Equal(t, "f3e94742aca4b5ef85488dc37c06c3282295ffec960994b2c0d5ac2a25a95766", rootHash.String())
}

func TestTwoTransactions(t *testing.T) {
	st, err := NewTree(1)
	require.NoError(t, err)

	if st.Size() != 2 {
		t.Errorf("expected size to be 4, got %d", st.Size())
	}

	hash1, _ := chainhash.NewHashFromStr("de2c2e8628ab837ceff3de0217083d9d5feb71f758a5d083ada0b33a36e1b30e")
	hash2, _ := chainhash.NewHashFromStr("89878bfd69fba52876e5217faec126fc6a20b1845865d4038c12f03200793f48")
	_ = st.AddNode(*hash1, 111, 0)
	_ = st.AddNode(*hash2, 111, 0)

	rootHash := st.RootHash()
	assert.Equal(t, "7a059188283323a2ef0e02dd9f8ba1ac550f94646290d0a52a586e5426c956c5", rootHash.String())
}

func TestSubtree_GetMerkleProof(t *testing.T) {
	st, err := NewTree(3)
	require.NoError(t, err)

	if st.Size() != 8 {
		t.Errorf("expected size to be 4, got %d", st.Size())
	}

	txIDS := []string{
		"4634057867994ae379e82b408cc9eb145a6e921b95ca38f2ced7eb880685a290",
		"7f87fe1100963977975cef49344e442b4fa3dd9d41de19bc94609c100210ca05",
		"a28c1021f07263101f5a5052c6a7bdc970ac1d0ab09d8d20aa7a4a61ad9d6597",
		"dcd31c71368f757f65105d68ee1a2e5598db84900e28dabecba23651c5cda468",
		"7bac32882547cbb540914f48c6ac99ac682ef001c3aa3d4dcdb5951c8db79678",
		"67c0f4eb336057ecdf940497a75fcbd1a131e981edf568b54eed2f944889e441",
	}

	var txHash *chainhash.Hash
	for _, txID := range txIDS {
		txHash, _ = chainhash.NewHashFromStr(txID)
		_ = st.AddNode(*txHash, 101, 0)
	}

	proof, err := st.GetMerkleProof(1)
	require.NoError(t, err)
	assert.Equal(t, 3, len(proof))
	assert.Equal(t, "4634057867994ae379e82b408cc9eb145a6e921b95ca38f2ced7eb880685a290", proof[0].String())
	assert.Equal(t, "a9e6413abb02b534ff5250cbabdc673480656d0e053cfd23fd010241d5e045f2", proof[1].String())
	assert.Equal(t, "63fd0f07ff87223f688d0809f46a8118f185bab04d300406513acdc8832bad5e", proof[2].String())
	assert.Equal(t, "68e239fc6684a224142add79ebed60569baedf667c6be03a5f8719aba44a488b", st.RootHash().String())

	proof, err = st.GetMerkleProof(4)
	require.NoError(t, err)
	assert.Equal(t, 3, len(proof))
	assert.Equal(t, "67c0f4eb336057ecdf940497a75fcbd1a131e981edf568b54eed2f944889e441", proof[0].String())
	assert.Equal(t, "e2a6065233b307b77a5f73f9f27843d42e48d5e061567416b4508517ef2dd452", proof[1].String())
	assert.Equal(t, "bfd8a13a5cb1ba128319ee95e09a7e2ff67a52d0c9af8485bfffae737e32d6bf", proof[2].String())
	assert.Equal(t, "68e239fc6684a224142add79ebed60569baedf667c6be03a5f8719aba44a488b", st.RootHash().String())

	proof, err = st.GetMerkleProof(6)
	require.Error(t, err) // out of range
	assert.Len(t, proof, 0)
}

func Test_Serialize(t *testing.T) {
	t.Run("Serialize", func(t *testing.T) {
		st, err := NewTree(2)
		require.NoError(t, err)

		if st.Size() != 4 {
			t.Errorf("expected size to be 4, got %d", st.Size())
		}

		hash1, _ := chainhash.NewHashFromStr("8c14f0db3df150123e6f3dbbf30f8b955a8249b62ac1d1ff16284aefa3d06d87")
		hash2, _ := chainhash.NewHashFromStr("fff2525b8931402dd09222c50775608f75787bd2b87e56995a7bdd30f79702c4")
		hash3, _ := chainhash.NewHashFromStr("6359f0868171b1d194cbee1af2f16ea598ae8fad666d9b012c8ed2b79a236ec4")
		hash4, _ := chainhash.NewHashFromStr("e9a66845e05d5abc0ad04ec80f774a7e585c6e8db975962d069a522137b80c1d")
		_ = st.AddNode(*hash1, 111, 0)
		_ = st.AddNode(*hash2, 111, 0)
		_ = st.AddNode(*hash3, 111, 0)
		_ = st.AddNode(*hash4, 111, 0)

		serializedBytes, err := st.Serialize()
		require.NoError(t, err)

		newSubtree, err := NewTree(2)
		require.NoError(t, err)

		err = newSubtree.Deserialize(serializedBytes)
		require.NoError(t, err)
		assert.Equal(t, st.Fees, newSubtree.Fees)
		assert.Equal(t, st.Size(), newSubtree.Size())
		assert.Equal(t, st.RootHash(), newSubtree.RootHash())

		assert.Equal(t, len(st.Nodes), len(newSubtree.Nodes))

		for i := 0; i < len(st.Nodes); i++ {
			assert.Equal(t, st.Nodes[i].Hash.String(), newSubtree.Nodes[i].Hash.String())
			assert.Equal(t, st.Nodes[i].Fee, newSubtree.Nodes[i].Fee)
		}
	})

	t.Run("Serialize nodes", func(t *testing.T) {
		st, err := NewTree(2)
		require.NoError(t, err)

		if st.Size() != 4 {
			t.Errorf("expected size to be 4, got %d", st.Size())
		}

		hash1, _ := chainhash.NewHashFromStr("8c14f0db3df150123e6f3dbbf30f8b955a8249b62ac1d1ff16284aefa3d06d87")
		hash2, _ := chainhash.NewHashFromStr("fff2525b8931402dd09222c50775608f75787bd2b87e56995a7bdd30f79702c4")
		hash3, _ := chainhash.NewHashFromStr("6359f0868171b1d194cbee1af2f16ea598ae8fad666d9b012c8ed2b79a236ec4")
		hash4, _ := chainhash.NewHashFromStr("e9a66845e05d5abc0ad04ec80f774a7e585c6e8db975962d069a522137b80c1d")
		_ = st.AddNode(*hash1, 111, 0)
		_ = st.AddNode(*hash2, 111, 0)
		_ = st.AddNode(*hash3, 111, 0)
		_ = st.AddNode(*hash4, 111, 0)

		subtreeBytes, err := st.SerializeNodes()
		require.NoError(t, err)

		require.Equal(t, chainhash.HashSize*4, len(subtreeBytes))

		txHashes := make([]chainhash.Hash, len(subtreeBytes)/chainhash.HashSize)
		for i := 0; i < len(subtreeBytes); i += chainhash.HashSize {
			txHashes[i/chainhash.HashSize] = chainhash.Hash(subtreeBytes[i : i+chainhash.HashSize])
		}

		assert.Equal(t, hash1.String(), txHashes[0].String())
		assert.Equal(t, hash2.String(), txHashes[1].String())
		assert.Equal(t, hash3.String(), txHashes[2].String())
		assert.Equal(t, hash4.String(), txHashes[3].String())
	})

	t.Run("New subtree from bytes", func(t *testing.T) {
		st, serializedBytes := getSubtreeBytes(t)

		newSubtree, err := NewSubtreeFromBytes(serializedBytes)
		require.NoError(t, err)

		for i := 0; i < newSubtree.Size(); i += chainhash.HashSize {
			assert.Equal(t, st.Nodes[i/chainhash.HashSize].Hash.String(), newSubtree.Nodes[i/chainhash.HashSize].Hash.String())
		}
	})

	t.Run("New subtree from reader", func(t *testing.T) {
		st, serializedBytes := getSubtreeBytes(t)

		newSubtree, err := NewSubtreeFromReader(bytes.NewReader(serializedBytes))
		require.NoError(t, err)

		for i := 0; i < newSubtree.Size(); i += chainhash.HashSize {
			assert.Equal(t, st.Nodes[i/chainhash.HashSize].Hash.String(), newSubtree.Nodes[i/chainhash.HashSize].Hash.String())
		}
	})

	t.Run("DeserializeNodes with reader", func(t *testing.T) {
		st, serializedBytes := getSubtreeBytes(t)

		subtreeBytes, err := DeserializeNodesFromReader(bytes.NewReader(serializedBytes))
		require.NoError(t, err)

		require.Equal(t, chainhash.HashSize*4, len(subtreeBytes))

		for i := 0; i < len(subtreeBytes); i += chainhash.HashSize {
			txHash := chainhash.Hash(subtreeBytes[i : i+chainhash.HashSize])
			assert.Equal(t, st.Nodes[i/chainhash.HashSize].Hash.String(), txHash.String())
		}
	})

	t.Run("Deserialize with reader", func(t *testing.T) {
		st, serializedBytes := getSubtreeBytes(t)

		newSubtree, err := NewTree(2)
		require.NoError(t, err)

		r := bytes.NewReader(serializedBytes)

		err = newSubtree.DeserializeFromReader(r)
		require.NoError(t, err)
		assert.Equal(t, st.Fees, newSubtree.Fees)
		assert.Equal(t, st.Size(), newSubtree.Size())
		assert.Equal(t, st.RootHash(), newSubtree.RootHash())

		assert.Equal(t, len(st.Nodes), len(newSubtree.Nodes))

		for i := 0; i < len(st.Nodes); i++ {
			assert.Equal(t, st.Nodes[i].Hash.String(), newSubtree.Nodes[i].Hash.String())
			assert.Equal(t, st.Nodes[i].Fee, newSubtree.Nodes[i].Fee)
		}
	})

	t.Run("Serialize with conflicting", func(t *testing.T) {
		st, err := NewTree(2)
		require.NoError(t, err)

		if st.Size() != 4 {
			t.Errorf("expected size to be 4, got %d", st.Size())
		}

		hash1, _ := chainhash.NewHashFromStr("8c14f0db3df150123e6f3dbbf30f8b955a8249b62ac1d1ff16284aefa3d06d87")
		hash2, _ := chainhash.NewHashFromStr("fff2525b8931402dd09222c50775608f75787bd2b87e56995a7bdd30f79702c4")
		hash3, _ := chainhash.NewHashFromStr("6359f0868171b1d194cbee1af2f16ea598ae8fad666d9b012c8ed2b79a236ec4")
		hash4, _ := chainhash.NewHashFromStr("e9a66845e05d5abc0ad04ec80f774a7e585c6e8db975962d069a522137b80c1d")
		_ = st.AddNode(*hash1, 111, 0)
		_ = st.AddNode(*hash2, 111, 0)
		_ = st.AddNode(*hash3, 111, 0)
		_ = st.AddNode(*hash4, 111, 0)

		err = st.AddConflictingNode(*hash3)
		require.NoError(t, err)

		err = st.AddConflictingNode(*hash4)
		require.NoError(t, err)

		serializedBytes, err := st.Serialize()
		require.NoError(t, err)

		newSubtree, err := NewTree(2)
		require.NoError(t, err)

		err = newSubtree.Deserialize(serializedBytes)
		require.NoError(t, err)
		assert.Equal(t, st.Fees, newSubtree.Fees)
		assert.Equal(t, st.Size(), newSubtree.Size())
		assert.Equal(t, st.RootHash(), newSubtree.RootHash())

		assert.Equal(t, len(st.Nodes), len(newSubtree.Nodes))

		for i := 0; i < len(st.Nodes); i++ {
			assert.Equal(t, st.Nodes[i].Hash.String(), newSubtree.Nodes[i].Hash.String())
			assert.Equal(t, st.Nodes[i].Fee, newSubtree.Nodes[i].Fee)
		}

		assert.Equal(t, len(st.ConflictingNodes), len(newSubtree.ConflictingNodes))

		for i := 0; i < len(st.ConflictingNodes); i++ {
			assert.Equal(t, st.ConflictingNodes[i].String(), newSubtree.ConflictingNodes[i].String())
		}

		conflictingNodes, err := DeserializeSubtreeConflictingFromReader(bytes.NewReader(serializedBytes))
		require.NoError(t, err)

		assert.Equal(t, len(st.ConflictingNodes), len(conflictingNodes))

		for i := 0; i < len(st.ConflictingNodes); i++ {
			assert.Equal(t, st.ConflictingNodes[i].String(), conflictingNodes[i].String())
		}
	})
}

func Test_Duplicate(t *testing.T) {
	t.Run("Duplicate", func(t *testing.T) {
		st, err := NewTree(2)
		require.NoError(t, err)

		if st.Size() != 4 {
			t.Errorf("expected size to be 4, got %d", st.Size())
		}

		hash1, _ := chainhash.NewHashFromStr("8c14f0db3df150123e6f3dbbf30f8b955a8249b62ac1d1ff16284aefa3d06d87")
		hash2, _ := chainhash.NewHashFromStr("fff2525b8931402dd09222c50775608f75787bd2b87e56995a7bdd30f79702c4")
		hash3, _ := chainhash.NewHashFromStr("6359f0868171b1d194cbee1af2f16ea598ae8fad666d9b012c8ed2b79a236ec4")
		hash4, _ := chainhash.NewHashFromStr("e9a66845e05d5abc0ad04ec80f774a7e585c6e8db975962d069a522137b80c1d")
		_ = st.AddNode(*hash1, 111, 0)
		_ = st.AddNode(*hash2, 111, 0)
		_ = st.AddNode(*hash3, 111, 0)
		_ = st.AddNode(*hash4, 111, 0)

		newSubtree := st.Duplicate()

		require.NoError(t, err)
		assert.Equal(t, st.Fees, newSubtree.Fees)
		assert.Equal(t, st.Size(), newSubtree.Size())
		assert.Equal(t, st.RootHash(), newSubtree.RootHash())

		assert.Equal(t, len(st.Nodes), len(newSubtree.Nodes))

		for i := 0; i < len(st.Nodes); i++ {
			assert.Equal(t, st.Nodes[i].Hash.String(), newSubtree.Nodes[i].Hash.String())
			assert.Equal(t, st.Nodes[i].Fee, newSubtree.Nodes[i].Fee)
		}
	})

	t.Run("Clone - not same root hash", func(t *testing.T) {
		st, err := NewTree(2)
		require.NoError(t, err)

		if st.Size() != 4 {
			t.Errorf("expected size to be 4, got %d", st.Size())
		}

		hash1, _ := chainhash.NewHashFromStr("8c14f0db3df150123e6f3dbbf30f8b955a8249b62ac1d1ff16284aefa3d06d87")
		hash2, _ := chainhash.NewHashFromStr("fff2525b8931402dd09222c50775608f75787bd2b87e56995a7bdd30f79702c4")
		hash3, _ := chainhash.NewHashFromStr("6359f0868171b1d194cbee1af2f16ea598ae8fad666d9b012c8ed2b79a236ec4")
		hash4, _ := chainhash.NewHashFromStr("e9a66845e05d5abc0ad04ec80f774a7e585c6e8db975962d069a522137b80c1d")
		_ = st.AddNode(*hash1, 111, 0)
		_ = st.AddNode(*hash2, 111, 0)
		_ = st.AddNode(*hash3, 111, 0)
		_ = st.AddNode(*hash4, 111, 0)

		newSubtree := st.Duplicate()
		newSubtree.ReplaceRootNode(hash4, 111, 0)
		assert.NotEqual(t, st.RootHash(), newSubtree.RootHash())
	})
}

func TestSubtree_NodeIndex(t *testing.T) {
	tx1 := tx.Clone()
	tx1.Version = 1
	hash1 := *tx1.TxIDChainHash()

	tx2 := tx.Clone()
	tx2.Version = 2
	hash2 := *tx2.TxIDChainHash()

	tx3 := tx.Clone()
	tx3.Version = 3
	hash3 := *tx3.TxIDChainHash()

	t.Run("existing node", func(t *testing.T) {
		st, err := NewTree(4)
		require.NoError(t, err)

		_ = st.AddNode(hash1, 111, 1)
		_ = st.AddNode(hash2, 112, 2)

		index := st.NodeIndex(hash1)
		assert.Equal(t, 0, index)

		index = st.NodeIndex(hash2)
		assert.Equal(t, 1, index)
	})

	t.Run("non-existing node", func(t *testing.T) {
		st, err := NewTree(4)
		require.NoError(t, err)

		_ = st.AddNode(hash1, 111, 1)
		_ = st.AddNode(hash2, 112, 2)

		index := st.NodeIndex(hash3)
		assert.Equal(t, -1, index)
	})

	t.Run("remove existing node", func(t *testing.T) {
		st, err := NewTree(4)
		require.NoError(t, err)

		_ = st.AddNode(hash1, 111, 1)
		_ = st.AddNode(hash2, 112, 2)

		assert.Equal(t, 2, st.Length())

		err = st.RemoveNodeAtIndex(0)
		require.NoError(t, err)
		assert.Equal(t, 1, st.Length())

		// hash2 should now be at node 0
		assert.Equal(t, hash2, st.Nodes[0].Hash)
	})

	t.Run("remove non-existing node", func(t *testing.T) {
		st, err := NewTree(4)
		require.NoError(t, err)

		err = st.RemoveNodeAtIndex(0)
		assert.Error(t, err, "index out of range")
	})
}

func getSubtreeBytes(t *testing.T) (*Subtree, []byte) {
	st, err := NewTree(2)
	require.NoError(t, err)

	if st.Size() != 4 {
		t.Errorf("expected size to be 4, got %d", st.Size())
	}

	hash1, _ := chainhash.NewHashFromStr("8c14f0db3df150123e6f3dbbf30f8b955a8249b62ac1d1ff16284aefa3d06d87")
	hash2, _ := chainhash.NewHashFromStr("fff2525b8931402dd09222c50775608f75787bd2b87e56995a7bdd30f79702c4")
	hash3, _ := chainhash.NewHashFromStr("6359f0868171b1d194cbee1af2f16ea598ae8fad666d9b012c8ed2b79a236ec4")
	hash4, _ := chainhash.NewHashFromStr("e9a66845e05d5abc0ad04ec80f774a7e585c6e8db975962d069a522137b80c1d")
	_ = st.AddNode(*hash1, 111, 0)
	_ = st.AddNode(*hash2, 111, 0)
	_ = st.AddNode(*hash3, 111, 0)
	_ = st.AddNode(*hash4, 111, 0)

	serializedBytes, err := st.Serialize()
	require.NoError(t, err)

	return st, serializedBytes
}

func Test_BuildMerkleTreeStoreFromBytes(t *testing.T) {
	t.Run("complete tree", func(t *testing.T) {
		hashes := make([]*chainhash.Hash, 8)
		hashes[0], _ = chainhash.NewHashFromStr("97af9ad3583e2f83fc1e44e475e3a3ee31ec032449cc88b491479ef7d187c115")
		hashes[1], _ = chainhash.NewHashFromStr("7ce05dda56bc523048186c0f0474eb21c92fe35de6d014bd016834637a3ed08d")
		hashes[2], _ = chainhash.NewHashFromStr("3070fb937289e24720c827cbc24f3fce5c361cd7e174392a700a9f42051609e0")
		hashes[3], _ = chainhash.NewHashFromStr("d3cde0ab7142cc99acb31c5b5e1e941faed1c5cf5f8b63ed663972845d663487")
		hashes[4], _ = chainhash.NewHashFromStr("87af9ad3583e2f83fc1e44e475e3a3ee31ec032449cc88b491479ef7d187c115")
		hashes[5], _ = chainhash.NewHashFromStr("6ce05dda56bc523048186c0f0474eb21c92fe35de6d014bd016834637a3ed08d")
		hashes[6], _ = chainhash.NewHashFromStr("2070fb937289e24720c827cbc24f3fce5c361cd7e174392a700a9f42051609e0")
		hashes[7], _ = chainhash.NewHashFromStr("c3cde0ab7142cc99acb31c5b5e1e941faed1c5cf5f8b63ed663972845d663487")

		subtree, err := NewTreeByLeafCount(8)
		require.NoError(t, err)

		for _, hash := range hashes {
			_ = subtree.AddNode(*hash, 111, 0)
		}

		merkleStore, err := BuildMerkleTreeStoreFromBytes(subtree.Nodes)
		require.NoError(t, err)

		expectedMerkleStore := []string{
			"2207df31366e6fdd96a7ef3286278422c1c6dd3d74c3f85bbcfee82a8d31da25",
			"c32db78e5f8437648888713982ea3d49628dbde0b4b48857147f793b55d26f09",
			"4cfd8f882dc64dd7a123d545785bd2670c981493ea85ec058e6428cb95f04fa7",
			"0bb2f84f4071e1a04f61bb04a10dc17affcf7fd558945a3a31b1d1f0fb6ec121",
			"b47df6aa4fe0a1d3841c635444be4e33eb8cdc2f2e929ced06d0a8454fb28225",
			"1e3cfb94c292e8fc2ac692c4c4db4ea73784978ff47424668233a7f491e218a3",
			"86867b9f3e7dcb4bdf5b5cc99322122fe492bc466621f3709d4e389e7e14c16c",
		}

		actualMerkleStore := make([]string, len(*merkleStore))
		for idx, merkle := range *merkleStore {
			actualMerkleStore[idx] = merkle.String()
		}

		assert.Equal(t, expectedMerkleStore, actualMerkleStore)
	})

	t.Run("incomplete tree", func(t *testing.T) {
		st, err := NewTreeByLeafCount(8)
		require.NoError(t, err)

		txIDS := []string{
			"4634057867994ae379e82b408cc9eb145a6e921b95ca38f2ced7eb880685a290",
			"7f87fe1100963977975cef49344e442b4fa3dd9d41de19bc94609c100210ca05",
			"a28c1021f07263101f5a5052c6a7bdc970ac1d0ab09d8d20aa7a4a61ad9d6597",
			"dcd31c71368f757f65105d68ee1a2e5598db84900e28dabecba23651c5cda468",
			"7bac32882547cbb540914f48c6ac99ac682ef001c3aa3d4dcdb5951c8db79678",
			"67c0f4eb336057ecdf940497a75fcbd1a131e981edf568b54eed2f944889e441",
		}

		var txHash *chainhash.Hash
		for _, txID := range txIDS {
			txHash, _ = chainhash.NewHashFromStr(txID)
			_ = st.AddNode(*txHash, 101, 0)
		}

		merkleStore, err := BuildMerkleTreeStoreFromBytes(st.Nodes)
		require.NoError(t, err)

		expectedMerkleStore := []string{
			"dc9ab938cd3124ad36e90c30bcb02256eb73eb62dc657d93e89a0a29f323c3c7",
			"a9e6413abb02b534ff5250cbabdc673480656d0e053cfd23fd010241d5e045f2",
			"e2a6065233b307b77a5f73f9f27843d42e48d5e061567416b4508517ef2dd452",
			"",
			"bfd8a13a5cb1ba128319ee95e09a7e2ff67a52d0c9af8485bfffae737e32d6bf",
			"63fd0f07ff87223f688d0809f46a8118f185bab04d300406513acdc8832bad5e",
			"68e239fc6684a224142add79ebed60569baedf667c6be03a5f8719aba44a488b",
		}

		actualMerkleStore := make([]string, len(*merkleStore))

		for idx, merkle := range *merkleStore {
			if merkle.Equal(chainhash.Hash{}) {
				actualMerkleStore[idx] = ""
			} else {
				actualMerkleStore[idx] = merkle.String()
			}
		}

		assert.Equal(t, expectedMerkleStore, actualMerkleStore)
	})

	t.Run("incomplete tree 2", func(t *testing.T) {
		hashes := make([]*chainhash.Hash, 5)
		hashes[0], _ = chainhash.NewHashFromStr("97af9ad3583e2f83fc1e44e475e3a3ee31ec032449cc88b491479ef7d187c115")
		hashes[1], _ = chainhash.NewHashFromStr("7ce05dda56bc523048186c0f0474eb21c92fe35de6d014bd016834637a3ed08d")
		hashes[2], _ = chainhash.NewHashFromStr("3070fb937289e24720c827cbc24f3fce5c361cd7e174392a700a9f42051609e0")
		hashes[3], _ = chainhash.NewHashFromStr("d3cde0ab7142cc99acb31c5b5e1e941faed1c5cf5f8b63ed663972845d663487")
		hashes[4], _ = chainhash.NewHashFromStr("87af9ad3583e2f83fc1e44e475e3a3ee31ec032449cc88b491479ef7d187c115")

		subtree, err := NewTreeByLeafCount(8)
		require.NoError(t, err)

		for _, hash := range hashes {
			_ = subtree.AddNode(*hash, 111, 0)
		}

		merkleStore, err := BuildMerkleTreeStoreFromBytes(subtree.Nodes)
		require.NoError(t, err)

		expectedMerkleStore := []string{
			"2207df31366e6fdd96a7ef3286278422c1c6dd3d74c3f85bbcfee82a8d31da25",
			"c32db78e5f8437648888713982ea3d49628dbde0b4b48857147f793b55d26f09",
			"61a34fe6c63b5276e042a10a559e9ee9bb785f7b40f753fefdf0fe615d8a6be1",
			"",
			"b47df6aa4fe0a1d3841c635444be4e33eb8cdc2f2e929ced06d0a8454fb28225",
			"95d960d5691c5a92beb94501d0f775dbc161e6fe1c6ca420e158ef22f25320cb",
			"e641bf2a1c0a2298d628ad70e25976cbda419e825eeb21d854976d6f93192a24",
		}

		actualMerkleStore := make([]string, len(*merkleStore))

		for idx, merkle := range *merkleStore {
			if merkle.Equal(chainhash.Hash{}) {
				actualMerkleStore[idx] = ""
			} else {
				actualMerkleStore[idx] = merkle.String()
			}
		}

		assert.Equal(t, expectedMerkleStore, actualMerkleStore)
	})
}

// func TestSubtree_AddNode(t *testing.T) {
//	t.Run("fee hash", func(t *testing.T) {
//		st := NewTree(1)
//		assert.Equal(t, "0000000000000000000000000000000000000000000000000000000000000000", st.FeeHash.String())
//	})
//
//	t.Run("fee hash 1", func(t *testing.T) {
//		st := NewTree(1)
//		hash1, _ := chainhash.NewHashFromStr("de2c2e8628ab837ceff3de0217083d9d5feb71f758a5d083ada0b33a36e1b30e")
//		_ = st.AddNode(hash1, 111, 0)
//
//		assert.Equal(t, "66e4e66648f366400333d922e2371ad132b37054d53410b2767876089707eb43", st.FeeHash.String())
//	})
//
//	t.Run("fee hash 2", func(t *testing.T) {
//		st := NewTree(1)
//		hash1, _ := chainhash.NewHashFromStr("de2c2e8628ab837ceff3de0217083d9d5feb71f758a5d083ada0b33a36e1b30e")
//		hash2, _ := chainhash.NewHashFromStr("89878bfd69fba52876e5217faec126fc6a20b1845865d4038c12f03200793f48")
//		_ = st.AddNode(hash1, 111, 0)
//		_ = st.AddNode(hash2, 123, 0)
//
//		assert.Equal(t, "e6e65a874a12c4753485b3b42d1c378b36b02196ef2b3461da1d452d7d1434fb", st.FeeHash.String())
//	})
// }

func TestAddSubtreeNode(t *testing.T) {
	t.Run("successfully add node to empty subtree", func(t *testing.T) {
		st, err := NewTree(1) // Creates a subtree that can hold 2 nodes
		require.NoError(t, err)

		hash, _ := chainhash.NewHashFromStr("97af9ad3583e2f83fc1e44e475e3a3ee31ec032449cc88b491479ef7d187c115")
		node := SubtreeNode{
			Hash:        *hash,
			Fee:         1000,
			SizeInBytes: 250,
		}

		err = st.AddSubtreeNode(node)
		require.NoError(t, err)

		require.Equal(t, 1, len(st.Nodes))
		require.Equal(t, *hash, st.Nodes[0].Hash)
		require.Equal(t, uint64(1000), st.Fees)
		require.Equal(t, uint64(250), st.SizeInBytes)
		require.Nil(t, st.rootHash) // Should be reset
	})

	t.Run("successfully add multiple nodes", func(t *testing.T) {
		st, err := NewTree(1) // Creates a subtree that can hold 2 nodes
		require.NoError(t, err)

		hash1, _ := chainhash.NewHashFromStr("97af9ad3583e2f83fc1e44e475e3a3ee31ec032449cc88b491479ef7d187c115")
		node1 := SubtreeNode{
			Hash:        *hash1,
			Fee:         1000,
			SizeInBytes: 250,
		}

		hash2, _ := chainhash.NewHashFromStr("7ce05dda56bc523048186c0f0474eb21c92fe35de6d014bd016834637a3ed08d")
		node2 := SubtreeNode{
			Hash:        *hash2,
			Fee:         2000,
			SizeInBytes: 500,
		}

		err = st.AddSubtreeNode(node1)
		require.NoError(t, err)
		err = st.AddSubtreeNode(node2)
		require.NoError(t, err)

		require.Equal(t, 2, len(st.Nodes))
		require.Equal(t, *hash1, st.Nodes[0].Hash)
		require.Equal(t, *hash2, st.Nodes[1].Hash)
		require.Equal(t, uint64(3000), st.Fees)
		require.Equal(t, uint64(750), st.SizeInBytes)
	})

	t.Run("error when subtree is full", func(t *testing.T) {
		st, err := NewTree(1) // Creates a subtree that can hold 2 nodes
		require.NoError(t, err)

		// Add two nodes to fill the subtree
		hash1, _ := chainhash.NewHashFromStr("97af9ad3583e2f83fc1e44e475e3a3ee31ec032449cc88b491479ef7d187c115")
		node1 := SubtreeNode{Hash: *hash1, Fee: 1000, SizeInBytes: 250}
		hash2, _ := chainhash.NewHashFromStr("7ce05dda56bc523048186c0f0474eb21c92fe35de6d014bd016834637a3ed08d")
		node2 := SubtreeNode{Hash: *hash2, Fee: 2000, SizeInBytes: 500}

		err = st.AddSubtreeNode(node1)
		require.NoError(t, err)
		err = st.AddSubtreeNode(node2)
		require.NoError(t, err)

		require.True(t, st.IsComplete())

		// Try to add a third node
		hash3, _ := chainhash.NewHashFromStr("3070fb937289e24720c827cbc24f3fce5c361cd7e174392a700a9f42051609e0")
		node3 := SubtreeNode{Hash: *hash3, Fee: 3000, SizeInBytes: 750}
		err = st.AddSubtreeNode(node3)
		require.Error(t, err)
		require.Equal(t, "subtree is full", err.Error())
		require.Equal(t, 2, len(st.Nodes))
	})

	t.Run("error when adding coinbase placeholder", func(t *testing.T) {
		st, err := NewTree(1)
		require.NoError(t, err)

		node := SubtreeNode{
			Hash:        CoinbasePlaceholder,
			Fee:         0,
			SizeInBytes: 0,
		}

		err = st.AddSubtreeNode(node)
		require.Error(t, err)
		require.Contains(t, err.Error(), "coinbase placeholder node should be added with AddCoinbaseNode")
		require.Equal(t, 0, len(st.Nodes))
	})

	t.Run("node index is updated when it exists", func(t *testing.T) {
		st, err := NewTree(1)
		require.NoError(t, err)

		// Initialize node index
		st.nodeIndex = make(map[chainhash.Hash]int)

		hash, _ := chainhash.NewHashFromStr("97af9ad3583e2f83fc1e44e475e3a3ee31ec032449cc88b491479ef7d187c115")
		node := SubtreeNode{
			Hash:        *hash,
			Fee:         1000,
			SizeInBytes: 250,
		}

		err = st.AddSubtreeNode(node)
		require.NoError(t, err)

		// Check that the node was added to the index
		index, exists := st.nodeIndex[*hash]
		require.True(t, exists)
		require.Equal(t, 0, index)
	})
}

func TestSubtree_ConflictingNodes(t *testing.T) {
	tx1 := tx.Clone()
	tx1.Version = 1
	hash1 := *tx1.TxIDChainHash()

	tx2 := tx.Clone()
	tx2.Version = 2
	hash2 := *tx2.TxIDChainHash()

	st, err := NewTree(4)
	require.NoError(t, err)

	err = st.AddNode(hash1, 111, 1)
	require.NoError(t, err)

	err = st.AddNode(hash2, 112, 2)
	require.NoError(t, err)

	assert.Len(t, st.Nodes, 2)
	assert.Equal(t, 2, st.Length())

	err = st.AddConflictingNode(hash1)
	require.NoError(t, err)
	assert.Len(t, st.Nodes, 2)
	assert.Equal(t, 2, st.Length())
	assert.Len(t, st.ConflictingNodes, 1)

	bytes, err := st.Serialize()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(bytes), 48)

	newSt, err := NewSubtreeFromBytes(bytes)
	require.NoError(t, err)
	assert.Len(t, newSt.Nodes, 2)
	assert.Equal(t, 2, newSt.Length())
	assert.Len(t, newSt.ConflictingNodes, 1)
}

func BenchmarkSubtree_Deserialize(b *testing.B) {
	// populate subtree for test
	subtree, _ := NewTreeByLeafCount(1024 * 1024)

	for i := uint64(0); i < 1024*1024; i++ {
		hash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%x", i))
		_ = subtree.AddNode(*hash, i, i)
	}

	subtreeBytes, _ := subtree.Serialize()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := NewSubtreeFromBytes(subtreeBytes)
		require.NoError(b, err)
	}
}

func BenchmarkSubtree_DeserializeNodesFromReader(b *testing.B) {
	// populate subtree for test
	subtree, _ := NewTreeByLeafCount(1024 * 1024)

	for i := uint64(0); i < 1024*1024; i++ {
		hash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%x", i))
		_ = subtree.AddNode(*hash, i, i)
	}

	subtreeBytes, _ := subtree.Serialize()
	subtreeReader := bytes.NewReader(subtreeBytes)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := DeserializeNodesFromReader(subtreeReader)
		require.NoError(b, err)

		// reset the subtree reader for the next loop
		_, _ = subtreeReader.Seek(0, 0)
	}
}

func BenchmarkSubtree_DeserializeFromReader(b *testing.B) {
	// populate subtree for test
	subtree, _ := NewTreeByLeafCount(1024 * 1024)

	for i := uint64(0); i < 1024*1024; i++ {
		hash, _ := chainhash.NewHashFromStr(fmt.Sprintf("%x", i))
		_ = subtree.AddNode(*hash, i, i)
	}

	subtreeBytes, _ := subtree.Serialize()
	subtreeReader := bytes.NewReader(subtreeBytes)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		s := &Subtree{}
		err := s.DeserializeFromReader(subtreeReader)
		require.NoError(b, err)

		// reset the subtree reader for the next loop
		_, _ = subtreeReader.Seek(0, 0)
	}
}

func Benchmark_SubtreeNodeIndex(b *testing.B) {
	// populate subtree for test
	subtree, _ := NewTreeByLeafCount(1024 * 1024)

	for i := uint64(0); i < 1024*1024; i++ {
		hash := chainhash.HashH([]byte(fmt.Sprintf("tx_%x", i)))
		_ = subtree.AddNode(hash, i, i)
	}

	subtreeLength := subtree.Length()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		index := subtree.NodeIndex(subtree.Nodes[i%subtreeLength].Hash)
		require.GreaterOrEqual(b, index, 0)
	}
}

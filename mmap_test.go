package subtree

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/bsv-blockchain/go-bt/v2/chainhash"
	"github.com/stretchr/testify/require"
)

func TestMmapSubtree_AddNodesAndRootHash(t *testing.T) {
	dir := t.TempDir()

	// Create mmap-backed subtree
	mmapTree, err := NewTreeMmap(2, dir) // height 2 = 4 capacity
	require.NoError(t, err)
	defer mmapTree.Close()

	// Create heap-backed subtree for comparison
	heapTree, err := NewTree(2)
	require.NoError(t, err)

	require.True(t, mmapTree.IsMmapBacked())
	require.False(t, heapTree.IsMmapBacked())

	// Add same nodes to both
	nodes := []Node{
		{Hash: chainhash.HashH([]byte("tx1")), Fee: 100, SizeInBytes: 250},
		{Hash: chainhash.HashH([]byte("tx2")), Fee: 200, SizeInBytes: 350},
		{Hash: chainhash.HashH([]byte("tx3")), Fee: 150, SizeInBytes: 300},
	}

	for _, n := range nodes {
		require.NoError(t, mmapTree.AddSubtreeNodeWithoutLock(n))
		require.NoError(t, heapTree.AddSubtreeNodeWithoutLock(n))
	}

	// Verify lengths match
	require.Equal(t, heapTree.Length(), mmapTree.Length())

	// Verify root hashes match
	heapRoot := heapTree.RootHash()
	mmapRoot := mmapTree.RootHash()
	require.NotNil(t, heapRoot)
	require.NotNil(t, mmapRoot)
	require.True(t, heapRoot.IsEqual(mmapRoot), "root hashes should match: heap=%s, mmap=%s", heapRoot, mmapRoot)

	// Verify fees and size
	require.Equal(t, heapTree.Fees, mmapTree.Fees)
	require.Equal(t, heapTree.SizeInBytes, mmapTree.SizeInBytes)
}

func TestMmapSubtree_RemoveNode(t *testing.T) {
	dir := t.TempDir()

	tree, err := NewTreeMmap(2, dir)
	require.NoError(t, err)
	defer tree.Close()

	nodes := []Node{
		{Hash: chainhash.HashH([]byte("tx1")), Fee: 100, SizeInBytes: 250},
		{Hash: chainhash.HashH([]byte("tx2")), Fee: 200, SizeInBytes: 350},
		{Hash: chainhash.HashH([]byte("tx3")), Fee: 150, SizeInBytes: 300},
	}

	for _, n := range nodes {
		require.NoError(t, tree.AddSubtreeNodeWithoutLock(n))
	}

	require.Equal(t, 3, tree.Length())

	// Remove middle node
	require.NoError(t, tree.RemoveNodeAtIndex(1))

	require.Equal(t, 2, tree.Length())
	require.Equal(t, uint64(250), tree.Fees) // 100 + 150, tx2 (200) removed
}

func TestMmapSubtree_Duplicate(t *testing.T) {
	dir := t.TempDir()

	tree, err := NewTreeMmap(2, dir)
	require.NoError(t, err)
	defer tree.Close()

	require.NoError(t, tree.AddSubtreeNodeWithoutLock(Node{
		Hash: chainhash.HashH([]byte("tx1")), Fee: 100, SizeInBytes: 250,
	}))

	// Duplicate creates a heap-backed copy
	dup := tree.Duplicate()
	require.False(t, dup.IsMmapBacked(), "duplicate should be heap-backed")
	require.Equal(t, tree.Length(), dup.Length())
	require.True(t, tree.RootHash().IsEqual(dup.RootHash()))
}

func TestMmapSubtree_Serialize(t *testing.T) {
	dir := t.TempDir()

	// Create and populate mmap subtree
	mmapTree, err := NewTreeMmap(2, dir)
	require.NoError(t, err)
	defer mmapTree.Close()

	heapTree, err := NewTree(2)
	require.NoError(t, err)

	nodes := []Node{
		{Hash: chainhash.HashH([]byte("tx1")), Fee: 100, SizeInBytes: 250},
		{Hash: chainhash.HashH([]byte("tx2")), Fee: 200, SizeInBytes: 350},
	}

	for _, n := range nodes {
		require.NoError(t, mmapTree.AddSubtreeNodeWithoutLock(n))
		require.NoError(t, heapTree.AddSubtreeNodeWithoutLock(n))
	}

	// Serialize both and compare
	mmapBytes, err := mmapTree.Serialize()
	require.NoError(t, err)

	heapBytes, err := heapTree.Serialize()
	require.NoError(t, err)

	require.Equal(t, heapBytes, mmapBytes, "serialization should produce identical output")
}

func TestMmapSubtree_CloseCleanup(t *testing.T) {
	dir := t.TempDir()

	tree, err := NewTreeMmap(2, dir)
	require.NoError(t, err)

	require.NoError(t, tree.AddSubtreeNodeWithoutLock(Node{
		Hash: chainhash.HashH([]byte("tx1")), Fee: 100, SizeInBytes: 250,
	}))

	// Verify temp file exists
	files, err := filepath.Glob(filepath.Join(dir, "subtree-nodes-*"))
	require.NoError(t, err)
	require.Len(t, files, 1)

	// Close should remove the file
	require.NoError(t, tree.Close())

	files, err = filepath.Glob(filepath.Join(dir, "subtree-nodes-*"))
	require.NoError(t, err)
	require.Len(t, files, 0)

	// Double close should be safe
	require.NoError(t, tree.Close())
}

func TestMmapSubtree_HeapCloseIsNoop(t *testing.T) {
	tree, err := NewTree(2)
	require.NoError(t, err)

	// Close on heap-backed subtree is a no-op
	require.NoError(t, tree.Close())
	require.False(t, tree.IsMmapBacked())
}

func TestMmapSubtree_NilCloseIsSafe(t *testing.T) {
	var tree *Subtree
	require.NoError(t, tree.Close())
	require.False(t, tree.IsMmapBacked())
}

func TestMmapSubtree_ByLeafCount(t *testing.T) {
	dir := t.TempDir()

	tree, err := NewTreeByLeafCountMmap(1024, dir)
	require.NoError(t, err)
	defer tree.Close()

	require.True(t, tree.IsMmapBacked())
	require.Equal(t, 1024, tree.Size())
}

func TestMmapSubtree_NodeIndex(t *testing.T) {
	dir := t.TempDir()

	tree, err := NewTreeMmap(2, dir)
	require.NoError(t, err)
	defer tree.Close()

	hash := chainhash.HashH([]byte("tx1"))
	require.NoError(t, tree.AddSubtreeNodeWithoutLock(Node{
		Hash: hash, Fee: 100, SizeInBytes: 250,
	}))

	// NodeIndex should work with mmap-backed nodes
	idx := tree.NodeIndex(hash)
	require.Equal(t, 0, idx)

	// Non-existent hash
	idx = tree.NodeIndex(chainhash.HashH([]byte("nonexistent")))
	require.Equal(t, -1, idx)
}

func TestMmapSubtree_DeserializeFromReaderMmap(t *testing.T) {
	dir := t.TempDir()

	// Create and serialize a heap subtree
	original, err := NewTree(2)
	require.NoError(t, err)

	nodes := []Node{
		{Hash: chainhash.HashH([]byte("tx1")), Fee: 100, SizeInBytes: 250},
		{Hash: chainhash.HashH([]byte("tx2")), Fee: 200, SizeInBytes: 350},
	}

	for _, n := range nodes {
		require.NoError(t, original.AddSubtreeNodeWithoutLock(n))
	}

	serialized, err := original.Serialize()
	require.NoError(t, err)

	// Deserialize into mmap-backed subtree
	mmapTree, err := NewSubtreeFromReaderMmap(bytes.NewReader(serialized), dir)
	require.NoError(t, err)
	defer mmapTree.Close()

	require.True(t, mmapTree.IsMmapBacked())
	require.Equal(t, original.Length(), mmapTree.Length())
	require.True(t, original.RootHash().IsEqual(mmapTree.RootHash()))
	require.Equal(t, original.Fees, mmapTree.Fees)
	require.Equal(t, original.SizeInBytes, mmapTree.SizeInBytes)

	// Re-serialize and compare
	reSerialized, err := mmapTree.Serialize()
	require.NoError(t, err)
	require.Equal(t, serialized, reSerialized)
}

func TestMmapSubtree_FallbackOnError(t *testing.T) {
	// Try to create mmap in non-existent directory — should fail
	_, err := NewTreeMmap(2, "/nonexistent/path/that/does/not/exist")
	require.Error(t, err)
}

func TestMmapSubtree_LargeCapacity(t *testing.T) {
	dir := t.TempDir()

	// Test with a larger subtree (64K nodes capacity)
	tree, err := NewTreeByLeafCountMmap(65536, dir)
	require.NoError(t, err)
	defer tree.Close()

	require.True(t, tree.IsMmapBacked())
	require.Equal(t, 65536, tree.Size())

	// Add a few nodes
	for i := 0; i < 100; i++ {
		data := make([]byte, 8)
		data[0] = byte(i)
		data[1] = byte(i >> 8)
		require.NoError(t, tree.AddSubtreeNodeWithoutLock(Node{
			Hash: chainhash.HashH(data), Fee: uint64(i * 10), SizeInBytes: uint64(i * 100),
		}))
	}

	require.Equal(t, 100, tree.Length())
	require.NotNil(t, tree.RootHash())

	// Verify temp file size
	files, _ := filepath.Glob(filepath.Join(dir, "subtree-nodes-*"))
	require.Len(t, files, 1)

	info, err := os.Stat(files[0])
	require.NoError(t, err)
	require.Equal(t, int64(65536*nodeSize), info.Size())
}

func TestTxInpoints_SubtreeIndex(t *testing.T) {
	inpoints := NewTxInpoints()
	require.Equal(t, int16(-1), inpoints.SubtreeIndex, "default SubtreeIndex should be -1")

	inpoints.SubtreeIndex = 42
	require.Equal(t, int16(42), inpoints.SubtreeIndex)
}

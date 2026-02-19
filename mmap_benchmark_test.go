package subtree

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/bsv-blockchain/go-bt/v2/chainhash"
	"github.com/stretchr/testify/require"
)

// TestLargeScaleMmap verifies memory behavior with 100+ mmap-backed subtrees.
// Run with: go test -v -run TestLargeScaleMmap -memprofile mem.prof
func TestLargeScaleMmap(t *testing.T) {
	dir := t.TempDir()

	const (
		numSubtrees    = 128
		nodesPerSubtree = 65536 // 64K nodes per subtree
	)

	// Force GC and capture baseline heap
	runtime.GC()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)

	// --- Heap-based subtrees ---
	heapTrees := make([]*Subtree, numSubtrees)
	for i := 0; i < numSubtrees; i++ {
		tree, err := NewTreeByLeafCount(nodesPerSubtree)
		require.NoError(t, err)
		for j := 0; j < 100; j++ { // add 100 nodes each
			data := []byte(fmt.Sprintf("heap-%d-%d", i, j))
			require.NoError(t, tree.AddSubtreeNodeWithoutLock(Node{
				Hash: chainhash.HashH(data), Fee: uint64(j), SizeInBytes: 250,
			}))
		}
		heapTrees[i] = tree
	}

	runtime.GC()
	var heapStats runtime.MemStats
	runtime.ReadMemStats(&heapStats)
	heapAlloc := heapStats.HeapAlloc - baseline.HeapAlloc

	// Clear heap trees
	for _, tree := range heapTrees {
		tree.Close()
	}
	heapTrees = nil
	runtime.GC()

	// --- mmap-backed subtrees ---
	var mmapBaseline runtime.MemStats
	runtime.ReadMemStats(&mmapBaseline)

	mmapTrees := make([]*Subtree, numSubtrees)
	for i := 0; i < numSubtrees; i++ {
		tree, err := NewTreeByLeafCountMmap(nodesPerSubtree, dir)
		require.NoError(t, err)
		for j := 0; j < 100; j++ {
			data := []byte(fmt.Sprintf("mmap-%d-%d", i, j))
			require.NoError(t, tree.AddSubtreeNodeWithoutLock(Node{
				Hash: chainhash.HashH(data), Fee: uint64(j), SizeInBytes: 250,
			}))
		}
		mmapTrees[i] = tree
	}

	runtime.GC()
	var mmapStats runtime.MemStats
	runtime.ReadMemStats(&mmapStats)
	mmapAlloc := mmapStats.HeapAlloc - mmapBaseline.HeapAlloc

	// Verify root hashes work on mmap trees
	for _, tree := range mmapTrees {
		require.NotNil(t, tree.RootHash())
	}

	// Cleanup
	for _, tree := range mmapTrees {
		require.NoError(t, tree.Close())
	}

	t.Logf("=== Memory Comparison (%d subtrees × %d capacity) ===", numSubtrees, nodesPerSubtree)
	t.Logf("Heap-backed:  %d MB heap allocated", heapAlloc/(1024*1024))
	t.Logf("Mmap-backed:  %d MB heap allocated", mmapAlloc/(1024*1024))
	if heapAlloc > 0 {
		t.Logf("Reduction:    %.1f%%", float64(heapAlloc-mmapAlloc)/float64(heapAlloc)*100)
	}
}

func BenchmarkSubtreeAddNode_Heap(b *testing.B) {
	tree, _ := NewTreeByLeafCount(1 << 20) // 1M capacity
	node := Node{Hash: chainhash.HashH([]byte("bench")), Fee: 100, SizeInBytes: 250}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(tree.Nodes) >= cap(tree.Nodes) {
			tree.Nodes = tree.Nodes[:0] // reset
		}
		_ = tree.AddSubtreeNodeWithoutLock(node)
	}
}

func BenchmarkSubtreeAddNode_Mmap(b *testing.B) {
	dir := b.TempDir()
	tree, err := NewTreeByLeafCountMmap(1<<20, dir)
	if err != nil {
		b.Fatal(err)
	}
	defer tree.Close()

	node := Node{Hash: chainhash.HashH([]byte("bench")), Fee: 100, SizeInBytes: 250}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if len(tree.Nodes) >= cap(tree.Nodes) {
			tree.Nodes = tree.Nodes[:0] // reset
		}
		_ = tree.AddSubtreeNodeWithoutLock(node)
	}
}

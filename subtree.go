package subtree

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"sync"

	"github.com/bsv-blockchain/go-bt/v2/chainhash"
	safe "github.com/bsv-blockchain/go-safe-conversion"
	txmap "github.com/bsv-blockchain/go-tx-map"
)

// Node represents a node in the subtree.
type Node struct {
	Hash        chainhash.Hash `json:"txid"` // This is called txid so that the UI knows to add a link to /tx/<txid>
	Fee         uint64         `json:"fee"`
	SizeInBytes uint64         `json:"size"`
}

// Subtree represents a subtree in a Merkle tree structure.
type Subtree struct {
	Height           int
	Fees             uint64
	SizeInBytes      uint64
	FeeHash          chainhash.Hash
	Nodes            []Node
	ConflictingNodes []chainhash.Hash // conflicting nodes need to be checked when doing block assembly

	// temporary (calculated) variables
	rootHash *chainhash.Hash
	treeSize int

	// feeBytes []byte // unused, but kept for reference

	// feeHashBytes []byte // unused, but kept for reference

	mu        sync.RWMutex           // protects Nodes slice
	nodeIndex map[chainhash.Hash]int // maps txid to index in Nodes slice
}

// TxMap is an interface for a map of transaction hashes to values.
type TxMap interface {
	Put(hash chainhash.Hash, value uint64) error
	Get(hash chainhash.Hash) (uint64, bool)
	Exists(hash chainhash.Hash) bool
	Length() int
	Keys() []chainhash.Hash
}

// NewTree creates a new Subtree with a fixed height
//
//	is the number if levels in a merkle tree of the subtree
func NewTree(height int) (*Subtree, error) {
	if height < 0 {
		return nil, ErrHeightNegative
	}

	treeSize := int(math.Pow(2, float64(height)))

	return &Subtree{
		Nodes:    make([]Node, 0, treeSize),
		Height:   height,
		FeeHash:  chainhash.Hash{},
		treeSize: treeSize,
		// feeBytes:     make([]byte, 8),
		// feeHashBytes: make([]byte, 40),
	}, nil
}

// NewTreeByLeafCount creates a new Subtree with a height calculated from the maximum number of leaves.
func NewTreeByLeafCount(maxNumberOfLeaves int) (*Subtree, error) {
	if !IsPowerOfTwo(maxNumberOfLeaves) {
		return nil, ErrNotPowerOfTwo
	}

	height := math.Ceil(math.Log2(float64(maxNumberOfLeaves)))

	return NewTree(int(height))
}

// NewIncompleteTreeByLeafCount creates a new Subtree with a height calculated from the maximum number of leaves.
func NewIncompleteTreeByLeafCount(maxNumberOfLeaves int) (*Subtree, error) {
	height := math.Ceil(math.Log2(float64(maxNumberOfLeaves)))

	return NewTree(int(height))
}

// NewSubtreeFromBytes creates a new Subtree from the provided byte slice.
func NewSubtreeFromBytes(b []byte) (*Subtree, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered in NewSubtreeFromBytes: %v\n", r)
		}
	}()

	subtree := &Subtree{}

	err := subtree.Deserialize(b)
	if err != nil {
		return nil, err
	}

	return subtree, nil
}

// NewSubtreeFromReader creates a new Subtree from the provided reader.
func NewSubtreeFromReader(reader io.Reader) (*Subtree, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered in NewSubtreeFromReader: %v\n", r)
		}
	}()

	subtree := &Subtree{}

	if err := subtree.DeserializeFromReader(reader); err != nil {
		return nil, err
	}

	return subtree, nil
}

// DeserializeNodesFromReader deserializes the nodes from the provided reader.
func DeserializeNodesFromReader(reader io.Reader) (subtreeBytes []byte, err error) {
	buf := bufio.NewReaderSize(reader, 32*1024) // 32KB buffer

	// root len(st.rootHash[:]) bytes
	// first 8 bytes, fees
	// second 8 bytes, sizeInBytes
	// third 8 bytes, number of leaves
	// total read at once = len(st.rootHash[:]) + 8 + 8 + 8
	byteBuffer := make([]byte, chainhash.HashSize+24)
	if _, err = io.ReadFull(buf, byteBuffer); err != nil {
		return nil, fmt.Errorf("unable to read subtree root information: %w", err)
	}

	numLeaves := binary.LittleEndian.Uint64(byteBuffer[chainhash.HashSize+16 : chainhash.HashSize+24])
	subtreeBytes = make([]byte, chainhash.HashSize*int(numLeaves)) //nolint:gosec // G115: integer overflow conversion

	byteBuffer = byteBuffer[8:] // reduce read byteBuffer size by 8
	for i := uint64(0); i < numLeaves; i++ {
		if _, err = io.ReadFull(buf, byteBuffer); err != nil {
			return nil, fmt.Errorf("unable to read subtree node information: %w", err)
		}

		copy(subtreeBytes[i*chainhash.HashSize:(i+1)*chainhash.HashSize], byteBuffer[:chainhash.HashSize])
	}

	return subtreeBytes, nil
}

// Duplicate creates a deep copy of the Subtree.
func (st *Subtree) Duplicate() *Subtree {
	newSubtree := &Subtree{
		Height:           st.Height,
		Fees:             st.Fees,
		SizeInBytes:      st.SizeInBytes,
		FeeHash:          st.FeeHash,
		Nodes:            make([]Node, len(st.Nodes)),
		ConflictingNodes: make([]chainhash.Hash, len(st.ConflictingNodes)),
		rootHash:         st.rootHash,
		treeSize:         st.treeSize,
		// feeBytes:         make([]byte, 8),
		// feeHashBytes:     make([]byte, 40),
	}

	copy(newSubtree.Nodes, st.Nodes)
	copy(newSubtree.ConflictingNodes, st.ConflictingNodes)

	return newSubtree
}

// Size returns the capacity of the subtree
func (st *Subtree) Size() int {
	st.mu.RLock()
	size := cap(st.Nodes)
	st.mu.RUnlock()

	return size
}

// Length returns the number of nodes in the subtree
func (st *Subtree) Length() int {
	st.mu.RLock()
	length := len(st.Nodes)
	st.mu.RUnlock()

	return length
}

// IsComplete checks if the subtree is complete, meaning it has the maximum number of nodes as defined by its height.
func (st *Subtree) IsComplete() bool {
	st.mu.RLock()
	isComplete := len(st.Nodes) == cap(st.Nodes)
	st.mu.RUnlock()

	return isComplete
}

// ReplaceRootNode replaces the root node of the subtree with the given node and returns the new root hash.
func (st *Subtree) ReplaceRootNode(node *chainhash.Hash, fee, sizeInBytes uint64) *chainhash.Hash {
	if len(st.Nodes) < 1 {
		st.Nodes = append(st.Nodes, Node{
			Hash:        *node,
			Fee:         fee,
			SizeInBytes: sizeInBytes,
		})
	} else {
		st.Nodes[0] = Node{
			Hash:        *node,
			Fee:         fee,
			SizeInBytes: sizeInBytes,
		}
	}

	st.rootHash = nil // reset rootHash
	st.SizeInBytes += sizeInBytes

	return st.RootHash()
}

// AddSubtreeNode adds a Node to the subtree.
func (st *Subtree) AddSubtreeNode(node Node) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	if (len(st.Nodes) + 1) > st.treeSize {
		return ErrSubtreeFull
	}

	if node.Hash.Equal(CoinbasePlaceholder) {
		return fmt.Errorf("[AddSubtreeNode] %w, tree length is %d", ErrCoinbasePlaceholderMisuse, len(st.Nodes))
	}

	st.Nodes = append(st.Nodes, node)
	st.rootHash = nil // reset rootHash
	st.Fees += node.Fee
	st.SizeInBytes += node.SizeInBytes

	if st.nodeIndex != nil {
		// node index map exists, add the node to it
		st.nodeIndex[node.Hash] = len(st.Nodes) - 1
	}

	return nil
}

// AddSubtreeNodeWithoutLock adds a Node to the subtree without locking.
func (st *Subtree) AddSubtreeNodeWithoutLock(node Node) error {
	if (len(st.Nodes) + 1) > st.treeSize {
		return ErrSubtreeFull
	}

	st.Nodes = append(st.Nodes, node)
	st.rootHash = nil // reset rootHash
	st.Fees += node.Fee
	st.SizeInBytes += node.SizeInBytes

	if st.nodeIndex != nil {
		// node index map exists, add the node to it
		st.nodeIndex[node.Hash] = len(st.Nodes) - 1
	}

	return nil
}

// AddCoinbaseNode adds a coinbase node to the subtree.
func (st *Subtree) AddCoinbaseNode() error {
	if len(st.Nodes) != 0 {
		return ErrSubtreeNotEmpty
	}

	st.Nodes = append(st.Nodes, Node{
		Hash:        CoinbasePlaceholder,
		Fee:         0,
		SizeInBytes: 0,
	})
	st.rootHash = nil // reset rootHash
	st.Fees = 0
	st.SizeInBytes = 0

	return nil
}

// AddConflictingNode adds a conflicting node to the subtree.
func (st *Subtree) AddConflictingNode(newConflictingNode chainhash.Hash) error {
	if st.ConflictingNodes == nil {
		st.ConflictingNodes = make([]chainhash.Hash, 0, 1)
	}

	// check the conflicting node is actually in the subtree
	found := false

	for _, n := range st.Nodes {
		if n.Hash.Equal(newConflictingNode) {
			found = true
			break
		}
	}

	if !found {
		return ErrConflictingNodeNotInSubtree
	}

	// check whether the conflicting node has already been added
	for _, conflictingNode := range st.ConflictingNodes {
		if conflictingNode.Equal(newConflictingNode) {
			return nil
		}
	}

	st.ConflictingNodes = append(st.ConflictingNodes, newConflictingNode)

	return nil
}

// AddNode adds a node to the subtree
// WARNING: this function is not concurrency safe, so it should be called from a single goroutine
//
// Parameters:
//   - node: the transaction id of the node to add
//   - fee: the fee of the node
//   - sizeInBytes: the size of the node in bytes
//
// Returns:
//   - error: an error if the node could not be added
func (st *Subtree) AddNode(node chainhash.Hash, fee, sizeInBytes uint64) error {
	if (len(st.Nodes) + 1) > st.treeSize {
		return ErrSubtreeFull
	}

	if node.Equal(CoinbasePlaceholder) {
		return fmt.Errorf("[AddNode] %w", ErrCoinbasePlaceholderMisuse)
	}

	// AddNode is not concurrency safe, so we can reuse the same byte arrays
	// binary.LittleEndian.PutUint64(st.feeBytes, fee)
	// st.feeHashBytes = append(node[:], st.feeBytes[:]...)
	// if len(st.Nodes) == 0 {
	//	st.FeeHash = chainhash.HashH(st.feeHashBytes)
	// } else {
	//	st.FeeHash = chainhash.HashH(append(st.FeeHash[:], st.feeHashBytes...))
	// }

	st.Nodes = append(st.Nodes, Node{
		Hash:        node,
		Fee:         fee,
		SizeInBytes: sizeInBytes,
	})
	st.rootHash = nil // reset rootHash
	st.Fees += fee
	st.SizeInBytes += sizeInBytes

	if st.nodeIndex != nil {
		// node index map exists, add the node to it
		st.nodeIndex[node] = len(st.Nodes) - 1
	}

	return nil
}

// RemoveNodeAtIndex removes a node at the given index and makes sure the subtree is still valid
func (st *Subtree) RemoveNodeAtIndex(index int) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	if index >= len(st.Nodes) {
		return ErrIndexOutOfRange
	}

	st.Fees -= st.Nodes[index].Fee
	st.SizeInBytes -= st.Nodes[index].SizeInBytes

	hash := st.Nodes[index].Hash
	st.Nodes = append(st.Nodes[:index], st.Nodes[index+1:]...)
	st.rootHash = nil // reset rootHash

	if st.nodeIndex != nil {
		// remove the node from the node index map
		delete(st.nodeIndex, hash)
	}

	return nil
}

// RootHash calculates and returns the root hash of the subtree.
func (st *Subtree) RootHash() *chainhash.Hash {
	if st == nil {
		return nil
	}

	if st.rootHash != nil {
		return st.rootHash
	}

	if st.Length() == 0 {
		return nil
	}

	// calculate rootHash
	store, err := BuildMerkleTreeStoreFromBytes(st.Nodes)
	if err != nil {
		return nil
	}

	st.rootHash, _ = chainhash.NewHash((*store)[len(*store)-1][:])

	return st.rootHash
}

// RootHashWithReplaceRootNode replaces the root node of the subtree with the given node and returns the new root hash.
func (st *Subtree) RootHashWithReplaceRootNode(node *chainhash.Hash, fee, sizeInBytes uint64) (*chainhash.Hash, error) {
	if st == nil {
		return nil, ErrSubtreeNil
	}

	// clone the subtree, so we do not overwrite anything in it
	subtreeClone := st.Duplicate()
	subtreeClone.ReplaceRootNode(node, fee, sizeInBytes)

	// calculate rootHash
	store, err := BuildMerkleTreeStoreFromBytes(subtreeClone.Nodes)
	if err != nil {
		return nil, err
	}

	rootHash := chainhash.Hash((*store)[len(*store)-1][:])

	return &rootHash, nil
}

// GetMap returns a TxMap representation of the subtree, mapping transaction hashes to their indices.
func (st *Subtree) GetMap() (TxMap, error) {
	lengthUint32, err := safe.IntToUint32(len(st.Nodes))
	if err != nil {
		return nil, err
	}

	m := txmap.NewSwissMapUint64(lengthUint32)
	for idx, node := range st.Nodes {
		_ = m.Put(node.Hash, uint64(idx)) //nolint:gosec // G115: integer overflow conversion int -> uint32
	}

	return m, nil
}

// NodeIndex returns the index of the node with the given hash in the subtree.
func (st *Subtree) NodeIndex(hash chainhash.Hash) int {
	if st.nodeIndex == nil {
		// create the node index map
		st.mu.Lock()
		st.nodeIndex = make(map[chainhash.Hash]int, len(st.Nodes))

		for idx, node := range st.Nodes {
			st.nodeIndex[node.Hash] = idx
		}

		st.mu.Unlock()
	}

	nodeIndex, ok := st.nodeIndex[hash]
	if ok {
		return nodeIndex
	}

	return -1
}

// HasNode checks if the subtree contains a node with the given hash.
func (st *Subtree) HasNode(hash chainhash.Hash) bool {
	return st.NodeIndex(hash) != -1
}

// GetNode returns the Node with the given hash, or an error if it does not exist.
func (st *Subtree) GetNode(hash chainhash.Hash) (*Node, error) {
	nodeIndex := st.NodeIndex(hash)
	if nodeIndex != -1 {
		return &st.Nodes[nodeIndex], nil
	}

	return nil, ErrNodeNotFound
}

// Difference returns the nodes in the subtree that are not present in the given TxMap.
func (st *Subtree) Difference(ids TxMap) ([]Node, error) {
	// return all the ids that are in st.Nodes, but not in ids
	diff := make([]Node, 0, 1_000)

	for _, node := range st.Nodes {
		if !ids.Exists(node.Hash) {
			diff = append(diff, node)
		}
	}

	return diff, nil
}

// GetMerkleProof returns the merkle proof for the given index
// TODO rewrite this to calculate this from the subtree nodes needed, and not the whole tree
func (st *Subtree) GetMerkleProof(index int) ([]*chainhash.Hash, error) {
	if index >= len(st.Nodes) {
		return nil, ErrIndexOutOfRange
	}

	merkleTree, err := BuildMerkleTreeStoreFromBytes(st.Nodes)
	if err != nil {
		return nil, err
	}

	height := math.Ceil(math.Log2(float64(len(st.Nodes))))
	totalLength := int(math.Pow(2, height)) + len(*merkleTree)

	treeIndexPos := 0
	treeIndex := index
	nodes := make([]*chainhash.Hash, 0, int(height))

	for i := height; i > 0; i-- {
		if i == height {
			// we are at the leaf level and read from the Nodes array
			siblingHash := getLeafSiblingHash(st.Nodes, index)
			nodes = append(nodes, siblingHash)
		} else {
			treePos := calculateTreePosition(merkleTree, treeIndexPos, treeIndex, totalLength)
			nodes = append(nodes, &(*merkleTree)[treePos])
			treeIndexPos += int(math.Pow(2, i))
		}

		treeIndex = int(math.Floor(float64(treeIndex) / 2))
	}

	return nodes, nil
}

// getLeafSiblingHash returns the hash of the sibling node at the leaf level
func getLeafSiblingHash(nodes []Node, index int) *chainhash.Hash {
	if index%2 == 0 {
		// For even index, sibling is at index+1
		// But if index+1 is out of bounds (odd number of leaves),
		// duplicate the last node (Bitcoin convention)
		if index+1 >= len(nodes) {
			return &nodes[index].Hash
		}
		return &nodes[index+1].Hash
	}

	return &nodes[index-1].Hash
}

// calculateTreePosition calculates the tree position for the merkle proof sibling
func calculateTreePosition(merkleTree *[]chainhash.Hash, treeIndexPos, treeIndex, totalLength int) int {
	treePos := treeIndexPos + treeIndex

	if treePos%2 == 0 {
		if totalLength > treePos+1 && !(*merkleTree)[treePos+1].Equal(chainhash.Hash{}) {
			return treePos + 1
		}
	} else {
		if !(*merkleTree)[treePos-1].Equal(chainhash.Hash{}) {
			return treePos - 1
		}
	}

	return treePos
}

// Serialize serializes the subtree into a byte slice.
func (st *Subtree) Serialize() ([]byte, error) {
	bufBytes := make([]byte, 0, 32+8+8+8+(len(st.Nodes)*32)+8+(len(st.ConflictingNodes)*32))
	buf := bytes.NewBuffer(bufBytes)

	// write root hash - this is only for checking the correctness of the data
	_, err := buf.Write(st.RootHash()[:])
	if err != nil {
		return nil, fmt.Errorf("unable to write root hash: %w", err)
	}

	var b [8]byte

	// write fees
	binary.LittleEndian.PutUint64(b[:], st.Fees)

	if _, err = buf.Write(b[:]); err != nil {
		return nil, fmt.Errorf("unable to write fees: %w", err)
	}

	// write size
	binary.LittleEndian.PutUint64(b[:], st.SizeInBytes)

	if _, err = buf.Write(b[:]); err != nil {
		return nil, fmt.Errorf("unable to write sizeInBytes: %w", err)
	}

	// write number of nodes
	binary.LittleEndian.PutUint64(b[:], uint64(len(st.Nodes)))

	if _, err = buf.Write(b[:]); err != nil {
		return nil, fmt.Errorf("unable to write number of nodes: %w", err)
	}

	// write nodes
	feeBytes := make([]byte, 8)
	sizeBytes := make([]byte, 8)

	for _, subtreeNode := range st.Nodes {
		_, err = buf.Write(subtreeNode.Hash[:])
		if err != nil {
			return nil, fmt.Errorf("unable to write node: %w", err)
		}

		binary.LittleEndian.PutUint64(feeBytes, subtreeNode.Fee)

		_, err = buf.Write(feeBytes)
		if err != nil {
			return nil, fmt.Errorf("unable to write fee: %w", err)
		}

		binary.LittleEndian.PutUint64(sizeBytes, subtreeNode.SizeInBytes)

		_, err = buf.Write(sizeBytes)
		if err != nil {
			return nil, fmt.Errorf("unable to write sizeInBytes: %w", err)
		}
	}

	// write number of conflicting nodes
	binary.LittleEndian.PutUint64(b[:], uint64(len(st.ConflictingNodes)))

	if _, err = buf.Write(b[:]); err != nil {
		return nil, fmt.Errorf("unable to write number of conflicting nodes: %w", err)
	}

	// write conflicting nodes
	for _, nodeHash := range st.ConflictingNodes {
		_, err = buf.Write(nodeHash[:])
		if err != nil {
			return nil, fmt.Errorf("unable to write conflicting node: %w", err)
		}
	}

	return buf.Bytes(), nil
}

// SerializeNodes serializes only the nodes (list of transaction ids), not the root hash, fees, etc.
func (st *Subtree) SerializeNodes() ([]byte, error) {
	b := make([]byte, 0, len(st.Nodes)*32)
	buf := bytes.NewBuffer(b)

	var err error

	// write nodes
	for _, subtreeNode := range st.Nodes {
		if _, err = buf.Write(subtreeNode.Hash[:]); err != nil {
			return nil, fmt.Errorf("unable to write node: %w", err)
		}
	}

	return buf.Bytes(), nil
}

// Deserialize deserializes the subtree from the provided byte slice.
func (st *Subtree) Deserialize(b []byte) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered in Deserialize: %w: %v", err, r)
		}
	}()

	buf := bytes.NewBuffer(b)

	return st.DeserializeFromReader(buf)
}

// DeserializeFromReader deserializes the subtree from the provided reader.
func (st *Subtree) DeserializeFromReader(reader io.Reader) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered in DeserializeFromReader: %w: %v", err, r)
		}
	}()

	buf := bufio.NewReaderSize(reader, 32*1024) // 32KB buffer

	bytes8 := make([]byte, 8)

	// read root hash
	st.rootHash = new(chainhash.Hash)
	if _, err = io.ReadFull(buf, st.rootHash[:]); err != nil {
		return fmt.Errorf("unable to read root hash: %w", err)
	}

	// read fees
	if _, err = io.ReadFull(buf, bytes8); err != nil {
		return fmt.Errorf("unable to read fees: %w", err)
	}

	st.Fees = binary.LittleEndian.Uint64(bytes8)

	// read sizeInBytes
	if _, err = io.ReadFull(buf, bytes8); err != nil {
		return fmt.Errorf("unable to read sizeInBytes: %w", err)
	}

	st.SizeInBytes = binary.LittleEndian.Uint64(bytes8)

	if err = st.deserializeNodes(buf); err != nil {
		return err
	}

	if err = st.deserializeConflictingNodes(buf); err != nil {
		return err
	}

	return nil
}

// deserializeNodes deserializes the nodes from the provided buffered reader.
func (st *Subtree) deserializeNodes(buf *bufio.Reader) error {
	bytes8 := make([]byte, 8)

	// read number of leaves
	if _, err := io.ReadFull(buf, bytes8); err != nil {
		return fmt.Errorf("unable to read number of leaves: %w", err)
	}

	numLeaves := binary.LittleEndian.Uint64(bytes8)

	st.treeSize = int(numLeaves) //nolint:gosec // G115: integer overflow conversion int -> uint32
	// the height of a subtree is always a power of two
	st.Height = int(math.Ceil(math.Log2(float64(numLeaves))))

	// read leaves
	st.Nodes = make([]Node, numLeaves)

	bytes48 := make([]byte, 48)
	for i := uint64(0); i < numLeaves; i++ {
		// read all the node data in 1 go
		if _, err := io.ReadFull(buf, bytes48); err != nil {
			return fmt.Errorf("unable to read node: %w", err)
		}

		st.Nodes[i].Hash = chainhash.Hash(bytes48[:32])
		st.Nodes[i].Fee = binary.LittleEndian.Uint64(bytes48[32:40])
		st.Nodes[i].SizeInBytes = binary.LittleEndian.Uint64(bytes48[40:48])
	}

	return nil
}

// deserializeConflictingNodes deserializes the conflicting nodes from the provided buffered reader.
func (st *Subtree) deserializeConflictingNodes(buf *bufio.Reader) error {
	bytes8 := make([]byte, 8)

	// read the number of conflicting nodes
	if _, err := io.ReadFull(buf, bytes8); err != nil {
		return fmt.Errorf("unable to read number of conflicting nodes: %w", err)
	}

	numConflictingLeaves := binary.LittleEndian.Uint64(bytes8)

	// read conflicting nodes
	st.ConflictingNodes = make([]chainhash.Hash, numConflictingLeaves)

	for i := uint64(0); i < numConflictingLeaves; i++ {
		if _, err := io.ReadFull(buf, st.ConflictingNodes[i][:]); err != nil {
			return fmt.Errorf("unable to read conflicting node %d: %w", i, err)
		}
	}

	return nil
}

// DeserializeSubtreeConflictingFromReader deserializes the conflicting nodes from the provided reader.
func DeserializeSubtreeConflictingFromReader(reader io.Reader) (conflictingNodes []chainhash.Hash, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered in DeserializeSubtreeConflictingFromReader: %w: %v", err, r)
		}
	}()

	buf := bufio.NewReaderSize(reader, 32*1024) // 32KB buffer

	// skip root hash 32 bytes
	// skip fees, 8 bytes
	// skip sizeInBytes, 8 bytes
	_, _ = buf.Discard(32 + 8 + 8)

	bytes8 := make([]byte, 8)

	// read number of leaves
	if _, err = io.ReadFull(buf, bytes8); err != nil {
		return nil, fmt.Errorf("unable to read number of leaves: %w", err)
	}

	numLeaves := binary.LittleEndian.Uint64(bytes8)

	numLeavesInt, err := safe.Uint64ToInt(numLeaves)
	if err != nil {
		return nil, err
	}

	_, _ = buf.Discard(48 * numLeavesInt)

	// read the number of conflicting nodes
	if _, err = io.ReadFull(buf, bytes8); err != nil {
		return nil, fmt.Errorf("unable to read number of conflicting nodes: %w", err)
	}

	numConflictingLeaves := binary.LittleEndian.Uint64(bytes8)

	// read conflicting nodes
	conflictingNodes = make([]chainhash.Hash, numConflictingLeaves)
	for i := uint64(0); i < numConflictingLeaves; i++ {
		if _, err = io.ReadFull(buf, conflictingNodes[i][:]); err != nil {
			return nil, fmt.Errorf("unable to read conflicting node: %w", err)
		}
	}

	return conflictingNodes, nil
}

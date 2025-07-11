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

// SubtreeNode represents a node in the subtree.
type SubtreeNode struct {
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
	Nodes            []SubtreeNode
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
		return nil, fmt.Errorf("height must be at least 0")
	}

	treeSize := int(math.Pow(2, float64(height)))

	return &Subtree{
		Nodes:    make([]SubtreeNode, 0, treeSize),
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
		return nil, fmt.Errorf("numberOfLeaves must be a power of two")
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
	buf := bufio.NewReaderSize(reader, 1024*1024*16) // 16MB buffer

	// root len(st.rootHash[:]) bytes
	// first 8 bytes, fees
	// second 8 bytes, sizeInBytes
	// third 8 bytes, number of leaves
	// total read at once = len(st.rootHash[:]) + 8 + 8 + 8
	byteBuffer := make([]byte, chainhash.HashSize+24)
	if _, err = ReadBytes(buf, byteBuffer); err != nil {
		return nil, fmt.Errorf("unable to read subtree root information: %w", err)
	}

	numLeaves := binary.LittleEndian.Uint64(byteBuffer[chainhash.HashSize+16 : chainhash.HashSize+24])
	subtreeBytes = make([]byte, chainhash.HashSize*int(numLeaves)) //nolint:gosec // G115: integer overflow conversion

	byteBuffer = byteBuffer[8:] // reduce read byteBuffer size by 8
	for i := uint64(0); i < numLeaves; i++ {
		if _, err = ReadBytes(buf, byteBuffer); err != nil {
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
		Nodes:            make([]SubtreeNode, len(st.Nodes)),
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
func (st *Subtree) ReplaceRootNode(node *chainhash.Hash, fee uint64, sizeInBytes uint64) *chainhash.Hash {
	if len(st.Nodes) < 1 {
		st.Nodes = append(st.Nodes, SubtreeNode{
			Hash:        *node,
			Fee:         fee,
			SizeInBytes: sizeInBytes,
		})
	} else {
		st.Nodes[0] = SubtreeNode{
			Hash:        *node,
			Fee:         fee,
			SizeInBytes: sizeInBytes,
		}
	}

	st.rootHash = nil // reset rootHash
	st.SizeInBytes += sizeInBytes

	return st.RootHash()
}

// AddSubtreeNode adds a SubtreeNode to the subtree.
func (st *Subtree) AddSubtreeNode(node SubtreeNode) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	if (len(st.Nodes) + 1) > st.treeSize {
		return fmt.Errorf("subtree is full")
	}

	if node.Hash.Equal(CoinbasePlaceholder) {
		return fmt.Errorf("[AddSubtreeNode] coinbase placeholder node should be added with AddCoinbaseNode, tree length is %d", len(st.Nodes))
	}

	// AddNode is not concurrency safe, so we can reuse the same byte arrays
	// binary.LittleEndian.PutUint64(st.feeBytes, fee)
	// st.feeHashBytes = append(node[:], st.feeBytes[:]...)
	// if len(st.Nodes) == 0 {
	//	st.FeeHash = chainhash.HashH(st.feeHashBytes)
	// } else {
	//	st.FeeHash = chainhash.HashH(append(st.FeeHash[:], st.feeHashBytes...))
	// }

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
		return fmt.Errorf("subtree should be empty before adding a coinbase node")
	}

	st.Nodes = append(st.Nodes, SubtreeNode{
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
		return fmt.Errorf("conflicting node is not in the subtree")
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
func (st *Subtree) AddNode(node chainhash.Hash, fee uint64, sizeInBytes uint64) error {
	if (len(st.Nodes) + 1) > st.treeSize {
		return fmt.Errorf("subtree is full")
	}

	if node.Equal(CoinbasePlaceholder) {
		return fmt.Errorf("[AddNode] coinbase placeholder node should be added with AddCoinbaseNode")
	}

	// AddNode is not concurrency safe, so we can reuse the same byte arrays
	// binary.LittleEndian.PutUint64(st.feeBytes, fee)
	// st.feeHashBytes = append(node[:], st.feeBytes[:]...)
	// if len(st.Nodes) == 0 {
	//	st.FeeHash = chainhash.HashH(st.feeHashBytes)
	// } else {
	//	st.FeeHash = chainhash.HashH(append(st.FeeHash[:], st.feeHashBytes...))
	// }

	st.Nodes = append(st.Nodes, SubtreeNode{
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
		return fmt.Errorf("index out of range")
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
func (st *Subtree) RootHashWithReplaceRootNode(node *chainhash.Hash, fee uint64, sizeInBytes uint64) (*chainhash.Hash, error) {
	if st == nil {
		return nil, fmt.Errorf("subtree is nil")
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

// GetNode returns the SubtreeNode with the given hash, or an error if it does not exist.
func (st *Subtree) GetNode(hash chainhash.Hash) (*SubtreeNode, error) {
	nodeIndex := st.NodeIndex(hash)
	if nodeIndex != -1 {
		return &st.Nodes[nodeIndex], nil
	}

	return nil, fmt.Errorf("node not found")
}

// Difference returns the nodes in the subtree that are not present in the given TxMap.
func (st *Subtree) Difference(ids TxMap) ([]SubtreeNode, error) {
	// return all the ids that are in st.Nodes, but not in ids
	diff := make([]SubtreeNode, 0, 1_000)

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
		return nil, fmt.Errorf("index out of range")
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
			if index%2 == 0 {
				nodes = append(nodes, &st.Nodes[index+1].Hash)
			} else {
				nodes = append(nodes, &st.Nodes[index-1].Hash)
			}
		} else {
			treePos := treeIndexPos + treeIndex
			if treePos%2 == 0 {
				if totalLength > treePos+1 && !(*merkleTree)[treePos+1].Equal(chainhash.Hash{}) {
					treePos++
				}
			} else {
				if !(*merkleTree)[treePos-1].Equal(chainhash.Hash{}) {
					treePos--
				}
			}

			nodes = append(nodes, &(*merkleTree)[treePos])
			treeIndexPos += int(math.Pow(2, i))
		}

		treeIndex = int(math.Floor(float64(treeIndex) / 2))
	}

	return nodes, nil
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
			err = fmt.Errorf("recovered in Deserialize: %s", r)
		}
	}()

	buf := bytes.NewBuffer(b)

	return st.DeserializeFromReader(buf)
}

// DeserializeFromReader deserializes the subtree from the provided reader.
func (st *Subtree) DeserializeFromReader(reader io.Reader) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered in DeserializeFromReader: %s", r)
		}
	}()

	buf := bufio.NewReaderSize(reader, 1024*1024*16) // 16MB buffer

	var (
		n      int
		bytes8 = make([]byte, 8)
	)

	// read root hash
	st.rootHash = new(chainhash.Hash)
	if n, err = buf.Read(st.rootHash[:]); err != nil || n != chainhash.HashSize {
		// if _, err = io.ReadFull(buf, st.rootHash[:]); err != nil {
		return fmt.Errorf("unable to read root hash: %w", err)
	}

	// read fees
	if n, err = buf.Read(bytes8); err != nil || n != 8 {
		// if _, err = io.ReadFull(buf, bytes8); err != nil {
		return fmt.Errorf("unable to read fees: %w", err)
	}

	st.Fees = binary.LittleEndian.Uint64(bytes8)

	// read sizeInBytes
	if n, err = buf.Read(bytes8); err != nil || n != 8 {
		// if _, err = io.ReadFull(buf, bytes8); err != nil {
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
	if n, err := buf.Read(bytes8); err != nil || n != 8 {
		// if _, err = io.ReadFull(buf, bytes8); err != nil {
		return fmt.Errorf("unable to read number of leaves: %w", err)
	}

	numLeaves := binary.LittleEndian.Uint64(bytes8)

	st.treeSize = int(numLeaves) //nolint:gosec // G115: integer overflow conversion int -> uint32
	// the height of a subtree is always a power of two
	st.Height = int(math.Ceil(math.Log2(float64(numLeaves))))

	// read leaves
	st.Nodes = make([]SubtreeNode, numLeaves)

	bytes48 := make([]byte, 48)
	for i := uint64(0); i < numLeaves; i++ {
		// read all the node data in 1 go
		if n, err := ReadBytes(buf, bytes48); err != nil || n != 48 {
			// if _, err = io.ReadFull(buf, bytes48); err != nil {
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
	if n, err := buf.Read(bytes8); err != nil || n != 8 {
		// if _, err = io.ReadFull(buf, bytes8); err != nil {
		return fmt.Errorf("unable to read number of conflicting nodes: %w", err)
	}

	numConflictingLeaves := binary.LittleEndian.Uint64(bytes8)

	// read conflicting nodes
	st.ConflictingNodes = make([]chainhash.Hash, numConflictingLeaves)

	for i := uint64(0); i < numConflictingLeaves; i++ {
		if n, err := buf.Read(st.ConflictingNodes[i][:]); err != nil || n != 32 {
			return fmt.Errorf("unable to read conflicting node %d: %w", i, err)
		}
	}

	return nil
}

// ReadBytes reads bytes from the buffered reader into the provided byte slice.
func ReadBytes(buf *bufio.Reader, p []byte) (n int, err error) {
	minRead := len(p)
	for n < minRead && err == nil {
		p[n], err = buf.ReadByte()
		n++
	}

	if n >= minRead {
		err = nil
	} else if n > 0 && err == io.EOF {
		err = io.ErrUnexpectedEOF
	}

	return
}

// DeserializeSubtreeConflictingFromReader deserializes the conflicting nodes from the provided reader.
func DeserializeSubtreeConflictingFromReader(reader io.Reader) (conflictingNodes []chainhash.Hash, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered in DeserializeFromReader: %s", r)
		}
	}()

	buf := bufio.NewReaderSize(reader, 1024*1024*16) // 16MB buffer

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

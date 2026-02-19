package subtree

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"slices"

	"github.com/bsv-blockchain/go-bt/v2"
	"github.com/bsv-blockchain/go-bt/v2/chainhash"
)

// Inpoint represents an input point in a transaction, consisting of a parent transaction hash and an index.
type Inpoint struct {
	Hash  chainhash.Hash
	Index uint32
}

// TxInpoints represents a collection of transaction inpoints, which are the parent transaction hashes and their corresponding indexes.
type TxInpoints struct {
	ParentTxHashes []chainhash.Hash
	Idxs           [][]uint32

	// internal variable
	nrInpoints int

	// SubtreeIndex tracks which subtree this transaction belongs to in the subtree processor.
	// -1 = current (incomplete) subtree, 0..N = index in chainedSubtrees.
	// Runtime-only — not included in serialization.
	SubtreeIndex int16
}

// NewTxInpoints creates a new TxInpoints object with initialized slices for parent transaction hashes and indexes.
func NewTxInpoints() TxInpoints {
	return TxInpoints{
		ParentTxHashes: make([]chainhash.Hash, 0, 8), // initial capacity of 8 can grow as needed
		Idxs:           make([][]uint32, 0, 16),      // the initial capacity of 16 can grow as needed
		SubtreeIndex:   -1,                            // -1 = current/unassigned
	}
}

// NewTxInpointsFromTx creates a new TxInpoints object from a given transaction.
func NewTxInpointsFromTx(tx *bt.Tx) (TxInpoints, error) {
	p := NewTxInpoints()
	p.addTx(tx)

	return p, nil
}

// NewTxInpointsFromInputs creates a new TxInpoints object from a slice of transaction inputs.
func NewTxInpointsFromInputs(inputs []*bt.Input) (TxInpoints, error) {
	p := TxInpoints{}

	tx := &bt.Tx{}
	tx.Inputs = inputs

	p.addTx(tx)

	return p, nil
}

// NewTxInpointsFromBytes creates a new TxInpoints object from a byte slice.
func NewTxInpointsFromBytes(data []byte) (TxInpoints, error) {
	p := TxInpoints{}

	if err := p.deserializeFromReader(bytes.NewReader(data)); err != nil {
		return p, err
	}

	return p, nil
}

// NewTxInpointsFromReader creates a new TxInpoints object from an io.Reader.
func NewTxInpointsFromReader(buf io.Reader) (TxInpoints, error) {
	p := TxInpoints{}

	if err := p.deserializeFromReader(buf); err != nil {
		return p, err
	}

	return p, nil
}

// String returns a string representation of the TxInpoints object.
func (p *TxInpoints) String() string {
	return fmt.Sprintf("TxInpoints{ParentTxHashes: %v, Idxs: %v}", p.ParentTxHashes, p.Idxs)
}

// GetParentTxHashes returns the unique parent tx hashes
func (p *TxInpoints) GetParentTxHashes() []chainhash.Hash {
	return p.ParentTxHashes
}

// GetParentTxHashAtIndex returns the parent transaction hash at the specified index.
func (p *TxInpoints) GetParentTxHashAtIndex(index int) (chainhash.Hash, error) {
	if index >= len(p.ParentTxHashes) {
		return chainhash.Hash{}, ErrIndexOutOfRange
	}

	return p.ParentTxHashes[index], nil
}

// GetTxInpoints returns the unique parent inpoints for the tx
func (p *TxInpoints) GetTxInpoints() []Inpoint {
	inpoints := make([]Inpoint, 0, p.nrInpoints)

	for i, hash := range p.ParentTxHashes {
		for _, index := range p.Idxs[i] {
			inpoints = append(inpoints, Inpoint{
				Hash:  hash,
				Index: index,
			})
		}
	}

	return inpoints
}

// GetParentVoutsAtIndex returns the parent transaction output indexes at the specified index.
func (p *TxInpoints) GetParentVoutsAtIndex(index int) ([]uint32, error) {
	if index >= len(p.ParentTxHashes) {
		return nil, ErrIndexOutOfRange
	}

	return p.Idxs[index], nil
}

// Serialize serializes the TxInpoints object into a byte slice.
func (p *TxInpoints) Serialize() ([]byte, error) {
	if len(p.ParentTxHashes) != len(p.Idxs) {
		return nil, ErrParentTxHashesMismatch
	}

	bufBytes := make([]byte, 0, 1024) // 1KB (arbitrary size, should be enough for most cases)
	buf := bytes.NewBuffer(bufBytes)

	var (
		err         error
		bytesUint32 [4]byte
	)

	binary.LittleEndian.PutUint32(bytesUint32[:], len32(p.ParentTxHashes))

	if _, err = buf.Write(bytesUint32[:]); err != nil {
		return nil, fmt.Errorf("unable to write number of parent inpoints: %w", err)
	}

	// write the parent tx hashes
	for _, hash := range p.ParentTxHashes {
		if _, err = buf.Write(hash[:]); err != nil {
			return nil, fmt.Errorf("unable to write parent tx hash: %w", err)
		}
	}

	// write the parent indexes
	for _, indexes := range p.Idxs {
		binary.LittleEndian.PutUint32(bytesUint32[:], len32(indexes))

		if _, err = buf.Write(bytesUint32[:]); err != nil {
			return nil, fmt.Errorf("unable to write number of parent indexes: %w", err)
		}

		for _, idx := range indexes {
			binary.LittleEndian.PutUint32(bytesUint32[:], idx)

			if _, err = buf.Write(bytesUint32[:]); err != nil {
				return nil, fmt.Errorf("unable to write parent index: %w", err)
			}
		}
	}

	return buf.Bytes(), nil
}

// addTx adds a transaction to the TxInpoints object, extracting its inputs and updating the parent transaction hashes and indexes.
func (p *TxInpoints) addTx(tx *bt.Tx) {
	// Do not error out for transactions without inputs, seeded Teranodes will have txs without inputs
	for _, input := range tx.Inputs {
		hash := *input.PreviousTxIDChainHash()

		index := slices.Index(p.ParentTxHashes, hash)
		if index != -1 {
			p.Idxs[index] = append(p.Idxs[index], input.PreviousTxOutIndex)
		} else {
			p.ParentTxHashes = append(p.ParentTxHashes, hash)
			p.Idxs = append(p.Idxs, []uint32{input.PreviousTxOutIndex})
		}

		p.nrInpoints++
	}
}

// deserializeFromReader reads the TxInpoints data from the provided reader and populates the TxInpoints object.
func (p *TxInpoints) deserializeFromReader(buf io.Reader) error {
	// read the number of parent inpoints
	var bytesUint32 [4]byte

	if _, err := io.ReadFull(buf, bytesUint32[:]); err != nil {
		return fmt.Errorf("unable to read number of parent inpoints: %w", err)
	}

	totalInpointsLen := binary.LittleEndian.Uint32(bytesUint32[:])

	if totalInpointsLen == 0 {
		return nil
	}

	p.nrInpoints = int(totalInpointsLen)

	// read the parent inpoints
	p.ParentTxHashes = make([]chainhash.Hash, totalInpointsLen)
	p.Idxs = make([][]uint32, totalInpointsLen)

	// read the parent tx hash
	for i := uint32(0); i < totalInpointsLen; i++ {
		if _, err := io.ReadFull(buf, p.ParentTxHashes[i][:]); err != nil {
			return fmt.Errorf("unable to read parent tx hash: %w", err)
		}
	}

	// read the number of parent indexes
	for i := uint32(0); i < totalInpointsLen; i++ {
		if _, err := io.ReadFull(buf, bytesUint32[:]); err != nil {
			return fmt.Errorf("unable to read number of parent indexes: %w", err)
		}

		parentIndexesLen := binary.LittleEndian.Uint32(bytesUint32[:])

		// read the parent indexes
		p.Idxs[i] = make([]uint32, parentIndexesLen)

		for j := uint32(0); j < parentIndexesLen; j++ {
			if _, err := io.ReadFull(buf, bytesUint32[:]); err != nil {
				return fmt.Errorf("unable to read parent index: %w", err)
			}

			p.Idxs[i][j] = binary.LittleEndian.Uint32(bytesUint32[:])
		}
	}

	return nil
}

func len32[V any](b []V) uint32 {
	if b == nil {
		return 0
	}

	l := len(b)

	if l > math.MaxUint32 {
		return math.MaxInt32
	}

	return uint32(l)
}

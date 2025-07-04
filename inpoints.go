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

type Inpoint struct {
	Hash  chainhash.Hash
	Index uint32
}

type TxInpoints struct {
	ParentTxHashes []chainhash.Hash
	Idxs           [][]uint32

	// internal variable
	nrInpoints int
}

func NewTxInpoints() TxInpoints {
	return TxInpoints{
		ParentTxHashes: make([]chainhash.Hash, 0, 8), // initial capacity of 8, can grow as needed
		Idxs:           make([][]uint32, 0, 16),      // initial capacity of 16, can grow as needed
	}
}

func NewTxInpointsFromTx(tx *bt.Tx) (TxInpoints, error) {
	p := NewTxInpoints()
	p.addTx(tx)

	return p, nil
}

func NewTxInpointsFromInputs(inputs []*bt.Input) (TxInpoints, error) {
	p := TxInpoints{}

	tx := &bt.Tx{}
	tx.Inputs = inputs

	p.addTx(tx)

	return p, nil
}

func NewTxInpointsFromBytes(data []byte) (TxInpoints, error) {
	p := TxInpoints{}

	if err := p.deserializeFromReader(bytes.NewReader(data)); err != nil {
		return p, err
	}

	return p, nil
}

func NewTxInpointsFromReader(buf io.Reader) (TxInpoints, error) {
	p := TxInpoints{}

	if err := p.deserializeFromReader(buf); err != nil {
		return p, err
	}

	return p, nil
}

func (p *TxInpoints) String() string {
	return fmt.Sprintf("TxInpoints{ParentTxHashes: %v, Idxs: %v}", p.ParentTxHashes, p.Idxs)
}

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

// GetParentTxHashes returns the unique parent tx hashes
func (p *TxInpoints) GetParentTxHashes() []chainhash.Hash {
	return p.ParentTxHashes
}

func (p *TxInpoints) GetParentTxHashAtIndex(index int) (chainhash.Hash, error) {
	if index >= len(p.ParentTxHashes) {
		return chainhash.Hash{}, fmt.Errorf("index out of range")
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

func (p *TxInpoints) GetParentVoutsAtIndex(index int) ([]uint32, error) {
	if index >= len(p.ParentTxHashes) {
		return nil, fmt.Errorf("index out of range")
	}

	return p.Idxs[index], nil
}

func (p *TxInpoints) Serialize() ([]byte, error) {
	if len(p.ParentTxHashes) != len(p.Idxs) {
		return nil, fmt.Errorf("parent tx hashes and indexes length mismatch")
	}

	bufBytes := make([]byte, 0, 1024) // 1KB (arbitrary size, should be enough for most cases)
	buf := bytes.NewBuffer(bufBytes)

	var (
		err         error
		bytesUint32 [4]byte
	)

	binary.LittleEndian.PutUint32(bytesUint32[:], len32(p.ParentTxHashes))

	if _, err = buf.Write(bytesUint32[:]); err != nil {
		return nil, fmt.Errorf("unable to write number of parent inpoints: %s", err)
	}

	// write the parent tx hashes
	for _, hash := range p.ParentTxHashes {
		if _, err = buf.Write(hash[:]); err != nil {
			return nil, fmt.Errorf("unable to write parent tx hash: %s", err)
		}
	}

	// write the parent indexes
	for _, indexes := range p.Idxs {
		binary.LittleEndian.PutUint32(bytesUint32[:], len32(indexes))

		if _, err = buf.Write(bytesUint32[:]); err != nil {
			return nil, fmt.Errorf("unable to write number of parent indexes: %s", err)
		}

		for _, idx := range indexes {
			binary.LittleEndian.PutUint32(bytesUint32[:], idx)

			if _, err = buf.Write(bytesUint32[:]); err != nil {
				return nil, fmt.Errorf("unable to write parent index: %s", err)
			}
		}
	}

	return buf.Bytes(), nil
}

func (p *TxInpoints) deserializeFromReader(buf io.Reader) error {
	// read the number of parent inpoints
	var bytesUint32 [4]byte

	if _, err := io.ReadFull(buf, bytesUint32[:]); err != nil {
		return fmt.Errorf("unable to read number of parent inpoints: %s", err)
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
			return fmt.Errorf("unable to read parent tx hash: %s", err)
		}
	}

	// read the number of parent indexes
	for i := uint32(0); i < totalInpointsLen; i++ {
		if _, err := io.ReadFull(buf, bytesUint32[:]); err != nil {
			return fmt.Errorf("unable to read number of parent indexes: %s", err)
		}

		parentIndexesLen := binary.LittleEndian.Uint32(bytesUint32[:])

		// read the parent indexes
		p.Idxs[i] = make([]uint32, parentIndexesLen)

		for j := uint32(0); j < parentIndexesLen; j++ {
			if _, err := io.ReadFull(buf, bytesUint32[:]); err != nil {
				return fmt.Errorf("unable to read parent index: %s", err)
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

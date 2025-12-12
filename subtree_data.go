package subtree

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/bsv-blockchain/go-bt/v2"
	"github.com/bsv-blockchain/go-bt/v2/chainhash"
)

// Data represents the data associated with a subtree.
type Data struct {
	Subtree *Subtree
	Txs     []*bt.Tx
}

// NewSubtreeData creates a new Data object
// the size parameter is the number of nodes in the subtree,
// the index in that array should match the index of the node in the subtree
func NewSubtreeData(subtree *Subtree) *Data {
	return &Data{
		Subtree: subtree,
		Txs:     make([]*bt.Tx, subtree.Size()),
	}
}

// NewSubtreeDataFromBytes creates a new Data object from the provided byte slice.
func NewSubtreeDataFromBytes(subtree *Subtree, dataBytes []byte) (*Data, error) {
	s := &Data{
		Subtree: subtree,
	}
	if err := s.serializeFromReader(bytes.NewReader(dataBytes)); err != nil {
		return nil, fmt.Errorf("unable to create subtree data from bytes: %w", err)
	}

	return s, nil
}

// NewSubtreeDataFromReader creates a new Data object from the provided reader.
func NewSubtreeDataFromReader(subtree *Subtree, dataReader io.Reader) (*Data, error) {
	s := &Data{
		Subtree: subtree,
	}
	if err := s.serializeFromReader(dataReader); err != nil {
		return nil, fmt.Errorf("unable to create subtree data from reader: %w", err)
	}

	return s, nil
}

// RootHash returns the root hash of the subtree.
func (s *Data) RootHash() *chainhash.Hash {
	return s.Subtree.RootHash()
}

// AddTx adds a transaction to the subtree data at the specified index.
func (s *Data) AddTx(tx *bt.Tx, index int) error {
	if index == 0 && tx.IsCoinbase() && s.Subtree.Nodes[index].Hash.Equal(CoinbasePlaceholderHashValue) {
		// we got the coinbase tx as the first tx, we need to add it as the first tx and stop further processing
		s.Txs[index] = tx

		return nil
	}

	// check whether this is set in the main subtree
	if !s.Subtree.Nodes[index].Hash.Equal(*tx.TxIDChainHash()) {
		return ErrTxHashMismatch
	}

	s.Txs[index] = tx

	return nil
}

// Serialize returns the serialized form of the subtree meta
func (s *Data) Serialize() ([]byte, error) {
	var err error

	// only serialize when we have the matching subtree
	if s.Subtree == nil {
		return nil, ErrCannotSerializeSubtreeNotSet
	}

	var txStartIndex int
	if s.Subtree.Nodes[0].Hash.Equal(*CoinbasePlaceholderHash) {
		txStartIndex = 1
	}

	// check the data in the subtree matches the data in the tx data
	subtreeLen := s.Subtree.Length()
	for i := txStartIndex; i < subtreeLen; i++ {
		if s.Txs[i] == nil && i != 0 {
			return nil, ErrSubtreeLengthMismatch
		}
	}

	bufBytes := make([]byte, 0, 32*1024) // 16MB (arbitrary size, should be enough for most cases)
	buf := bytes.NewBuffer(bufBytes)

	for i := txStartIndex; i < subtreeLen; i++ {
		b := s.Txs[i].SerializeBytes()

		_, err = buf.Write(b)
		if err != nil {
			return nil, fmt.Errorf("error writing tx data: %w", err)
		}
	}

	return buf.Bytes(), nil
}

// WriteTransactionsToWriter writes a range of transactions directly to a writer.
//
// This enables memory-efficient serialization by streaming transactions to disk as they are loaded,
// without requiring all transactions to be in memory simultaneously. Transactions in the specified
// range are written sequentially, skipping any nil entries.
//
// Parameters:
//   - w: Writer to stream transactions to
//   - startIdx: Starting index (inclusive) of transactions to write
//   - endIdx: Ending index (exclusive) of transactions to write
//
// Returns an error if writing fails or if required transactions are missing (nil).
func (s *Data) WriteTransactionsToWriter(w io.Writer, startIdx, endIdx int) error {
	if s.Subtree == nil {
		return ErrCannotSerializeSubtreeNotSet
	}

	for i := startIdx; i < endIdx; i++ {
		// Skip coinbase placeholder if it's the first transaction
		if i == 0 && s.Subtree.Nodes[0].Hash.Equal(*CoinbasePlaceholderHash) {
			continue
		}

		if s.Txs[i] == nil {
			return fmt.Errorf("transaction at index %d is nil, cannot serialize", i)
		}

		// Serialize and stream transaction bytes to writer
		txBytes := s.Txs[i].SerializeBytes()
		if _, err := w.Write(txBytes); err != nil {
			return fmt.Errorf("error writing transaction at index %d: %w", i, err)
		}
	}

	return nil
}

// ReadTransactionsFromReader reads a range of transactions from a reader.
//
// This enables memory-efficient deserialization by reading only a chunk of transactions
// from disk at a time, rather than loading all transactions into memory.
//
// Parameters:
//   - r: Reader to read transactions from
//   - startIdx: Starting index (inclusive) where transactions should be stored
//   - endIdx: Ending index (exclusive) where transactions should be stored
//
// Returns the number of transactions read and any error encountered.
func (s *Data) ReadTransactionsFromReader(r io.Reader, startIdx, endIdx int) (int, error) {
	if s.Subtree == nil || len(s.Subtree.Nodes) == 0 {
		return 0, ErrSubtreeNodesEmpty
	}

	txsRead := 0
	for i := startIdx; i < endIdx; i++ {
		// Skip coinbase placeholder
		if i == 0 && s.Subtree.Nodes[0].Hash.Equal(CoinbasePlaceholderHashValue) {
			continue
		}

		tx := &bt.Tx{}
		if _, err := tx.ReadFrom(r); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return txsRead, fmt.Errorf("error reading transaction at index %d: %w", i, err)
		}

		// Validate tx hash matches expected
		if !s.Subtree.Nodes[i].Hash.Equal(*tx.TxIDChainHash()) {
			return txsRead, ErrTxHashMismatch
		}

		s.Txs[i] = tx
		txsRead++
	}

	return txsRead, nil
}

// serializeFromReader reads transactions from the provided reader and populates the Txs field.
func (s *Data) serializeFromReader(buf io.Reader) error {
	var (
		err     error
		txIndex int
	)

	if s.Subtree == nil || len(s.Subtree.Nodes) == 0 {
		return ErrSubtreeNodesEmpty
	}

	if s.Subtree.Nodes[0].Hash.Equal(CoinbasePlaceholderHashValue) {
		txIndex = 1
	}

	// initialize the txs array
	s.Txs = make([]*bt.Tx, s.Subtree.Length())

	for {
		tx := &bt.Tx{}

		_, err = tx.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return fmt.Errorf("error reading transaction: %w", err)
		}

		if txIndex == 1 && tx.IsCoinbase() {
			// we got the coinbase tx as the first tx, we need to add it as the first tx and continue
			s.Txs[0] = tx

			continue
		}

		if txIndex >= len(s.Subtree.Nodes) {
			return ErrTxIndexOutOfBounds
		}

		if !s.Subtree.Nodes[txIndex].Hash.Equal(*tx.TxIDChainHash()) {
			return ErrTxHashMismatch
		}

		s.Txs[txIndex] = tx
		txIndex++
	}

	return nil
}

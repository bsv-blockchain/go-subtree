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

// Inpoint represents an input point in a transaction, consisting of a parent
// transaction hash and an index.
type Inpoint struct {
	Hash  chainhash.Hash
	Index uint32
}

// TxInpoints represents a collection of transaction inpoints — the deduplicated
// parent transaction hashes referenced by a transaction's inputs and the vout
// indexes consumed at each parent.
//
// The vout indexes are stored in a single packed allocation rather than the
// previous nested [][]uint32. For the common 1-2-input transaction this halves
// the heap-object count per TxInpoints and saves ~40% of bytes. The previous
// public field
//
//	Idxs [][]uint32
//
// has been removed; external callers must use GetParentVoutsAtIndex or
// GetTxInpoints. The rename is deliberate — code on the old API will fail to
// compile on upgrade rather than silently misuse the new layout.
type TxInpoints struct {
	// ParentTxHashes holds the deduplicated parent transaction hashes
	// referenced by this transaction's inputs. Semantics unchanged from the
	// pre-packed-layout versions.
	ParentTxHashes []chainhash.Hash

	// voutIdxs stores per-parent vout indexes in a single count-prefixed
	// packed layout. For each parent in ParentTxHashes order, voutIdxs holds
	// one count word followed by that many vout-value words, concatenated:
	//
	//   [c_0, v_0_0, v_0_1, ..., v_0_(c_0-1),
	//    c_1, v_1_0, ..., v_1_(c_1-1),
	//    ...]
	//
	// The layout matches the wire format produced by Serialize, which lets the
	// deserializer write straight into a single allocation.
	//
	// Invariant: when len(ParentTxHashes) == 0 then voutIdxs is also empty.
	// When P > 0, voutIdxs has at least P entries (the count words), and
	// every count is >= 1 (a parent with zero vouts cannot enter the slice).
	voutIdxs []uint32

	// SubtreeIndex tracks which subtree this transaction belongs to in the
	// subtree processor. 0 = unassigned, 1..N+1 = index+1 in chainedSubtrees
	// (offset by 1 so zero value = unassigned). Runtime-only — not serialized.
	SubtreeIndex int16
}

// NewTxInpoints creates an empty TxInpoints.
//
// Prior versions pre-allocated cap-8 ParentTxHashes and cap-16 Idxs to absorb
// append-driven growth. With the packed layout the upper bound on slice size
// is always known at construction time (len(tx.Inputs)), so callers that build
// from a tx should use NewTxInpointsFromTx / NewTxInpointsFromInputs which
// size exactly and never re-grow. NewTxInpoints exists for callers that want
// an empty zero-value placeholder; it allocates nothing.
func NewTxInpoints() TxInpoints {
	return TxInpoints{}
}

// NewTxInpointsFromTx creates a new TxInpoints object from a given transaction.
// Internal buffers are sized to the worst case for len(tx.Inputs), so
// construction never reallocates.
func NewTxInpointsFromTx(tx *bt.Tx) (TxInpoints, error) {
	return newSizedFromInputs(tx.Inputs), nil
}

// NewTxInpointsFromInputs creates a new TxInpoints object from a slice of
// transaction inputs.
func NewTxInpointsFromInputs(inputs []*bt.Input) (TxInpoints, error) {
	return newSizedFromInputs(inputs), nil
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

// String returns a string representation of the TxInpoints object. The format
// is kept compatible with the pre-packed version (it still names the field
// "Idxs") so any code that greps logs or test output keeps working.
func (p *TxInpoints) String() string {
	idxs := make([][]uint32, len(p.ParentTxHashes))
	for i := range p.ParentTxHashes {
		idxs[i] = p.voutSliceForParent(i)
	}

	return fmt.Sprintf("TxInpoints{ParentTxHashes: %v, Idxs: %v}", p.ParentTxHashes, idxs)
}

// GetParentTxHashes returns the unique parent tx hashes.
func (p *TxInpoints) GetParentTxHashes() []chainhash.Hash {
	return p.ParentTxHashes
}

// GetParentTxHashAtIndex returns the parent transaction hash at the specified
// index.
func (p *TxInpoints) GetParentTxHashAtIndex(index int) (chainhash.Hash, error) {
	if index < 0 || index >= len(p.ParentTxHashes) {
		return chainhash.Hash{}, ErrIndexOutOfRange
	}

	return p.ParentTxHashes[index], nil
}

// GetTxInpoints returns the inpoints for the tx as a flat (hash, vout) slice
// in parent-then-vout order. Allocates a new slice — the caller owns it.
func (p *TxInpoints) GetTxInpoints() []Inpoint {
	inpoints := make([]Inpoint, 0, p.nrInputs())

	pos := 0

	for _, hash := range p.ParentTxHashes {
		count := int(p.voutIdxs[pos])
		for k := 1; k <= count; k++ {
			inpoints = append(inpoints, Inpoint{
				Hash:  hash,
				Index: p.voutIdxs[pos+k],
			})
		}

		pos += 1 + count
	}

	return inpoints
}

// GetParentVoutsAtIndex returns the parent transaction output indexes at the
// specified parent index. The returned slice aliases TxInpoints' internal
// storage and MUST be treated as read-only by the caller.
func (p *TxInpoints) GetParentVoutsAtIndex(index int) ([]uint32, error) {
	if index < 0 || index >= len(p.ParentTxHashes) {
		return nil, ErrIndexOutOfRange
	}

	return p.voutSliceForParent(index), nil
}

// Serialize serializes the TxInpoints object into a byte slice. The wire
// format is unchanged from the pre-packed-layout version.
func (p *TxInpoints) Serialize() ([]byte, error) {
	parentCount := len(p.ParentTxHashes)

	// Layout invariant: when parentCount > 0 voutIdxs must hold at least the
	// parentCount count words. An empty TxInpoints requires voutIdxs to also
	// be empty.
	if (parentCount == 0 && len(p.voutIdxs) != 0) ||
		(parentCount > 0 && len(p.voutIdxs) < parentCount) {
		return nil, ErrParentTxHashesMismatch
	}

	// Pre-size: 4 byte header + 32 bytes per parent hash + 4 bytes per
	// voutIdxs entry (both count and value words).
	bufBytes := make([]byte, 0, 4+parentCount*32+len(p.voutIdxs)*4)
	buf := bytes.NewBuffer(bufBytes)

	var (
		err         error
		bytesUint32 [4]byte
	)

	binary.LittleEndian.PutUint32(bytesUint32[:], len32(p.ParentTxHashes))

	if _, err = buf.Write(bytesUint32[:]); err != nil {
		return nil, fmt.Errorf("unable to write number of parent inpoints: %w", err)
	}

	for _, hash := range p.ParentTxHashes {
		if _, err = buf.Write(hash[:]); err != nil {
			return nil, fmt.Errorf("unable to write parent tx hash: %w", err)
		}
	}

	// voutIdxs is already laid out as [count_0, vals..., count_1, vals..., ...]
	// — exactly the wire format. Stream it out one uint32 at a time so we
	// stay endian-correct.
	for _, v := range p.voutIdxs {
		binary.LittleEndian.PutUint32(bytesUint32[:], v)

		if _, err = buf.Write(bytesUint32[:]); err != nil {
			return nil, fmt.Errorf("unable to write parent index: %w", err)
		}
	}

	return buf.Bytes(), nil
}

// voutSliceForParent returns the underlying vout slice (no copy) for the i-th
// parent. The caller MUST treat the result as read-only — it aliases voutIdxs.
//
// O(P) in the parent count; P is typically 1-3, so this is amortized constant.
func (p *TxInpoints) voutSliceForParent(i int) []uint32 {
	pos := 0
	for j := 0; j < i; j++ {
		// skip one count word + count vout words
		pos += 1 + int(p.voutIdxs[pos])
	}

	count := int(p.voutIdxs[pos])

	return p.voutIdxs[pos+1 : pos+1+count]
}

// nrInputs returns the total number of tx inputs represented across all
// parents. 0 for an empty TxInpoints.
//
// Derived from the layout: each parent contributes exactly one count word in
// voutIdxs, so total values = len(voutIdxs) - len(ParentTxHashes).
func (p *TxInpoints) nrInputs() int {
	parentCount := len(p.ParentTxHashes)
	if parentCount == 0 {
		return 0
	}

	return len(p.voutIdxs) - parentCount
}

// appendInput records a single (parent hash, vout) into the packed layout,
// preserving deduplication of repeated parent hashes.
//
// On dedup-miss the new parent is appended and contributes 2 entries to
// voutIdxs (count=1 + the vout value), keeping the no-grow guarantee given
// the pre-sized capacity of 2n.
//
// On dedup-hit the existing parent's count is incremented and the vout value
// is inserted into the existing run, shifting any following parents' data
// right by one. Insertion is O(remaining tail length); the total cost across
// a full tx is O(n²) which matches the slices.Index dedup lookup it sits
// alongside, so per-tx cost is unchanged. Typical n is 1-3.
func (p *TxInpoints) appendInput(hash chainhash.Hash, vout uint32) {
	idx := slices.Index(p.ParentTxHashes, hash)
	if idx == -1 {
		p.ParentTxHashes = append(p.ParentTxHashes, hash)
		p.voutIdxs = append(p.voutIdxs, 1, vout)

		return
	}

	// Walk to the count word for parent idx.
	pos := 0
	for j := 0; j < idx; j++ {
		pos += 1 + int(p.voutIdxs[pos])
	}

	count := p.voutIdxs[pos]
	insertAt := pos + 1 + int(count)

	// Grow by one slot at insertAt without reallocating (capacity was
	// pre-sized to 2n).
	p.voutIdxs = append(p.voutIdxs, 0)
	copy(p.voutIdxs[insertAt+1:], p.voutIdxs[insertAt:len(p.voutIdxs)-1])
	p.voutIdxs[insertAt] = vout
	p.voutIdxs[pos] = count + 1
}

// deserializeFromReader reads the TxInpoints data from the provided reader and
// populates the TxInpoints object.
func (p *TxInpoints) deserializeFromReader(buf io.Reader) error {
	var bytesUint32 [4]byte

	if _, err := io.ReadFull(buf, bytesUint32[:]); err != nil {
		return fmt.Errorf("unable to read number of parent inpoints: %w", err)
	}

	parentCount := binary.LittleEndian.Uint32(bytesUint32[:])

	if parentCount == 0 {
		return nil
	}

	p.ParentTxHashes = make([]chainhash.Hash, parentCount)

	for i := uint32(0); i < parentCount; i++ {
		if _, err := io.ReadFull(buf, p.ParentTxHashes[i][:]); err != nil {
			return fmt.Errorf("unable to read parent tx hash: %w", err)
		}
	}

	// Pre-size voutIdxs assuming 1 vout per parent (the common case); growth
	// only happens for parents with multiple vouts.
	p.voutIdxs = make([]uint32, 0, parentCount*2)

	for i := uint32(0); i < parentCount; i++ {
		if _, err := io.ReadFull(buf, bytesUint32[:]); err != nil {
			return fmt.Errorf("unable to read number of parent indexes: %w", err)
		}

		count := binary.LittleEndian.Uint32(bytesUint32[:])
		p.voutIdxs = append(p.voutIdxs, count)

		for j := uint32(0); j < count; j++ {
			if _, err := io.ReadFull(buf, bytesUint32[:]); err != nil {
				return fmt.Errorf("unable to read parent index: %w", err)
			}

			p.voutIdxs = append(p.voutIdxs, binary.LittleEndian.Uint32(bytesUint32[:]))
		}
	}

	return nil
}

// newSizedFromInputs builds a TxInpoints with buffers sized to len(inputs),
// the upper bound on both unique parents and total vouts. This preserves the
// no-grow guarantee the prior cap-8/cap-16 hard-coded constants were aiming
// for, but without paying for unused slack on the typical 1-2-input tx.
func newSizedFromInputs(inputs []*bt.Input) TxInpoints {
	n := len(inputs)
	if n == 0 {
		return TxInpoints{}
	}

	p := TxInpoints{
		// Worst case: every input has a unique parent.
		ParentTxHashes: make([]chainhash.Hash, 0, n),
		// Worst case: every input has a unique parent, each contributing
		// 1 count word + 1 vout word = 2n entries total.
		// Best case: 1 parent with n vouts = 1 + n entries. Same upper bound.
		voutIdxs: make([]uint32, 0, 2*n),
	}

	for _, input := range inputs {
		p.appendInput(*input.PreviousTxIDChainHash(), input.PreviousTxOutIndex)
	}

	return p
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

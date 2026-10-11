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
// size exactly and never re-grow.
//
// NewTxInpoints exists for callers that want an empty placeholder. The
// returned value has a non-nil-but-empty ParentTxHashes — Meta.Serialize
// distinguishes "no inpoints set yet" (nil) from "this node has no parents"
// (empty), and SubtreeMeta consumers depend on the latter shape for
// non-coinbase nodes that legitimately have zero unique parents. The slice
// header points at the runtime's zero-cap sentinel, so no heap allocation
// is incurred.
func NewTxInpoints() TxInpoints {
	return TxInpoints{
		ParentTxHashes: []chainhash.Hash{},
	}
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

// NewTxInpointsFromPacked builds a TxInpoints whose internal storage *aliases*
// the supplied slices. Zero allocation, zero validation: a hot-path
// constructor for trusted callers that already hold the packed layout in a
// large shared buffer (e.g., columnar gRPC requests where the validator has
// pre-arranged the data).
//
// The caller MUST guarantee that:
//
//  1. voutIdxs is laid out as the count-prefixed packed form documented on
//     TxInpoints: for each parent in `parents` order, one count word followed
//     by that many vout-value words, concatenated. Total length =
//     len(parents) + sum(count_i).
//  2. The two slices outlive every reference to the returned TxInpoints
//     (including copies); this constructor does NOT copy.
//
// Malformed input will not be detected here — accessors will read garbage or
// panic with index-out-of-range at use time. Wire-format validation belongs
// in the sender. Callers that need defensive construction from untrusted
// bytes should use NewTxInpointsFromBytes / NewTxInpointsFromReader.
func NewTxInpointsFromPacked(parents []chainhash.Hash, voutIdxs []uint32) TxInpoints {
	return TxInpoints{
		ParentTxHashes: parents,
		voutIdxs:       voutIdxs,
	}
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

// Serialize serializes the TxInpoints object into a newly allocated byte
// slice. The wire format is unchanged from the pre-packed-layout version.
//
// It is SerializedSize followed by AppendSerialized into an exactly sized
// buffer, so the result is always identical to what AppendSerialized appends.
func (p *TxInpoints) Serialize() ([]byte, error) {
	size, err := p.SerializedSize()
	if err != nil {
		return nil, err
	}

	return p.AppendSerialized(make([]byte, 0, size))
}

// SerializedSize returns the exact number of bytes Serialize returns and
// AppendSerialized appends: a 4 byte parent count, 32 bytes per parent hash,
// and 4 bytes per word of the packed vout layout. It does not allocate.
//
// It returns ErrParentTxHashesMismatch for exactly the layouts Serialize
// rejects: no parents with a non-empty vout layout, or fewer vout words than
// parents. Layouts NewTxInpointsFromPacked's contract forbids but Serialize
// accepts (for example trailing extra words) are sized as Serialize writes them.
func (p *TxInpoints) SerializedSize() (int, error) {
	parentCount := len(p.ParentTxHashes)

	// Layout invariant: when parentCount > 0 voutIdxs must hold at least the
	// parentCount count words. An empty TxInpoints requires voutIdxs to also
	// be empty.
	if (parentCount == 0 && len(p.voutIdxs) != 0) ||
		(parentCount > 0 && len(p.voutIdxs) < parentCount) {
		return 0, ErrParentTxHashesMismatch
	}

	// 4 byte header + 32 bytes per parent hash + 4 bytes per voutIdxs entry
	// (both count and value words).
	return 4 + parentCount*chainhash.HashSize + len(p.voutIdxs)*4, nil
}

// AppendSerialized appends the bytes Serialize would return to dst and returns
// the extended slice, in the manner of the standard library's Append*
// functions. dst is grown at most once, to the exact required size, so it does
// not allocate when cap(dst)-len(dst) >= SerializedSize.
//
// On error it returns dst with its original length and contents, and writes
// nothing to it. The errors are those of Serialize.
func (p *TxInpoints) AppendSerialized(dst []byte) ([]byte, error) {
	size, err := p.SerializedSize()
	if err != nil {
		return dst, err
	}

	dst = slices.Grow(dst, size)
	off := len(dst)
	dst = dst[:off+size]
	out := dst[off:]

	binary.LittleEndian.PutUint32(out, len32(p.ParentTxHashes))
	out = out[4:]

	for i := range p.ParentTxHashes {
		copy(out, p.ParentTxHashes[i][:])
		out = out[chainhash.HashSize:]
	}

	// voutIdxs is already laid out as [count_0, vals..., count_1, vals..., ...]
	// — exactly the wire format. Write it one uint32 at a time so we stay
	// endian-correct.
	for _, v := range p.voutIdxs {
		binary.LittleEndian.PutUint32(out, v)
		out = out[4:]
	}

	return dst, nil
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

// inpointsPreallocLimit bounds how many elements a wire-supplied count may
// pre-allocate before any of the body it describes has been read. It is a
// buffer-sizing hint, not a protocol limit: counts above it still deserialize
// correctly, they just grow incrementally instead of in one allocation.
const inpointsPreallocLimit = 1024

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

	// Grow into the count rather than allocating it up front. parentCount comes
	// straight off the wire and nothing here can tell a real one from a corrupt
	// one, so sizing from it hands an attacker — or a single flipped byte in a
	// cache file — a 4-billion-element allocation before a byte of the body has
	// been read. Appending instead means a bogus count costs one small buffer and
	// then fails on EOF, which is what it should have done all along.
	//
	// The cap keeps the common case to a single allocation: real parent counts are
	// typically 1-3, so anything under the limit behaves exactly as before. Larger
	// genuine counts grow by doubling, a few extra allocations on a path that is
	// already reading 32 bytes per parent off a reader.
	p.ParentTxHashes = make([]chainhash.Hash, 0, min(parentCount, inpointsPreallocLimit))

	for i := uint32(0); i < parentCount; i++ {
		var parentTxHash chainhash.Hash

		if _, err := io.ReadFull(buf, parentTxHash[:]); err != nil {
			return fmt.Errorf("unable to read parent tx hash: %w", err)
		}

		p.ParentTxHashes = append(p.ParentTxHashes, parentTxHash)
	}

	// Pre-size voutIdxs assuming 1 vout per parent (the common case); growth
	// only happens for parents with multiple vouts. Bounded like the hashes
	// above, and widened to uint64 first: parentCount*2 in uint32 wraps for any
	// count above 2^31, so the old expression could ask for a huge allocation or
	// silently ask for none at all.
	p.voutIdxs = make([]uint32, 0, min(uint64(parentCount)*2, inpointsPreallocLimit*2))

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

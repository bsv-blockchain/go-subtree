package subtree

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/bsv-blockchain/go-bt/v2/chainhash"
	"github.com/stretchr/testify/require"
)

// referenceSerialize is the Serialize body as it was before SerializedSize and
// AppendSerialized existed. It is the oracle every new encoder path is held to,
// byte for byte and error for error.
func referenceSerialize(p *TxInpoints) ([]byte, error) {
	parentCount := len(p.ParentTxHashes)

	if (parentCount == 0 && len(p.voutIdxs) != 0) ||
		(parentCount > 0 && len(p.voutIdxs) < parentCount) {
		return nil, ErrParentTxHashesMismatch
	}

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

	for _, v := range p.voutIdxs {
		binary.LittleEndian.PutUint32(bytesUint32[:], v)

		if _, err = buf.Write(bytesUint32[:]); err != nil {
			return nil, fmt.Errorf("unable to write parent index: %w", err)
		}
	}

	return buf.Bytes(), nil
}

func testHashes(n int) []chainhash.Hash {
	hs := make([]chainhash.Hash, n)
	for i := range hs {
		hs[i] = chainhash.HashH([]byte(fmt.Sprintf("parent-%d", i)))
	}

	return hs
}

// packedUniform returns the packed layout for n parents with k vouts each.
func packedUniform(n, k int) []uint32 {
	out := make([]uint32, 0, n*(k+1))

	for i := 0; i < n; i++ {
		out = append(out, uint32(k)) //nolint:gosec // test sizes are small

		for v := 0; v < k; v++ {
			out = append(out, uint32(i*1000+v))
		}
	}

	return out
}

// assertMatchesReference checks SerializedSize, Serialize and AppendSerialized
// against the reference encoder for p, including error identity and that dst is
// returned unchanged on error.
func assertMatchesReference(t *testing.T, p TxInpoints) {
	t.Helper()

	want, wantErr := referenceSerialize(&p)

	got, gotErr := p.Serialize()
	require.Equal(t, wantErr, gotErr, "Serialize error")
	require.Equal(t, want, got, "Serialize bytes")

	size, sizeErr := p.SerializedSize()
	require.Equal(t, wantErr, sizeErr, "SerializedSize error")

	if wantErr == nil {
		require.Equal(t, len(want), size, "SerializedSize")
	} else {
		require.Zero(t, size)
	}

	for _, prefix := range [][]byte{nil, {}, []byte("prefix")} {
		// Copy so a bug that scribbles on the caller's prefix is visible.
		dst := append([]byte(nil), prefix...)
		if prefix != nil && dst == nil {
			dst = []byte{}
		}

		out, err := p.AppendSerialized(dst)
		require.Equal(t, wantErr, err, "AppendSerialized error")

		if wantErr != nil {
			require.Len(t, out, len(prefix), "dst length changed on error")
			require.True(t, bytes.Equal(prefix, out), "dst content changed on error")

			continue
		}

		require.Equal(t, append(append([]byte(nil), prefix...), want...), out, "AppendSerialized bytes")
	}
}

func TestTxInpoints_SerializeEquivalence(t *testing.T) {
	hs := testHashes(5000)

	// 70000 vouts for a single parent: count word exceeds 16 bits.
	big := make([]uint32, 0, 70001)
	big = append(big, 70000)

	for i := uint32(0); i < 70000; i++ {
		big = append(big, i)
	}

	// Shared backing buffer: cap extends beyond len into data that must not
	// be serialized.
	backing := []uint32{1, 7, 0xDEADBEEF, 0xCAFEBABE}

	tests := []struct {
		name    string
		parents []chainhash.Hash
		vouts   []uint32
		wantErr error
	}{
		{name: "zero value", parents: nil, vouts: nil},
		{name: "nil parents empty vouts", parents: nil, vouts: []uint32{}},
		{name: "empty parents nil vouts", parents: []chainhash.Hash{}, vouts: nil},
		{name: "empty parents empty vouts", parents: []chainhash.Hash{}, vouts: []uint32{}},
		{name: "one parent one vout", parents: hs[:1], vouts: []uint32{1, 7}},
		{name: "one parent three vouts", parents: hs[:1], vouts: []uint32{3, 0, 4, 0xFFFFFFFF}},
		{name: "two parents", parents: hs[:2], vouts: []uint32{2, 4, 5, 1, 9}},
		{name: "16 parents", parents: hs[:16], vouts: packedUniform(16, 1)},
		{name: "17 parents", parents: hs[:17], vouts: packedUniform(17, 2)},
		{name: "many parents", parents: hs[:300], vouts: packedUniform(300, 3)},
		{name: "thousands of parents", parents: hs, vouts: packedUniform(5000, 1)},
		{name: "zero vouts for a parent", parents: hs[:2], vouts: []uint32{0, 1, 5}},
		{name: "all parents zero vouts", parents: hs[:3], vouts: []uint32{0, 0, 0}},
		{name: "70000 vouts", parents: hs[:1], vouts: big},
		{name: "cap beyond len on shared backing", parents: hs[:1], vouts: backing[:2]},

		// Malformed layouts. Only the two length checks in Serialize are
		// validation; everything else is written verbatim.
		{name: "too few words", parents: hs[:3], vouts: []uint32{1, 7}, wantErr: ErrParentTxHashesMismatch},
		{name: "parents but no vouts", parents: hs[:1], vouts: nil, wantErr: ErrParentTxHashesMismatch},
		{name: "zero parents with vouts", parents: nil, vouts: []uint32{1, 7}, wantErr: ErrParentTxHashesMismatch},
		{name: "empty parents with vouts", parents: []chainhash.Hash{}, vouts: []uint32{0}, wantErr: ErrParentTxHashesMismatch},
		{name: "exactly parentCount words", parents: hs[:2], vouts: []uint32{9, 9}},
		{name: "trailing extra words", parents: hs[:1], vouts: []uint32{1, 7, 11, 12, 13}},
		{name: "count claims more words than present", parents: hs[:1], vouts: []uint32{100, 1}},
		{name: "count max uint32", parents: hs[:1], vouts: []uint32{0xFFFFFFFF}},
		{name: "garbage counts", parents: hs[:2], vouts: []uint32{0xFFFFFFFF, 0xFFFFFFFF, 0, 0}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := NewTxInpointsFromPacked(tc.parents, tc.vouts)

			if tc.wantErr != nil {
				_, err := p.Serialize()
				require.ErrorIs(t, err, tc.wantErr)
			}

			assertMatchesReference(t, p)
		})
	}
}

// TestTxInpoints_AppendSerializedPreservesUnusedCapacity checks that a dst with
// spare capacity is filled in place and that bytes beyond the appended region
// are left alone.
func TestTxInpoints_AppendSerializedPreservesUnusedCapacity(t *testing.T) {
	p := NewTxInpointsFromPacked(testHashes(2), []uint32{2, 4, 5, 1, 9})

	size, err := p.SerializedSize()
	require.NoError(t, err)

	backing := bytes.Repeat([]byte{0xAA}, 3+size+10)
	copy(backing, "abc")

	out, err := p.AppendSerialized(backing[:3])
	require.NoError(t, err)
	require.Len(t, out, 3+size)
	require.Same(t, &backing[0], &out[0], "must reuse dst's backing array when it has room")
	require.Equal(t, bytes.Repeat([]byte{0xAA}, 10), backing[3+size:], "bytes past the result must be untouched")

	want, err := p.Serialize()
	require.NoError(t, err)
	require.Equal(t, want, out[3:])
}

// TestTxInpoints_AppendSerializedGrowsOnce checks a too-small dst is grown to
// the exact required size in one step.
func TestTxInpoints_AppendSerializedGrowsOnce(t *testing.T) {
	p := NewTxInpointsFromPacked(testHashes(3), packedUniform(3, 2))

	out, err := p.AppendSerialized([]byte("xy"))
	require.NoError(t, err)

	size, err := p.SerializedSize()
	require.NoError(t, err)
	require.Len(t, out, 2+size)
	require.Equal(t, []byte("xy"), out[:2])
}

func TestTxInpoints_SerializeRoundTrip(t *testing.T) {
	hs := testHashes(300)

	tests := []struct {
		name    string
		parents []chainhash.Hash
		vouts   []uint32
	}{
		{"empty", nil, nil},
		{"one parent", hs[:1], []uint32{1, 3}},
		{"zero vouts for a parent", hs[:2], []uint32{0, 1, 5}},
		{"multi vouts", hs[:2], []uint32{2, 4, 5, 1, 9}},
		{"many parents", hs, packedUniform(300, 2)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := NewTxInpointsFromPacked(tc.parents, tc.vouts)

			enc, err := p.AppendSerialized(nil)
			require.NoError(t, err)

			got, err := NewTxInpointsFromBytes(enc)
			require.NoError(t, err)

			require.Len(t, got.ParentTxHashes, len(tc.parents))

			for i := range tc.parents {
				require.Equal(t, tc.parents[i], got.ParentTxHashes[i])
			}

			require.Len(t, got.voutIdxs, len(tc.vouts))

			for i := range tc.vouts {
				require.Equal(t, tc.vouts[i], got.voutIdxs[i])
			}

			again, err := got.AppendSerialized(nil)
			require.NoError(t, err)
			require.Equal(t, enc, again)
		})
	}
}

func TestTxInpoints_SerializeAllocations(t *testing.T) {
	p := NewTxInpointsFromPacked(testHashes(16), packedUniform(16, 3))

	size, err := p.SerializedSize()
	require.NoError(t, err)

	dst := make([]byte, 0, size)

	require.Zero(t, testing.AllocsPerRun(100, func() {
		_, _ = p.SerializedSize()
	}), "SerializedSize must not allocate")

	require.Zero(t, testing.AllocsPerRun(100, func() {
		_, _ = p.AppendSerialized(dst[:0])
	}), "AppendSerialized with enough capacity must not allocate")

	bad := NewTxInpointsFromPacked(testHashes(3), []uint32{1})

	require.Zero(t, testing.AllocsPerRun(100, func() {
		_, _ = bad.SerializedSize()
		_, _ = bad.AppendSerialized(dst[:0])
	}), "error paths must not allocate")

	require.InDelta(t, 1, testing.AllocsPerRun(100, func() {
		_, _ = p.Serialize()
	}), 0, "Serialize allocates exactly the result")
}

// FuzzTxInpointsSerialize holds all three entry points to the reference
// encoder on arbitrary parent counts and packed words, valid or not.
func FuzzTxInpointsSerialize(f *testing.F) {
	f.Add([]byte{}, []byte{})
	f.Add([]byte{1}, []byte{1, 0, 0, 0, 7, 0, 0, 0})
	f.Add([]byte{0}, []byte{1, 0, 0, 0})
	f.Add([]byte{3}, []byte{1, 0, 0, 0})
	f.Add([]byte{2}, []byte{2, 0, 0, 0, 4, 0, 0, 0, 5, 0, 0, 0, 1, 0, 0, 0, 9, 0, 0, 0})
	f.Add([]byte{0x80, 1}, []byte("prefix and trailing bytes that are not a multiple of four"))

	f.Fuzz(func(t *testing.T, header, words []byte) {
		// header[0]: parent count (0..127); high bit selects nil vs empty
		// parents when the count is zero. header[1:] is the dst prefix.
		var (
			parentCount int
			prefix      []byte
			nilParents  bool
		)

		if len(header) > 0 {
			parentCount = int(header[0] & 0x7F)
			nilParents = header[0]&0x80 != 0
			prefix = header[1:]
		}

		var parents []chainhash.Hash

		if parentCount > 0 || !nilParents {
			parents = make([]chainhash.Hash, parentCount)
			for i := range parents {
				parents[i] = chainhash.HashH([]byte{byte(i)})
			}
		}

		vouts := make([]uint32, len(words)/4)
		for i := range vouts {
			vouts[i] = binary.LittleEndian.Uint32(words[i*4:])
		}

		p := NewTxInpointsFromPacked(parents, vouts)

		want, wantErr := referenceSerialize(&p)

		got, gotErr := p.Serialize()
		require.Equal(t, wantErr, gotErr)
		require.Equal(t, want, got)

		size, sizeErr := p.SerializedSize()
		require.Equal(t, wantErr, sizeErr)

		dst := append([]byte(nil), prefix...)
		out, err := p.AppendSerialized(dst)
		require.Equal(t, wantErr, err)

		if wantErr != nil {
			require.True(t, bytes.Equal(prefix, out))

			return
		}

		require.Equal(t, len(want), size)
		require.Equal(t, append(append([]byte(nil), prefix...), want...), out)
	})
}

// TestMeta_SerializeMatchesPerNodeEncoding pins Meta.Serialize's layout (root
// hash, node count, then each node's TxInpoints encoding) against the
// reference encoder. The payload is far larger than Meta's initial buffer so
// the grow path is exercised as well as the in-place one.
func TestMeta_SerializeMatchesPerNodeEncoding(t *testing.T) {
	const nodes = 64

	st, err := NewTreeByLeafCount(nodes)
	require.NoError(t, err)

	for i := 0; i < nodes; i++ {
		require.NoError(t, st.AddNode(chainhash.HashH([]byte(fmt.Sprintf("tx-%d", i))), 1, 1))
	}

	meta := NewSubtreeMeta(st)
	hs := testHashes(200)

	for i := 0; i < nodes; i++ {
		n := 1 + i*3 // varying parent counts, including a single parent
		require.NoError(t, meta.SetTxInpoints(i, NewTxInpointsFromPacked(hs[:n], packedUniform(n, 1+i%4))))
	}

	got, err := meta.Serialize()
	require.NoError(t, err)

	root := st.RootHash()
	want := append([]byte(nil), root[:]...)
	want = binary.LittleEndian.AppendUint32(want, nodes)

	for i := 0; i < nodes; i++ {
		enc, encErr := referenceSerialize(&meta.TxInpoints[i])
		require.NoError(t, encErr)

		want = append(want, enc...)
	}

	require.Greater(t, len(want), 32*1024)
	require.Equal(t, want, got)
}

func TestMeta_SerializeRejectsMalformedNode(t *testing.T) {
	st, err := NewTreeByLeafCount(2)
	require.NoError(t, err)
	require.NoError(t, st.AddNode(chainhash.HashH([]byte("a")), 1, 1))
	require.NoError(t, st.AddNode(chainhash.HashH([]byte("b")), 1, 1))

	meta := NewSubtreeMeta(st)
	require.NoError(t, meta.SetTxInpoints(0, NewTxInpoints()))
	require.NoError(t, meta.SetTxInpoints(1, NewTxInpointsFromPacked(testHashes(3), []uint32{1, 7})))

	_, err = meta.Serialize()
	require.ErrorIs(t, err, ErrParentTxHashesMismatch)
}

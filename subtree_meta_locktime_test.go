package subtree

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"io"
	"math"
	"testing"

	"github.com/bsv-blockchain/go-bt/v2/chainhash"
	"github.com/stretchr/testify/require"
)

func lockTimeTestMeta(t testing.TB) *Meta {
	t.Helper()
	st, err := NewTreeByLeafCount(4)
	require.NoError(t, err)
	for i := byte(1); i <= 3; i++ {
		require.NoError(t, st.AddNode(chainhash.Hash{i}, 1, 1))
	}
	m := NewSubtreeMeta(st)
	for i := range st.Nodes {
		require.NoError(t, m.SetTxInpoints(i, NewTxInpointsFromPacked(
			[]chainhash.Hash{{9}}, []uint32{1, 0})))
	}
	return m
}

func lockTimeTestTrailer(entries []TxLockTime) []byte {
	b := binary.LittleEndian.AppendUint32([]byte("SLT1"), len32(entries))
	for _, entry := range entries {
		b = binary.LittleEndian.AppendUint32(b, entry.Index)
		b = binary.LittleEndian.AppendUint32(b, entry.LockTime)
		flag := byte(0)
		if entry.Enabled {
			flag = 1
		}
		b = append(b, flag)
	}
	return b
}

func TestMetaLockTimesLegacyFixture(t *testing.T) {
	st, err := NewTreeByLeafCount(1)
	require.NoError(t, err)
	require.NoError(t, st.AddNode(chainhash.Hash{1}, 1, 1))
	legacy, err := hex.DecodeString("01000000000000000000000000000000000000000000000000000000000000000100000000000000")
	require.NoError(t, err)
	m := NewSubtreeMeta(st)
	got, err := m.Serialize()
	require.NoError(t, err)
	require.Equal(t, legacy, got)
	decoded, err := NewSubtreeMetaFromBytesWithLockTimes(st, legacy)
	require.NoError(t, err)
	require.Nil(t, decoded.LockTimes)
	got, err = decoded.Serialize()
	require.NoError(t, err)
	require.Equal(t, legacy, got)
}

func TestMetaLockTimesRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []TxLockTime
	}{
		{"unavailable", nil},
		{"known zero", []TxLockTime{}},
		{"sparse", []TxLockTime{{Index: 1, LockTime: 800000, Enabled: true}}},
		{"mixed", []TxLockTime{
			{Index: 0, LockTime: 499999999, Enabled: true},
			{Index: 1, LockTime: 500000000, Enabled: true},
			{Index: 2, LockTime: math.MaxUint32, Enabled: false},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := lockTimeTestMeta(t)
			legacy, err := m.Serialize()
			require.NoError(t, err)
			m.LockTimes = tc.entries
			wire, err := m.Serialize()
			require.NoError(t, err)
			want := bytes.Clone(legacy)
			if tc.entries != nil {
				want = append(want, lockTimeTestTrailer(tc.entries)...)
			}
			require.Equal(t, want, wire)
			require.Equal(t, len(wire), cap(wire))
			for _, fromReader := range []bool{false, true} {
				var decoded *Meta
				if fromReader {
					decoded, err = NewSubtreeMetaFromReaderWithLockTimes(m.Subtree, bytes.NewReader(wire))
				} else {
					decoded, err = NewSubtreeMetaFromBytesWithLockTimes(m.Subtree, wire)
				}
				require.NoError(t, err)
				require.Equal(t, tc.entries, decoded.LockTimes)
				require.Equal(t, m.TxInpoints, decoded.TxInpoints)
				again, serializeErr := decoded.Serialize()
				require.NoError(t, serializeErr)
				require.Equal(t, wire, again)
			}
			// Both original entrypoints still use the legacy decoder, leaving
			// extensions unavailable and dropping them on reserialization.
			r := bytes.NewReader(wire)
			old, err := NewSubtreeMetaFromReader(m.Subtree, r)
			require.NoError(t, err)
			require.Nil(t, old.LockTimes)
			require.Equal(t, len(wire)-len(legacy), r.Len())
			again, err := old.Serialize()
			require.NoError(t, err)
			require.Equal(t, legacy, again)
			old, err = NewSubtreeMetaFromBytes(m.Subtree, wire)
			require.NoError(t, err)
			require.Nil(t, old.LockTimes)
		})
	}
}

func TestMetaLockTimesRejectInvalidEntries(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []TxLockTime
	}{
		{"zero", []TxLockTime{{Index: 0, LockTime: 0}}},
		{"index at length", []TxLockTime{{Index: 3, LockTime: 1}}},
		{"index at capacity", []TxLockTime{{Index: 4, LockTime: 1}}},
		{"duplicate", []TxLockTime{{Index: 1, LockTime: 1}, {Index: 1, LockTime: 2}}},
		{"unsorted", []TxLockTime{{Index: 2, LockTime: 1}, {Index: 0, LockTime: 2}}},
		{"too many", []TxLockTime{{LockTime: 1}, {LockTime: 1}, {LockTime: 1}, {LockTime: 1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := lockTimeTestMeta(t)
			legacy, err := m.Serialize()
			require.NoError(t, err)
			m.LockTimes = tc.entries
			_, err = m.Serialize()
			require.Error(t, err)
			_, err = NewSubtreeMetaFromBytesWithLockTimes(m.Subtree, append(legacy, lockTimeTestTrailer(tc.entries)...))
			require.Error(t, err)
		})
	}
}

func TestMetaLockTimesRejectMalformedBlobs(t *testing.T) {
	m := lockTimeTestMeta(t)
	legacy, err := m.Serialize()
	require.NoError(t, err)
	valid := append(bytes.Clone(legacy), lockTimeTestTrailer([]TxLockTime{{Index: 1, LockTime: 42, Enabled: true}})...)
	for n := 0; n < len(valid); n++ {
		if n == len(legacy) {
			continue // An exact legacy blob is valid, with unavailable facts.
		}
		_, readErr := NewSubtreeMetaFromBytesWithLockTimes(m.Subtree, valid[:n])
		require.Error(t, readErr, "truncation at %d", n)
	}
	for _, tc := range []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"foreign root", func(b []byte) []byte { b[0] ^= 1; return b }},
		{"wrong node count", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[32:], 2); return b }},
		{"huge node count", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[32:], math.MaxUint32); return b }},
		{"unknown version", func(b []byte) []byte { b[len(legacy)+3] = '2'; return b }},
		{"unknown flags", func(b []byte) []byte { b[len(b)-1] = 2; return b }},
		{"huge exception count", func(b []byte) []byte { binary.LittleEndian.PutUint32(b[len(legacy)+4:], math.MaxUint32); return b }},
		{"surplus", func(b []byte) []byte { return append(b, 0) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, readErr := NewSubtreeMetaFromBytesWithLockTimes(m.Subtree, tc.mutate(bytes.Clone(valid)))
			require.Error(t, readErr)
		})
	}
	_, err = NewSubtreeMetaFromBytesWithLockTimes(nil, valid)
	require.Error(t, err)
}

func TestMetaLockTimesFramedReader(t *testing.T) {
	m := lockTimeTestMeta(t)
	m.LockTimes = []TxLockTime{{Index: 2, LockTime: 42, Enabled: true}}
	wire, err := m.Serialize()
	require.NoError(t, err)
	next := []byte("next frame")
	r := bytes.NewReader(append(bytes.Clone(wire), next...))
	frame := &io.LimitedReader{R: r, N: int64(len(wire))}
	got, err := NewSubtreeMetaFromReaderWithLockTimes(m.Subtree, frame)
	require.NoError(t, err)
	require.Zero(t, frame.N)
	require.Equal(t, m.LockTimes, got.LockTimes)
	require.Equal(t, len(next), r.Len())

	// The producer deliberately does not close the connection after the frame.
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pr.Close(); _ = pw.Close() })
	go func() { _, _ = pw.Write(wire) }()
	got, err = NewSubtreeMetaFromReaderWithLockTimes(m.Subtree, io.LimitReader(pr, int64(len(wire))))
	require.NoError(t, err)
	require.Equal(t, m.LockTimes, got.LockTimes)
}

type lockTimeErrorReader struct {
	err error
}

func (r lockTimeErrorReader) Read([]byte) (int, error) { return 0, r.err }

func TestMetaLockTimesReaderErrors(t *testing.T) {
	m := lockTimeTestMeta(t)
	legacy, err := m.Serialize()
	require.NoError(t, err)
	_, err = NewSubtreeMetaFromReaderWithLockTimes(m.Subtree, nil)
	require.ErrorIs(t, err, ErrInvalidLockTimeMetadata)
	empty, err := NewTreeByLeafCount(1)
	require.NoError(t, err)
	_, err = NewSubtreeMetaFromBytesWithLockTimes(empty, legacy)
	require.ErrorIs(t, err, ErrInvalidLockTimeMetadata)
	for _, prefix := range [][]byte{
		nil,
		legacy[:36],
		legacy,
		append(bytes.Clone(legacy), lockTimeTestTrailer([]TxLockTime{{Index: 1, LockTime: 1}})[:8]...),
		append(bytes.Clone(legacy), lockTimeTestTrailer([]TxLockTime{})...),
	} {
		r := io.MultiReader(bytes.NewReader(prefix), lockTimeErrorReader{io.ErrClosedPipe})
		_, err := NewSubtreeMetaFromReaderWithLockTimes(m.Subtree, r)
		require.ErrorIs(t, err, io.ErrClosedPipe)
	}
}

func TestMetaLockTimesKnownZeroAllocations(t *testing.T) {
	m := lockTimeTestMeta(t)
	legacy, err := m.Serialize()
	require.NoError(t, err)
	m.LockTimes = []TxLockTime{}
	knownZero, err := m.Serialize()
	require.NoError(t, err)
	serializeAllocs := func(entries []TxLockTime) float64 {
		m.LockTimes = entries
		return testing.AllocsPerRun(50, func() {
			if _, err := m.Serialize(); err != nil {
				panic(err)
			}
		})
	}
	decodeAllocs := func(wire []byte) float64 {
		return testing.AllocsPerRun(50, func() {
			if _, err := NewSubtreeMetaFromBytesWithLockTimes(m.Subtree, wire); err != nil {
				panic(err)
			}
		})
	}
	require.InDelta(t, serializeAllocs(nil), serializeAllocs([]TxLockTime{}), 0)
	require.InDelta(t, decodeAllocs(legacy), decodeAllocs(knownZero), 0)
}

func FuzzMetaLockTimes(f *testing.F) {
	m := lockTimeTestMeta(f)
	legacy, err := m.Serialize()
	require.NoError(f, err)
	m.LockTimes = []TxLockTime{{Index: 1, LockTime: 800000, Enabled: true}}
	wire, err := m.Serialize()
	require.NoError(f, err)
	// Fuzz only the trailer so malformed inpoints cannot dominate this target.
	f.Add([]byte{})
	f.Add(lockTimeTestTrailer([]TxLockTime{}))
	f.Add(wire[len(legacy):])
	f.Fuzz(func(t *testing.T, trailer []byte) {
		blob := append(bytes.Clone(legacy), trailer...)
		got, err := NewSubtreeMetaFromBytesWithLockTimes(m.Subtree, blob)
		if err != nil {
			return
		}
		again, err := got.Serialize()
		require.NoError(t, err)
		require.Equal(t, blob, again)
	})
}

func BenchmarkMetaLockTimes(b *testing.B) {
	for _, tc := range []struct {
		name    string
		entries []TxLockTime
	}{
		{"absent", nil},
		{"known zero", []TxLockTime{}},
		{"sparse", []TxLockTime{{Index: 1, LockTime: 42, Enabled: true}, {Index: 500, LockTime: 500000000}}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			m := buildLargeMeta(b, 1<<16)
			m.LockTimes = tc.entries
			wire, err := m.Serialize()
			require.NoError(b, err)
			b.Run("serialize", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := m.Serialize(); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("decode", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := NewSubtreeMetaFromBytesWithLockTimes(m.Subtree, wire); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

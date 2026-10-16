package subtree

import (
	"fmt"
	"testing"
)

// BenchmarkTxInpointsSerialize measures Serialize across tx shapes. The
// shapes are built with benchInpoints so before/after runs are comparable.
func BenchmarkTxInpointsSerialize(b *testing.B) {
	for _, shape := range benchShapes {
		p := benchInpoints(shape.parents, shape.vouts)

		b.Run(shape.name, func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				if _, err := p.Serialize(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

var benchShapes = []struct {
	name           string
	parents, vouts int
}{
	{"1parent_1vout", 1, 1},
	{"2parents_2vouts", 2, 2},
	{"16parents_1vout", 16, 1},
	{"1000parents_1vout", 1000, 1},
}

func benchInpoints(parents, voutsPerParent int) TxInpoints {
	p := NewTxInpoints()

	for i := 0; i < parents; i++ {
		var h [32]byte

		copy(h[:], fmt.Sprintf("parent-%d", i))

		for v := 0; v < voutsPerParent; v++ {
			p.appendInput(h, uint32(v))
		}
	}

	return p
}

// BenchmarkTxInpointsAppendSerialized measures AppendSerialized into a reused
// destination buffer, the way a caller encoding many nodes would use it.
func BenchmarkTxInpointsAppendSerialized(b *testing.B) {
	for _, shape := range benchShapes {
		p := benchInpoints(shape.parents, shape.vouts)

		size, err := p.SerializedSize()
		if err != nil {
			b.Fatal(err)
		}

		dst := make([]byte, 0, size)

		b.Run(shape.name, func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				if _, err := p.AppendSerialized(dst[:0]); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkTxInpointsGetTxInpoints measures GetTxInpoints, which allocates the
// result for every call.
func BenchmarkTxInpointsGetTxInpoints(b *testing.B) {
	for _, shape := range benchShapes {
		p := benchInpoints(shape.parents, shape.vouts)

		b.Run(shape.name, func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = p.GetTxInpoints()
			}
		})
	}
}

// BenchmarkTxInpointsAppendTxInpoints measures AppendTxInpoints into a reused
// destination buffer, the way a caller walking many txs would use it.
func BenchmarkTxInpointsAppendTxInpoints(b *testing.B) {
	for _, shape := range benchShapes {
		p := benchInpoints(shape.parents, shape.vouts)

		b.Run(shape.name, func(b *testing.B) {
			b.ReportAllocs()

			var buf []Inpoint

			for i := 0; i < b.N; i++ {
				buf = p.AppendTxInpoints(buf[:0])
			}
		})
	}
}

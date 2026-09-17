package subtree

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// FuzzNewSubtreeFromReaderMmap is the broad safety net for S-1: no serialized
// bytes, however malformed, may escape the mmap loader as a panic. A returned
// error or a valid subtree is fine; a panic is the process crash the audit found.
//
// Inputs whose declared count would map a huge (sparse) scratch file are skipped:
// they exercise the OS allocator rather than this parser and slow the fuzzer to a
// crawl. The bound and overflow handling for large counts are covered exhaustively
// by the table tests in mmap_store_test.go instead.
func FuzzNewSubtreeFromReaderMmap(f *testing.F) {
	auditHeader := make([]byte, numLeavesOffset+8)
	binary.LittleEndian.PutUint64(auditHeader[numLeavesOffset:], auditHostileCount)
	f.Add(auditHeader)

	maxHeader := make([]byte, numLeavesOffset+8)
	binary.LittleEndian.PutUint64(maxHeader[numLeavesOffset:], math.MaxUint64)
	f.Add(maxHeader)

	f.Add([]byte{})
	f.Add(make([]byte, numLeavesOffset+8)) // count == 0

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) >= numLeavesOffset+8 {
			if count := binary.LittleEndian.Uint64(data[numLeavesOffset:]); count > 4096 {
				return
			}
		}

		st, err := NewSubtreeFromReaderMmap(bytes.NewReader(data), t.TempDir())
		if err == nil && st != nil {
			_ = st.Close()
		}
	})
}

// FuzzCeilPowerOfTwo tests the CeilPowerOfTwo function with fuzzing
func FuzzCeilPowerOfTwo(f *testing.F) {
	// Add seed corpus
	f.Add(0)
	f.Add(1)
	f.Add(2)
	f.Add(7)
	f.Add(8)
	f.Add(15)
	f.Add(16)
	f.Add(31)
	f.Add(32)
	f.Add(1024)
	f.Add(-1)
	f.Add(-10)

	f.Fuzz(func(t *testing.T, num int) {
		result := CeilPowerOfTwo(num)

		// Property 1: Result should always be >= 1
		if result < 1 {
			t.Errorf("CeilPowerOfTwo(%d) = %d, want >= 1", num, result)
		}

		// Property 2: Result should always be a power of 2
		if !IsPowerOfTwo(result) {
			t.Errorf("CeilPowerOfTwo(%d) = %d, which is not a power of 2", num, result)
		}

		// Property 3: For positive numbers, result should be >= input
		if num > 0 && result < num {
			t.Errorf("CeilPowerOfTwo(%d) = %d, want >= %d", num, result, num)
		}

		// Property 4: For positive numbers, result/2 should be < input (unless input is a power of 2)
		if num > 1 && !IsPowerOfTwo(num) && result/2 >= num {
			t.Errorf("CeilPowerOfTwo(%d) = %d, but %d/2 = %d >= %d", num, result, result, result/2, num)
		}
	})
}

// FuzzNextPowerOfTwo tests the NextPowerOfTwo function with fuzzing
func FuzzNextPowerOfTwo(f *testing.F) {
	// Add seed corpus
	f.Add(0)
	f.Add(1)
	f.Add(2)
	f.Add(7)
	f.Add(8)
	f.Add(15)
	f.Add(16)
	f.Add(31)
	f.Add(32)
	f.Add(1024)
	f.Add(1073741823) // 2^30 - 1

	f.Fuzz(func(t *testing.T, n int) {
		// Skip negative numbers as the function behavior is undefined for them
		if n <= 0 {
			t.Skip()
		}

		result := NextPowerOfTwo(n)

		// Property 1: Result should be a power of 2
		if !IsPowerOfTwo(result) {
			t.Errorf("NextPowerOfTwo(%d) = %d, which is not a power of 2", n, result)
		}

		// Property 2: Result should be >= n
		if result < n {
			t.Errorf("NextPowerOfTwo(%d) = %d, want >= %d", n, result, n)
		}

		// Property 3: If n is already a power of 2, result should equal n
		if IsPowerOfTwo(n) && result != n {
			t.Errorf("NextPowerOfTwo(%d) = %d, want %d (input is already power of 2)", n, result, n)
		}

		// Property 4: If n is not a power of 2, result should be the smallest power of 2 > n
		if !IsPowerOfTwo(n) && result/2 >= n {
			t.Errorf("NextPowerOfTwo(%d) = %d, but %d/2 = %d >= %d", n, result, result, result/2, n)
		}
	})
}

// FuzzNextLowerPowerOfTwo tests the NextLowerPowerOfTwo function with fuzzing
func FuzzNextLowerPowerOfTwo(f *testing.F) {
	// Add seed corpus
	f.Add(uint(0))
	f.Add(uint(1))
	f.Add(uint(2))
	f.Add(uint(7))
	f.Add(uint(8))
	f.Add(uint(15))
	f.Add(uint(16))
	f.Add(uint(31))
	f.Add(uint(32))
	f.Add(uint(1024))
	f.Add(uint(4294967295)) // Max uint32

	f.Fuzz(func(t *testing.T, x uint) {
		result := NextLowerPowerOfTwo(x)

		// Property 1: For x = 0, result should be 0
		if x == 0 && result != 0 {
			t.Errorf("NextLowerPowerOfTwo(0) = %d, want 0", result)
		}

		// Property 2: For x > 0, result should be a power of 2
		if x > 0 && !IsPowerOfTwo(int(result)) {
			t.Errorf("NextLowerPowerOfTwo(%d) = %d, which is not a power of 2", x, result)
		}

		// Property 3: Result should be <= x
		if result > x {
			t.Errorf("NextLowerPowerOfTwo(%d) = %d, want <= %d", x, result, x)
		}

		// Property 4: For x > 1, result*2 should be > x (unless x is a power of 2)
		if x > 1 && !IsPowerOfTwo(int(x)) && result*2 <= x {
			t.Errorf("NextLowerPowerOfTwo(%d) = %d, but %d*2 = %d <= %d", x, result, result, result*2, x)
		}

		// Property 5: If x is a power of 2, result should equal x
		if x > 0 && IsPowerOfTwo(int(x)) && result != x {
			t.Errorf("NextLowerPowerOfTwo(%d) = %d, want %d (input is power of 2)", x, result, x)
		}
	})
}

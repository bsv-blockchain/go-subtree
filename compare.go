package subtree

import "cmp"

// Min returns the minimum of two ordered values
func Min[T cmp.Ordered](a, b T) T {
	if a < b {
		return a
	}

	return b
}

// Max returns the maximum of two ordered values
func Max[T cmp.Ordered](a, b T) T {
	if a > b {
		return a
	}

	return b
}

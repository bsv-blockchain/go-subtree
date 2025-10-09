package subtree

import "golang.org/x/exp/constraints"

// Min returns the minimum of two ordered values
func Min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}

	return b
}

// Max returns the maximum of two ordered values
func Max[T constraints.Ordered](a, b T) T {
	if a > b {
		return a
	}

	return b
}

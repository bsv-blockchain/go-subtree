package subtree

import "errors"

// Sentinel errors for the subtree package

// Range and bounds errors
var (
	// ErrIndexOutOfRange is returned when an index is out of range
	ErrIndexOutOfRange = errors.New("index out of range")

	// ErrTxIndexOutOfBounds is returned when a transaction index is out of bounds
	ErrTxIndexOutOfBounds = errors.New("transaction index out of bounds")
)

// Validation errors
var (
	// ErrHeightNegative is returned when height is negative
	ErrHeightNegative = errors.New("height must be at least 0")

	// ErrNotPowerOfTwo is returned when the number of leaves must be a power of two
	ErrNotPowerOfTwo = errors.New("numberOfLeaves must be a power of two")

	// ErrSubtreeFull is returned when trying to add a node to a full subtree
	ErrSubtreeFull = errors.New("subtree is full")

	// ErrSubtreeNil is returned when the subtree is nil
	ErrSubtreeNil = errors.New("subtree is nil")

	// ErrSubtreeNotEmpty is returned when subtree should be empty before adding a coinbase node
	ErrSubtreeNotEmpty = errors.New("subtree should be empty before adding a coinbase node")

	// ErrSubtreeNodesEmpty is returned when subtree nodes slice is empty
	ErrSubtreeNodesEmpty = errors.New("subtree nodes slice is empty")

	// ErrNoSubtreesAvailable is returned when no subtrees are available
	ErrNoSubtreesAvailable = errors.New("no subtrees available")

	// ErrCoinbasePlaceholderMisuse is returned when coinbase placeholder node should be added with AddCoinbaseNode
	ErrCoinbasePlaceholderMisuse = errors.New("coinbase placeholder node should be added with AddCoinbaseNode")

	// ErrConflictingNodeNotInSubtree is returned when conflicting node is not in the subtree
	ErrConflictingNodeNotInSubtree = errors.New("conflicting node is not in the subtree")

	// ErrNodeNotFound is returned when a node is not found
	ErrNodeNotFound = errors.New("node not found")
)

// Data mismatch errors
var (
	// ErrParentTxHashesMismatch is returned when parent tx hashes and indexes length mismatch
	ErrParentTxHashesMismatch = errors.New("parent tx hashes and indexes length mismatch")

	// ErrTxHashMismatch is returned when transaction hash does not match subtree node hash
	ErrTxHashMismatch = errors.New("transaction hash does not match subtree node hash")

	// ErrSubtreeLengthMismatch is returned when subtree length does not match tx data length
	ErrSubtreeLengthMismatch = errors.New("subtree length does not match tx data length")
)

// Serialization errors
var (
	// ErrCannotSerializeSubtreeNotSet is returned when cannot serialize because subtree is not set
	ErrCannotSerializeSubtreeNotSet = errors.New("cannot serialize, subtree is not set")

	// ErrReadError is a generic read error for testing
	ErrReadError = errors.New("read error")
)

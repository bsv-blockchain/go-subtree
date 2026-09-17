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

	// ErrTargetHeightTooSmall is returned when RootHashPadded is asked to lift a
	// subtree to a height that is smaller than the height implied by its current
	// leaf count.
	ErrTargetHeightTooSmall = errors.New("target height is smaller than the subtree's actual height")
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

	// ErrTransactionNil is returned when a transaction is nil during serialization
	ErrTransactionNil = errors.New("transaction is nil, cannot serialize")

	// ErrTransactionWrite is returned when writing a transaction fails
	ErrTransactionWrite = errors.New("error writing transaction")

	// ErrTransactionRead is returned when reading a transaction fails
	ErrTransactionRead = errors.New("error reading transaction")

	// ErrNumLeavesOutOfRange is returned when a serialized leaf count would
	// overflow the byte offsets computed from it
	ErrNumLeavesOutOfRange = errors.New("number of leaves is out of range")

	// ErrConflictingCountExceedsLeaves is returned when a serialized subtree
	// claims more conflicting nodes than it has leaves, which no well-formed
	// subtree can, since conflicting nodes are a deduplicated subset of the leaves
	ErrConflictingCountExceedsLeaves = errors.New("conflicting node count exceeds number of leaves")

	// ErrSeekerNotReader is returned when an io.Seeker does not also implement io.Reader
	ErrSeekerNotReader = errors.New("seeker does not implement io.Reader")
)

// Mmap errors
var (
	// ErrCapacityNotPositive is returned when mmap capacity is not positive
	ErrCapacityNotPositive = errors.New("capacity must be positive")

	// ErrCapacityTooLarge is returned when the requested mmap capacity would
	// overflow the arithmetic used to map it: capacity * nodeSize (the mapped
	// byte size) and the unsafe.Slice length. Bounded by math.MaxInt/nodeSize.
	ErrCapacityTooLarge = errors.New("mmap capacity too large")

	// ErrMmapPanic is returned when an unsafe or mmap operation panics. The
	// panic is recovered and converted into an ordinary error rather than being
	// allowed to escape into the caller's process.
	ErrMmapPanic = errors.New("mmap operation panicked")

	// ErrNodeCountExceedsInput is returned when a serialized subtree declares more
	// nodes than the reader could possibly contain. It stops a hostile count from
	// driving a huge scratch-file mmap before a single node has been read, when the
	// reader's remaining size is knowable.
	ErrNodeCountExceedsInput = errors.New("declared node count exceeds reader size")
)

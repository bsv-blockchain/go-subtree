package subtree

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

const (
	lockTimeMagic      = "SLT1"
	lockTimeHeaderSize = 8
	lockTimeRecordSize = 9
)

// TxLockTime preserves a transaction's raw nonzero nLockTime. Index is its
// position in the subtree's Nodes. Enabled is true when at least one input's
// nSequence is not 0xffffffff; false preserves a nonzero but disabled locktime.
//
// An enabled value below 500000000 requires a strictly greater candidate block
// height; other values require a strictly greater consensus comparison time.
// Callers select that time for the candidate block's consensus era and branch.
// These fields do not describe script locks or relative sequence locks.
type TxLockTime struct {
	Index    uint32
	LockTime uint32
	Enabled  bool
}

// NewSubtreeMetaFromBytesWithLockTimes reads a complete metadata blob, including
// its optional locktime trailer. Legacy blobs yield nil LockTimes. Unlike
// NewSubtreeMetaFromBytes, it validates the root and node count against subtree
// and rejects unrecognized or surplus trailing data.
//
// The trailer's completeness and values must be verified from transaction data
// before using them for consensus decisions; matching the root authenticates
// neither. In particular, a peer-supplied empty table is not proof of finality.
func NewSubtreeMetaFromBytesWithLockTimes(subtree *Subtree, data []byte) (*Meta, error) {
	return NewSubtreeMetaFromReaderWithLockTimes(subtree, bytes.NewReader(data))
}

// NewSubtreeMetaFromReaderWithLockTimes reads one finite, EOF-delimited metadata
// blob without buffering the entire blob. It has the same validation and trust
// requirements as NewSubtreeMetaFromBytesWithLockTimes. Legacy blobs yield nil
// LockTimes; a present empty trailer yields a non-nil empty slice.
//
// Unlike NewSubtreeMetaFromReader, this function reads through the blob's EOF.
// For a framed transport, limit reader to exactly one frame before adding any
// buffering. If an exact length is promised, retain an *io.LimitedReader and
// check its N is zero after success to detect a short underlying stream.
func NewSubtreeMetaFromReaderWithLockTimes(subtree *Subtree, reader io.Reader) (*Meta, error) {
	root := subtree.RootHash()
	if root == nil || reader == nil {
		return nil, fmt.Errorf("subtree root and reader must be set: %w", ErrInvalidLockTimeMetadata)
	}

	var header [36]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, fmt.Errorf("unable to read subtree meta header: %w", err)
	}

	count := binary.LittleEndian.Uint32(header[32:])
	if !bytes.Equal(root[:], header[:32]) || int64(count) != int64(subtree.Length()) {
		return nil, fmt.Errorf("subtree meta root or node count mismatch: %w", ErrInvalidLockTimeMetadata)
	}

	s := &Meta{Subtree: subtree, TxInpoints: make([]TxInpoints, subtree.Size())}
	copy(s.rootHash[:], header[:32])
	if err := s.deserializeTxInpointsFromReader(reader, count); err != nil {
		return nil, err
	}
	if err := s.deserializeLockTimes(reader, count); err != nil {
		return nil, err
	}

	return s, nil
}

func (entry TxLockTime) validate(nodeCount int, previousIndex int64) error {
	if entry.LockTime == 0 || int64(entry.Index) >= int64(nodeCount) || int64(entry.Index) <= previousIndex {
		return fmt.Errorf("nonzero values and increasing unique indexes below node count required (index %d): %w", entry.Index, ErrInvalidLockTimeMetadata)
	}

	return nil
}

func (s *Meta) lockTimeSerializedSize(nodeCount int) (int, error) {
	if s.LockTimes == nil {
		return 0, nil
	}

	count := len(s.LockTimes)
	if count > nodeCount || uint64(count) > math.MaxUint32 || count > (math.MaxInt-lockTimeHeaderSize)/lockTimeRecordSize {
		return 0, fmt.Errorf("locktime count exceeds node count or serialized size limit: %w", ErrInvalidLockTimeMetadata)
	}

	previousIndex := int64(-1)
	for _, entry := range s.LockTimes {
		if err := entry.validate(nodeCount, previousIndex); err != nil {
			return 0, err
		}
		previousIndex = int64(entry.Index)
	}

	return lockTimeHeaderSize + count*lockTimeRecordSize, nil
}

func (s *Meta) serializeLockTimes(buf *bytes.Buffer) error {
	out := append(buf.AvailableBuffer(), lockTimeMagic...)
	out = binary.LittleEndian.AppendUint32(out, len32(s.LockTimes)) // validated by lockTimeSerializedSize
	for _, entry := range s.LockTimes {
		out = binary.LittleEndian.AppendUint32(out, entry.Index)
		out = binary.LittleEndian.AppendUint32(out, entry.LockTime)
		flags := byte(0)
		if entry.Enabled {
			flags = 1
		}
		out = append(out, flags)
	}
	if _, err := buf.Write(out); err != nil {
		return fmt.Errorf("unable to write locktime trailer: %w", err)
	}

	return nil
}

func (s *Meta) deserializeLockTimes(reader io.Reader, nodeCount uint32) error {
	var scratch [lockTimeRecordSize]byte
	n, err := io.ReadFull(reader, scratch[:lockTimeHeaderSize])
	if n == 0 && errors.Is(err, io.EOF) {
		return nil // Legacy blob: no claim about transaction locktimes.
	}
	if err != nil {
		return fmt.Errorf("unable to read locktime header: %w", err)
	}
	if string(scratch[:4]) != lockTimeMagic {
		return fmt.Errorf("unknown locktime trailer: %w", ErrInvalidLockTimeMetadata)
	}

	count := binary.LittleEndian.Uint32(scratch[4:lockTimeHeaderSize])
	if count > nodeCount {
		return fmt.Errorf("locktime count %d exceeds node count %d: %w", count, nodeCount, ErrInvalidLockTimeMetadata)
	}
	// Bound speculative allocation from wire counts, just as the inpoint decoder
	// does. Even a huge valid node count cannot make a truncated trailer allocate
	// its whole claimed table before any records have been read.
	s.LockTimes = make([]TxLockTime, 0, min(count, inpointsPreallocLimit))
	previousIndex := int64(-1)
	for i := uint32(0); i < count; i++ {
		if _, err := io.ReadFull(reader, scratch[:]); err != nil {
			return fmt.Errorf("unable to read locktime entry %d: %w", i, err)
		}
		if scratch[8] > 1 {
			return fmt.Errorf("unknown locktime flags: %w", ErrInvalidLockTimeMetadata)
		}
		entry := TxLockTime{
			Index:    binary.LittleEndian.Uint32(scratch[:4]),
			LockTime: binary.LittleEndian.Uint32(scratch[4:8]),
			Enabled:  scratch[8] == 1,
		}
		if err := entry.validate(int(nodeCount), previousIndex); err != nil {
			return err
		}
		s.LockTimes = append(s.LockTimes, entry)
		previousIndex = int64(entry.Index)
	}

	if _, err := io.ReadFull(reader, scratch[:1]); !errors.Is(err, io.EOF) {
		if err != nil {
			return fmt.Errorf("unable to read end of locktime trailer: %w", err)
		}
		return fmt.Errorf("surplus bytes after locktime trailer: %w", ErrInvalidLockTimeMetadata)
	}

	return nil
}

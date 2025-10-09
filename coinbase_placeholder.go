// Package subtree provides utilities for working with Bitcoin SV merkle tree subtrees,
// including coinbase placeholder handling and subtree data structures.
package subtree

import (
	"github.com/bsv-blockchain/go-bt/v2"
	"github.com/bsv-blockchain/go-bt/v2/chainhash"
)

var (
	// CoinbasePlaceholder hard code this value to avoid having to calculate it every time
	// to help the compiler optimize the code.
	CoinbasePlaceholder = [32]byte{
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	}
	// CoinbasePlaceholderHashValue is the hash representation of the coinbase placeholder
	CoinbasePlaceholderHashValue = chainhash.Hash(CoinbasePlaceholder)
	// CoinbasePlaceholderHash is a pointer to the coinbase placeholder hash value
	CoinbasePlaceholderHash = &CoinbasePlaceholderHashValue

	// FrozenBytes represents the frozen transaction bytes used for placeholder transactions
	FrozenBytes = [36]byte{
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
	}
	// FrozenBytesTxBytes represents the transaction bytes portion of FrozenBytes
	FrozenBytesTxBytes = FrozenBytes[0:32]
	// FrozenBytesTxHash is the hash of the frozen transaction bytes
	FrozenBytesTxHash = chainhash.Hash(FrozenBytesTxBytes)
)

func generateCoinbasePlaceholderTx() *bt.Tx {
	tx := bt.NewTx()
	tx.Version = 0xFFFFFFFF
	tx.LockTime = 0xFFFFFFFF

	return tx
}

// IsCoinbasePlaceHolderTx checks if the given transaction is a coinbase placeholder transaction.
func IsCoinbasePlaceHolderTx(tx *bt.Tx) bool {
	coinbasePlaceholderTx := generateCoinbasePlaceholderTx()

	coinbasePlaceholderTxHash := coinbasePlaceholderTx.TxIDChainHash()

	return tx.TxIDChainHash().IsEqual(coinbasePlaceholderTxHash)
}

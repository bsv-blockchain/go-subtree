package subtree

import (
	"testing"

	"github.com/bsv-blockchain/go-bt/v2"
	"github.com/stretchr/testify/assert"
)

func TestCoinbasePlaceholderTx(t *testing.T) {
	coinbasePlaceholderTx := generateCoinbasePlaceholderTx()
	coinbasePlaceholderTxHash := coinbasePlaceholderTx.TxIDChainHash()
	assert.True(t, IsCoinbasePlaceHolderTx(coinbasePlaceholderTx))
	assert.Equal(t, coinbasePlaceholderTx.Version, uint32(0xFFFFFFFF))
	assert.Equal(t, coinbasePlaceholderTx.LockTime, uint32(0xFFFFFFFF))
	assert.Equal(t, coinbasePlaceholderTx.TxIDChainHash(), coinbasePlaceholderTxHash)
	assert.False(t, IsCoinbasePlaceHolderTx(bt.NewTx()))
	assert.Equal(t, "a8502e9c08b3c851201a71d25bf29fd38a664baedb777318b12d19242f0e46ab", coinbasePlaceholderTx.TxIDChainHash().String())
}

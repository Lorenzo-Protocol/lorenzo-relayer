package types

import (
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// BtcTxInfo stores information of a BTC tx as part of a checkpoint
type BtcTxInfo struct {
	TxId          *chainhash.Hash
	Tx            *wire.MsgTx
	ChangeAddress btcutil.Address
	Utxo          *UTXO          // the UTXO used to build this BTC tx
	Size          int64          // the size of the BTC tx
	Fee           btcutil.Amount // tx fee cost by the BTC tx
}

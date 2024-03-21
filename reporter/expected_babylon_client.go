package reporter

import (
	"context"
	btclctypes "github.com/Lorenzo-Protocol/lorenzo/x/btclightclient/types"
	"github.com/Lorenzo-Protocol/rpc-client/config"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	pv "github.com/cosmos/relayer/v2/relayer/provider"
)

type LorenzoClient interface {
	MustGetAddr() string
	GetConfig() *config.LorenzoConfig
	InsertHeaders(ctx context.Context, msgs *btclctypes.MsgInsertHeaders) (*pv.RelayerTxResponse, error)
	ContainsBTCBlock(blockHash *chainhash.Hash) (*btclctypes.QueryContainsBytesResponse, error)
	BTCHeaderChainTip() (*btclctypes.QueryTipResponse, error)
	BTCBaseHeader() (*btclctypes.QueryBaseHeaderResponse, error)
}

package bnbreporter

import (
	"context"

	"github.com/Lorenzo-Protocol/lorenzo/v3/x/bnblightclient/types"
	pv "github.com/cosmos/relayer/v2/relayer/provider"
)

type LorenzoClient interface {
	MustGetAddr() string
	BNBUploadHeaders(ctx context.Context, msgHeaders *types.MsgUploadHeaders) (*pv.RelayerTxResponse, error)
	BNBLatestHeader() (*types.Header, error)
}

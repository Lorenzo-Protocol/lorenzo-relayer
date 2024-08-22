package bnbclient

import (
	"github.com/ethereum/go-ethereum/common"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/v2/bnbclient/bnbtypes"
)

type BNBClient interface {
	BlockNumber() (uint64, error)
	LatestHeader() (*bnbtypes.Header, error)
	RangeHeaders(start, end uint64) ([]*bnbtypes.Header, error)
	HeaderByNumber(number uint64) (*bnbtypes.Header, error)
	HeaderByHash(hash common.Hash) (*bnbtypes.Header, error)
}

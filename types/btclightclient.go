package types

import (
	lorenzotypes "github.com/Lorenzo-Protocol/lorenzo/v3/types"
	btcltypes "github.com/Lorenzo-Protocol/lorenzo/v3/x/btclightclient/types"
)

func NewMsgInsertHeaders(
	signer string,
	headers []*IndexedBlock,
) *btcltypes.MsgInsertHeaders {

	headerBytes := make([]lorenzotypes.BTCHeaderBytes, len(headers))
	for i, h := range headers {
		header := h
		headerBytes[i] = lorenzotypes.NewBTCHeaderBytesFromBlockHeader(header.Header)
	}

	return &btcltypes.MsgInsertHeaders{
		Signer:  signer,
		Headers: headerBytes,
	}
}

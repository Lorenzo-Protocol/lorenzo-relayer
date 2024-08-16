package bnbreporter

import (
	"bytes"

	"github.com/Lorenzo-Protocol/lorenzo/v3/x/bnblightclient/types"
	"github.com/ethereum/go-ethereum/rlp"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/v2/bnbclient/bnbtypes"
)

func ConvertLorenzoBNBResponseToHeader(header *types.Header) (*bnbtypes.Header, error) {
	bnbHeader := &bnbtypes.Header{}
	if err := rlp.Decode(bytes.NewReader(header.RawHeader), bnbHeader); err != nil {
		return nil, err
	}

	return bnbHeader, nil
}

func ConvertBNBHeaderToLorenzoBNBHeaders(headers []*bnbtypes.Header) ([]*types.Header, error) {
	lorenzoBNBHeaders := make([]*types.Header, 0, len(headers))
	for _, header := range headers {
		lorenzoBNBHeader := &types.Header{}
		var headerRawBuf bytes.Buffer
		err := header.EncodeRLP(&headerRawBuf)
		if err != nil {
			return nil, err
		}
		lorenzoBNBHeader.RawHeader = headerRawBuf.Bytes()
		lorenzoBNBHeader.Number = header.Number.Uint64()
		lorenzoBNBHeader.Hash = header.Hash().Bytes()
		lorenzoBNBHeader.ParentHash = header.ParentHash.Bytes()
		lorenzoBNBHeader.ReceiptRoot = header.ReceiptHash.Bytes()

		lorenzoBNBHeaders = append(lorenzoBNBHeaders, lorenzoBNBHeader)
	}

	return lorenzoBNBHeaders, nil
}

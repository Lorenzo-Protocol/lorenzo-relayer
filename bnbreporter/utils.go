package bnbreporter

import (
	"bytes"
	"sync"

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
	lorenzoBNBHeaders := make([]*types.Header, len(headers))
	errs := make(chan error, len(headers))

	var wg sync.WaitGroup
	for i, header := range headers {
		wg.Add(1)
		go func(i int, header *bnbtypes.Header) {
			defer wg.Done()
			lorenzoBNBHeader := &types.Header{}
			var headerRawBuf bytes.Buffer
			err := header.EncodeRLP(&headerRawBuf)
			if err != nil {
				errs <- err
				return
			}
			lorenzoBNBHeader.RawHeader = headerRawBuf.Bytes()
			lorenzoBNBHeader.Number = header.Number.Uint64()
			lorenzoBNBHeader.Hash = header.Hash().Bytes()
			lorenzoBNBHeader.ParentHash = header.ParentHash.Bytes()
			lorenzoBNBHeader.ReceiptRoot = header.ReceiptHash.Bytes()
			lorenzoBNBHeaders[i] = lorenzoBNBHeader
		}(i, header)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return lorenzoBNBHeaders, nil
}

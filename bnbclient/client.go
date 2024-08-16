package bnbclient

import (
	"context"
	"errors"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/v2/bnbclient/bnbtypes"
)

type Client struct {
	ethClient *ethclient.Client
	// Supplement to ethclient
	rpcClient *rpc.Client
}

func New(rpcUrl string) (*Client, error) {
	client, err := ethclient.Dial(rpcUrl)
	if err != nil {
		return nil, err
	}

	rpcClient, err := rpc.DialContext(context.Background(), rpcUrl)
	if err != nil {
		return nil, err
	}

	return &Client{
		ethClient: client,
		rpcClient: rpcClient,
	}, nil
}

func (c *Client) LatestHeader() (*bnbtypes.Header, error) {
	latestBlockNumber, err := c.BlockNumber()
	if err != nil {
		return nil, err
	}

	return c.HeaderByNumber(latestBlockNumber)
}

func (c *Client) RangeHeaders(start, end uint64) ([]*bnbtypes.Header, error) {
	if start > end {
		return nil, errors.New("start block number should be less than end block number")
	}
	endHeader, err := c.HeaderByNumber(end)
	if err != nil {
		return nil, err
	}
	chainHeaders := make([]*bnbtypes.Header, end-start+1)
	chainHeaders[len(chainHeaders)-1] = endHeader
	preHeaderHash := endHeader.ParentHash

	for i := len(chainHeaders) - 2; i >= 0; i-- {
		header, err := c.HeaderByHash(preHeaderHash)
		if err != nil {
			return nil, err
		}
		chainHeaders[i] = header
		preHeaderHash = header.ParentHash
	}

	return chainHeaders, nil
}

func (c *Client) HeaderByNumber(number uint64) (*bnbtypes.Header, error) {
	var header *bnbtypes.Header
	err := c.rpcClient.CallContext(context.Background(), &header, "eth_getBlockByNumber", hexutil.EncodeUint64(number), false)
	if err == nil && header == nil {
		err = ethereum.NotFound
	}

	return header, err
}

func (c *Client) HeaderByHash(hash common.Hash) (*bnbtypes.Header, error) {
	var header *bnbtypes.Header
	err := c.rpcClient.CallContext(context.Background(), &header, "eth_getBlockByHash", hash, false)
	if err == nil && header == nil {
		err = ethereum.NotFound
	}

	return header, err
}

func (c *Client) BlockNumber() (uint64, error) {
	blockNumber, err := c.ethClient.BlockNumber(context.Background())
	if err != nil {
		return 0, err
	}

	return blockNumber, nil
}

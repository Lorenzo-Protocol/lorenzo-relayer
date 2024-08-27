package bnbclient

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"

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
	if end-start > 10 {
		return c.rangeHeadersByMultiTask(start, end)
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

func (c *Client) rangeHeadersByMultiTask(start, end uint64) ([]*bnbtypes.Header, error) {
	total := end - start + 1
	chainHeaders := make([]*bnbtypes.Header, total)
	errs := make(chan error, len(chainHeaders))
	var wg sync.WaitGroup

	wg.Add(int(total))
	for i := uint64(0); i < total; i++ {
		go func(i uint64) {
			defer wg.Done()

			maxTryTimes := 5
			for tryTimes := 1; tryTimes <= maxTryTimes; tryTimes++ {
				header, err := c.HeaderByNumber(start + i)
				if err != nil {
					if tryTimes == maxTryTimes {
						errs <- err
						return
					}

					time.Sleep(time.Millisecond * time.Duration(100*tryTimes+rand.Intn(100)))
					continue
				}

				chainHeaders[i] = header
				break
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			return nil, err
		}
	}

	endHeader := chainHeaders[len(chainHeaders)-1]
	for i := len(chainHeaders) - 2; i >= 0; i-- {
		if endHeader.ParentHash != chainHeaders[i].Hash() {
			return nil, errors.New("chain headers are not continuous")
		}

		endHeader = chainHeaders[i]
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

package btcclient

import (
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/v2/types"
)

// GetBestBlock provides similar functionality with the btcd.rpcclient.GetBestBlock function
// We implement this, because this function is only provided by btcd.
func (c *Client) GetBestBlock() (*chainhash.Hash, uint64, error) {
	btcLatestBlockHash, err := c.GetBestBlockHash()
	if err != nil {
		return nil, 0, err
	}
	btcLatestBlock, err := c.GetBlockVerbose(btcLatestBlockHash)
	if err != nil {
		return nil, 0, err
	}
	btcLatestBlockHeight := uint64(btcLatestBlock.Height)
	return btcLatestBlockHash, btcLatestBlockHeight, nil
}

func (c *Client) GetBlockByHash(blockHash *chainhash.Hash) (*types.IndexedBlock, *wire.MsgBlock, error) {
	blockInfo, err := c.GetBlockVerbose(blockHash)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get block verbose by hash %s: %w", blockHash.String(), err)
	}

	mBlock, err := c.GetBlock(blockHash)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get block by hash %s: %w", blockHash.String(), err)
	}

	btcTxs := types.GetWrappedTxs(mBlock)
	return types.NewIndexedBlock(int32(blockInfo.Height), &mBlock.Header, btcTxs), mBlock, nil
}

// GetBlockByHeight returns a block with the given height
func (c *Client) GetBlockByHeight(height uint64) (*types.IndexedBlock, *wire.MsgBlock, error) {
	blockHash, err := c.GetBlockHash(int64(height))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get block by height %d: %w", height, err)
	}

	return c.GetBlockByHash(blockHash)
}

// getChainBlocks returns a chain of indexed blocks from the block at baseHeight to the tipBlock
// note: the caller needs to ensure that tipBlock is on the blockchain
func (c *Client) getChainBlocks(baseHeight uint64, tipBlock *types.IndexedBlock) ([]*types.IndexedBlock, error) {
	tipHeight := uint64(tipBlock.Height)
	if tipHeight < baseHeight {
		return nil, fmt.Errorf("the tip block height %v is less than the base height %v", tipHeight, baseHeight)
	}

	// the returned blocks include the block at the base height and the tip block
	chainBlocks := make([]*types.IndexedBlock, tipHeight-baseHeight+1)
	chainBlocks[len(chainBlocks)-1] = tipBlock

	if tipHeight == baseHeight {
		return chainBlocks, nil
	}

	prevHash := &tipBlock.Header.PrevBlock
	// minus 2 is because the tip block is already put in the last position of the slice,
	// and it is ensured that the length of chainBlocks is more than 1
	for i := len(chainBlocks) - 2; i >= 0; i-- {
		ib, mb, err := c.GetBlockByHash(prevHash)
		if err != nil {
			return nil, fmt.Errorf("failed to get block by hash %x: %w", prevHash, err)
		}
		chainBlocks[i] = ib
		prevHash = &mb.Header.PrevBlock
	}

	return chainBlocks, nil
}

func (c *Client) getBestIndexedBlock() (*types.IndexedBlock, error) {
	tipHash, err := c.GetBestBlockHash()
	if err != nil {
		return nil, fmt.Errorf("failed to get the best block: %w", err)
	}
	tipIb, _, err := c.GetBlockByHash(tipHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get the block by hash %x: %w", tipHash, err)
	}

	return tipIb, nil
}

// FindTailBlocksByHeight returns the chain of blocks from the block at baseHeight to the tip
func (c *Client) FindTailBlocksByHeight(baseHeight uint64) ([]*types.IndexedBlock, error) {
	tipIb, err := c.getBestIndexedBlock()
	if err != nil {
		return nil, err
	}

	if baseHeight > uint64(tipIb.Height) {
		return nil, fmt.Errorf("invalid base height %d, should not be higher than tip block %d", baseHeight, tipIb.Height)
	}

	return c.getChainBlocks(baseHeight, tipIb)
}

func (c *Client) FindRangeBlocksByHeight(startHeight, endHeight uint64) ([]*types.IndexedBlock, error) {
	endId, _, err := c.GetBlockByHeight(endHeight)
	if err != nil {
		return nil, err
	}

	return c.getChainBlocks(startHeight, endId)
}

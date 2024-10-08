package reporter

import (
	"fmt"
	"time"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/v2/types"
)

const (
	BlockEventCheckInterval = time.Minute
)

// blockEventHandler handles connected and disconnected blocks from the BTC client.
func (r *Reporter) blockEventHandler() {
	defer r.wg.Done()
	quit := r.quitChan()

	for {
		select {
		case event, open := <-r.btcClient.BlockEventChan():
			//delay block processing until the block is mature
			for {
				select {
				case <-quit:
					return
				default:
				}

				_, h, err := r.btcClient.GetBestBlock()
				if err != nil {
					r.logger.Warnf("Failed to get best block from BTC client: %v", err)
					time.Sleep(time.Second)
					continue
				}
				if h >= r.delayBlocks+uint64(event.Height) {
					break
				}
				r.logger.Debugf("Delaying block processing for %d blocks. blockHeight: %d, btcTip: %d",
					r.delayBlocks, event.Height, h)
				time.Sleep(BlockEventCheckInterval)
			}

			if !open {
				r.logger.Errorf("Block event channel is closed")
				return // channel closed
			}

			var errorRequiringBootstrap error
			if event.EventType == types.BlockConnected {
				errorRequiringBootstrap = r.handleConnectedBlocks(event)
			} else if event.EventType == types.BlockDisconnected {
				errorRequiringBootstrap = r.handleDisconnectedBlocks(event)
			}

			if errorRequiringBootstrap != nil {
				r.logger.Warnf("Due to error in event processing: %v, bootstrap process need to be restarted", errorRequiringBootstrap)
				r.bootstrapWithRetries(true)
			}

		case <-quit:
			// We have been asked to stop
			return
		}
	}
}

// handleConnectedBlocks handles connected blocks from the BTC client.
func (r *Reporter) handleConnectedBlocks(event *types.BlockEvent) error {
	// After delay blocks, hope the connected block is on the best chain, otherwise restart bootstrap.
	// It is just to reduce branching on Lorenzo side.
	{
		ib, _, err := r.btcClient.GetBlockByHeight(uint64(event.Height))
		if err != nil {
			return err
		}
		eventBlockHash := event.Header.BlockHash()
		ibBlockHash := ib.BlockHash()
		if !eventBlockHash.IsEqual(&ibBlockHash) {
			return fmt.Errorf("connected block[%s] isn't on the best chain", eventBlockHash.String())
		}
	}

	// if the header is too early, ignore it
	// NOTE: this might happen when bootstrapping is triggered after the reporter
	// has subscribed to the BTC blocks
	firstCacheBlock := r.btcCache.First()
	if firstCacheBlock == nil {
		return fmt.Errorf("cache is empty, restart bootstrap process")
	}
	if event.Height < firstCacheBlock.Height {
		r.logger.Debugf(
			"the connecting block (height: %d, hash: %s) is too early, skipping the block",
			event.Height,
			event.Header.BlockHash().String(),
		)
		return nil
	}

	// if the received header is within the cache's region, then this means the events have
	// an overlap with the cache. Then, perform a consistency check. If the block is duplicated,
	// then ignore the block, otherwise there is an inconsistency and redo bootstrap
	// NOTE: this might happen when bootstrapping is triggered after the reporter
	// has subscribed to the BTC blocks
	if b := r.btcCache.FindBlock(uint64(event.Height)); b != nil {
		if b.BlockHash() == event.Header.BlockHash() {
			r.logger.Debugf(
				"the connecting block (height: %d, hash: %s) is known to cache, skipping the block",
				b.Height,
				b.BlockHash().String(),
			)
			return nil
		}
		return fmt.Errorf(
			"the connecting block (height: %d, hash: %s) is different from the header (height: %d, hash: %s) at the same height in cache",
			event.Height,
			event.Header.BlockHash().String(),
			b.Height,
			b.BlockHash().String(),
		)
	}

	// get the block from hash
	blockHash := event.Header.BlockHash()
	ib, mBlock, err := r.btcClient.GetBlockByHash(&blockHash)
	if err != nil {
		return fmt.Errorf("failed to get block %v with number %d ,from BTC client: %w", blockHash, event.Height, err)
	}

	// if the parent of the block is not the tip of the cache, then the cache is not up-to-date,
	// and we might have missed some blocks. In this case, restart the bootstrap process.
	parentHash := mBlock.Header.PrevBlock
	cacheTip := r.btcCache.Tip() // NOTE: cache is guaranteed to be non-empty at this stage
	if parentHash != cacheTip.BlockHash() {
		return fmt.Errorf("cache (tip %d) is not up-to-date while connecting block %d, restart bootstrap process", cacheTip.Height, ib.Height)
	}

	// otherwise, add the block to the cache
	r.btcCache.Add(ib)

	var headersToProcess []*types.IndexedBlock

	if r.reorgList.size() > 0 {
		// we are in the middle of reorg, we need to check whether we already have all blocks of better chain
		// as reorgs in btc nodes happen only when better chain is available.
		// 1. First we get oldest header from our reorg branch
		// 2. Then we get all headers from our cache starting the height of the oldest header of new branch
		// 3. then we calculate if work on new branch starting from the first reorged height is larger
		// than removed branch work.
		oldestBlockFromOldBranch := r.reorgList.getLastRemovedBlock()
		currentBranch, err := r.btcCache.GetLastBlocks(oldestBlockFromOldBranch.height)
		if err != nil {
			panic(fmt.Errorf("failed to get block from cache after reorg: %w", err))
		}

		currentBranchWork := calculateBranchWork(currentBranch)

		// if current branch is better than reorg branch, we can submit headers and clear reorg list
		if currentBranchWork.GT(r.reorgList.removedBranchWork()) {
			r.logger.Debugf("Current branch is better than reorg branch. Length of current branch: %d, work of branch: %s", len(currentBranch), currentBranchWork)
			headersToProcess = append(headersToProcess, currentBranch...)
			r.reorgList.clear()
		}
	} else {
		lorenzoTip, err := r.lorenzoClient.BTCHeaderChainTip()
		if err != nil {
			return err
		}
		// after bootstrap, btcCache tip must be higher than lorenzo BTC Header tip
		// so we make lorenzo BTC Header tip catch up
		if lorenzoTip.Header.Height < uint64(ib.Height-1) {
			ibs, err := r.btcCache.GetLastBlocks(lorenzoTip.Header.Height + 1)
			if err != nil {
				return err
			}
			headersToProcess = append(headersToProcess, ibs...)
		} else {
			headersToProcess = append(headersToProcess, ib)
		}
	}

	if len(headersToProcess) == 0 {
		r.logger.Debug("No new headers to submit to Lorenzo")
		return nil
	}

	// extracts and submits headers for each blocks in ibs
	signer := r.lorenzoClient.MustGetAddr()
	_, err = r.ProcessHeaders(signer, headersToProcess)
	if err != nil {
		r.logger.Warnf("Failed to submit header: %v", err)
	}

	return nil
}

// handleDisconnectedBlocks handles disconnected blocks from the BTC client.
func (r *Reporter) handleDisconnectedBlocks(event *types.BlockEvent) error {
	// get cache tip
	cacheTip := r.btcCache.Tip()
	if cacheTip == nil {
		return fmt.Errorf("cache is empty, restart bootstrap process")
	}

	// if the block to be disconnected is not the tip of the cache, then the cache is not up-to-date,
	if event.Header.BlockHash() != cacheTip.BlockHash() {
		return fmt.Errorf("cache is not up-to-date while disconnecting block, restart bootstrap process")
	}

	// at this point, the block to be disconnected is the tip of the cache so we can
	// add it to our reorg list
	r.reorgList.addRemovedBlock(
		uint64(cacheTip.Height),
		cacheTip.Header,
	)

	// otherwise, remove the block from the cache
	if err := r.btcCache.RemoveLast(); err != nil {
		r.logger.Warnf("Failed to remove last block from cache: %v, restart bootstrap process", err)
		panic(err)
	}

	return nil
}

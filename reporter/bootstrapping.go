package reporter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/btcsuite/btcd/chaincfg/chainhash"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/types"
)

var (
	bootstrapAttempts      = uint(60)
	bootstrapAttemptsAtt   = retry.Attempts(bootstrapAttempts)
	bootstrapRetryInterval = retry.Delay(30 * time.Second)
	bootstrapDelayType     = retry.DelayType(retry.FixedDelay)
	bootstrapErrReportType = retry.LastErrorOnly(true)
)

type consistencyCheckInfo struct {
	lorenzoLatestBlockHeight uint64
	startSyncHeight          uint64
}

// checkConsistency checks whether the `max(lorenzo_tip_height - confirmation_depth, lorenzo_base_height)` block is same
// between Lorenzo header chain and BTC main chain.` This makes sure that already confirmed chain is the same from point
// of view of both chains.
func (r *Reporter) checkConsistency() (*consistencyCheckInfo, error) {

	tipRes, err := r.lorenzoClient.BTCHeaderChainTip()
	if err != nil {
		return nil, err
	}

	// Find the base height of Lorenzo header chain
	baseRes, err := r.lorenzoClient.BTCBaseHeader()
	if err != nil {
		return nil, err
	}

	var consistencyCheckHeight uint64
	if tipRes.Header.Height >= baseRes.Header.Height+r.btcConfirmationDepth {
		consistencyCheckHeight = tipRes.Header.Height - r.btcConfirmationDepth
	} else {
		consistencyCheckHeight = baseRes.Header.Height
	}

	// this checks whether header at already confirmed height is the same in reporter btc cache and in lorenzo btc light client
	if err := r.checkHeaderConsistency(consistencyCheckHeight); err != nil {
		return nil, err
	}

	return &consistencyCheckInfo{
		lorenzoLatestBlockHeight: tipRes.Header.Height,
		// we are staring from the block after already confirmed block
		startSyncHeight: consistencyCheckHeight + 1,
	}, nil
}

func (r *Reporter) bootstrap(skipBlockSubscription bool) error {
	defer func(start time.Time) {
		r.logger.Debugf("bootstrap time used %v", time.Since(start))
	}(time.Now())

	var (
		btcLatestBlockHeight uint64
		ibs                  []*types.IndexedBlock
		err                  error
	)

	// if we are bootstraping, we will definitely not handle reorgs
	r.reorgList.clear()

	// ensure BTC has caught up with Lorenzo header chain
	if err := r.waitUntilBTCSync(); err != nil {
		return err
	}

	// initialize cache with the latest blocks
	if err := r.initBTCCache(); err != nil {
		return err
	}
	r.logger.Debugf("BTC cache size: %d", r.btcCache.Size())

	// Subscribe new blocks right after initialising BTC cache, in order to ensure subscribed blocks and cached blocks do not have overlap.
	// Otherwise, if we subscribe too early, then they will have overlap, leading to duplicated header/ckpt submissions.
	if !skipBlockSubscription {
		r.btcClient.MustSubscribeBlocks()
	}

	consistencyInfo, err := r.checkConsistency()

	if err != nil {
		return err
	}

	ibs, err = r.btcCache.GetLastBlocks(consistencyInfo.startSyncHeight)
	if err != nil {
		panic(err)
	}

	signer := r.lorenzoClient.MustGetAddr()

	r.logger.Infof("BTC height: %d. BTCLightclient height: %d. Start syncing from height %d.", btcLatestBlockHeight, consistencyInfo.lorenzoLatestBlockHeight, consistencyInfo.startSyncHeight)

	// extracts and submits headers for each block in ibs
	// Note: As we are retrieving blocks from btc cache from block just after confirmed block which
	// we already checked for consistency, we can be sure that even if rest of the block headers is different from in Lorenzo
	// due to reorg, our fork will be better than the one in Lorenzo.
	ibs = ibs[:len(ibs)-int(r.delayBlocks)] //only process the last delayBlocks
	_, err = r.ProcessHeaders(signer, ibs)
	if err != nil {
		// this can happen when there are two contentious lrzrelayer or if our btc node is behind.
		r.logger.Errorf("Failed to submit headers: %v", err)
		// returning error as it is up to the caller to decide what do next
		return err
	}

	// trim cache to the latest k+w blocks on BTC (which are same as in Lorenzo)
	maxEntries := r.btcConfirmationDepth + r.checkpointFinalizationTimeout
	if err = r.btcCache.Resize(maxEntries); err != nil {
		r.logger.Errorf("Failed to resize BTC cache: %v", err)
		panic(err)
	}
	r.btcCache.Trim()

	r.logger.Infof("Size of the BTC cache: %d", r.btcCache.Size())

	r.logger.Info("Successfully finished bootstrapping")
	return nil
}

func (r *Reporter) reporterQuitCtx() (context.Context, func()) {
	quit := r.quitChan()
	ctx, cancel := context.WithCancel(context.Background())
	r.wg.Add(1)
	go func() {
		defer cancel()
		defer r.wg.Done()

		select {
		case <-quit:

		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}

func (r *Reporter) bootstrapWithRetries(skipBlockSubscription bool) {
	// if we are exiting, we need to cancel this process
	ctx, cancel := r.reporterQuitCtx()
	defer cancel()
	if err := retry.Do(func() error {
		return r.bootstrap(skipBlockSubscription)
	},
		retry.Context(ctx),
		bootstrapAttemptsAtt,
		bootstrapRetryInterval,
		bootstrapDelayType,
		bootstrapErrReportType, retry.OnRetry(func(n uint, err error) {
			r.logger.Warnf("Failed to bootstap reporter: %v. Attempt: %d, Max attempts: %d", err, n+1, bootstrapAttempts)
		})); err != nil {

		if errors.Is(err, context.Canceled) {
			// context was cancelled we do not need to anything more, app is quiting
			return
		}

		// we failed to bootstrap multiple time, we should panic as something unexpected is happening.
		r.logger.Fatalf("Failed to bootstrap reporter: %v after %d attempts", err, bootstrapAttempts)
	}
}

// initBTCCache fetches the blocks since T-k-w in the BTC canonical chain
// where T is the height of the latest block in Lorenzo header chain
func (r *Reporter) initBTCCache() error {
	var (
		err                      error
		lorenzoLatestBlockHeight uint64
		lorenzoBaseHeight        uint64
		baseHeight               uint64
		ibs                      []*types.IndexedBlock
	)

	r.btcCache, err = types.NewBTCCache(r.Cfg.BTCCacheSize) // TODO: give an option to be unsized
	if err != nil {
		panic(err)
	}

	// get T, i.e., total block count in Lorenzo header chain
	tipRes, err := r.lorenzoClient.BTCHeaderChainTip()
	if err != nil {
		return err
	}
	lorenzoLatestBlockHeight = tipRes.Header.Height

	// Find the base height
	baseRes, err := r.lorenzoClient.BTCBaseHeader()
	if err != nil {
		return err
	}
	lorenzoBaseHeight = baseRes.Header.Height

	// Fetch block since `baseHeight = T - k - w` from BTC, where
	// - T is total block count in Lorenzo header chain
	// - k is btcConfirmationDepth of Lorenzo
	// - w is checkpointFinalizationTimeout of Lorenzo
	if lorenzoLatestBlockHeight > lorenzoBaseHeight+r.btcConfirmationDepth+r.checkpointFinalizationTimeout {
		baseHeight = lorenzoLatestBlockHeight - r.btcConfirmationDepth - r.checkpointFinalizationTimeout + 1
	} else {
		baseHeight = lorenzoBaseHeight
	}

	ibs, err = r.btcClient.FindTailBlocksByHeight(baseHeight)
	if err != nil {
		panic(err)
	}

	if err = r.btcCache.Init(ibs); err != nil {
		panic(err)
	}
	return nil
}

func (r *Reporter) waitLorenzoCatchUpCloseToBTCTip() error {
	closeGap := r.btcConfirmationDepth * 2
	_, btcTip, err := r.btcClient.GetBestBlock()
	if err != nil {
		return err
	}

	lorenzoTip, err := r.lorenzoClient.BTCHeaderChainTip()
	if err != nil {
		return err
	}
	if lorenzoTip.Header.Height+closeGap >= btcTip {
		// don't anything
		return nil
	}
	r.logger.Infof("lorenzo begin catch up to close btc tip. from (%d) to (%d)", lorenzoTip.Header.Height, btcTip-closeGap)
	defer func(start time.Time) {
		r.logger.Infof("waitLorenzoCatchUpCloseToBTCTip time used: %v", time.Since(start))
	}(time.Now())

	if lorenzoTip.Header.Height+closeGap < btcTip {
		overCh := make(chan struct{})
		errorCh := make(chan error)
		ibCh := make(chan *types.IndexedBlock, 10)
		go func() {
			for h := lorenzoTip.Header.Height + 1; h < btcTip-closeGap; h++ {
				ib, _, err := r.btcClient.GetBlockByHeight(h)
				if err != nil {
					errorCh <- err
					return
				}
				ibCh <- ib
			}

			// fetch block over
			close(overCh)
		}()

		lorenzoNewTipHeader := lorenzoTip.Header.Hash.ToChainhash()
		signer := r.lorenzoClient.MustGetAddr()
		for {
			select {
			case ib := <-ibCh:
				if ib.Header.PrevBlock.IsEqual(lorenzoNewTipHeader) == false {
					r.logger.Panicf("height(%d) PrevBlock(%s) is not lorenzo tip(%s)",
						ib.Height, ib.Header.PrevBlock.String(), lorenzoNewTipHeader.String())
				}

				_, err = r.ProcessHeaders(signer, []*types.IndexedBlock{ib})
				if err != nil {
					panic(err)
				}
				currentHash := ib.Header.BlockHash()
				lorenzoNewTipHeader = &currentHash
			case err := <-errorCh:
				return err
			case <-overCh:
				return nil
			}
		}
	}

	return nil
}

// waitUntilBTCSync waits for BTC to synchronize until BTC is no shorter than Lorenzo's BTC light client.
// It returns BTC last block hash, BTC last block height, and Lorenzo's base height.
func (r *Reporter) waitUntilBTCSync() error {
	var (
		btcLatestBlockHash       *chainhash.Hash
		btcLatestBlockHeight     uint64
		lorenzoLatestBlockHash   *chainhash.Hash
		lorenzoLatestBlockHeight uint64
		err                      error
	)

	// Retrieve hash/height of the latest block in BTC
	btcLatestBlockHash, btcLatestBlockHeight, err = r.btcClient.GetBestBlock()
	if err != nil {
		return err
	}
	r.logger.Debugf("BTC latest block hash and height: (%v, %d)", btcLatestBlockHash, btcLatestBlockHeight)

	// TODO: if BTC falls behind BTCLightclient's base header, then the lrzrelayer is incorrectly configured and should panic

	// Retrieve hash/height of the latest block in Lorenzo header chain
	tipRes, err := r.lorenzoClient.BTCHeaderChainTip()
	if err != nil {
		return err
	}
	lorenzoLatestBlockHash = tipRes.Header.Hash.ToChainhash()
	lorenzoLatestBlockHeight = tipRes.Header.Height
	r.logger.Infof("Lorenzo header chain latest block hash and height: (%v, %d)", lorenzoLatestBlockHash, lorenzoLatestBlockHeight)

	// If BTC chain is shorter than Lorenzo header chain, pause until BTC catches up
	if btcLatestBlockHeight == 0 || btcLatestBlockHeight < lorenzoLatestBlockHeight {
		r.logger.Infof("BTC chain (length %d) falls behind Lorenzo header chain (length %d), wait until BTC catches up", btcLatestBlockHeight, lorenzoLatestBlockHeight)

		// periodically check if BTC catches up with Lorenzo.
		// When BTC catches up, break and continue the bootstrapping process
		ticker := time.NewTicker(5 * time.Second)
		for range ticker.C {
			_, btcLatestBlockHeight, err = r.btcClient.GetBestBlock()
			if err != nil {
				return err
			}
			tipRes, err = r.lorenzoClient.BTCHeaderChainTip()
			if err != nil {
				return err
			}
			lorenzoLatestBlockHeight = tipRes.Header.Height
			if btcLatestBlockHeight > 0 && btcLatestBlockHeight >= lorenzoLatestBlockHeight {
				r.logger.Infof("BTC chain (length %d) now catches up with Lorenzo header chain (length %d), continue bootstrapping", btcLatestBlockHeight, lorenzoLatestBlockHeight)
				break
			}
			r.logger.Infof("BTC chain (length %d) still falls behind Lorenzo header chain (length %d), keep waiting", btcLatestBlockHeight, lorenzoLatestBlockHeight)
		}
	}

	return nil
}

func (r *Reporter) checkHeaderConsistency(consistencyCheckHeight uint64) error {
	var err error

	consistencyCheckBlock := r.btcCache.FindBlock(consistencyCheckHeight)
	if consistencyCheckBlock == nil {
		err = fmt.Errorf("cannot find the %d-th block of Lorenzo header chain in BTC cache for initial consistency check", consistencyCheckHeight)
		panic(err)
	}
	consistencyCheckHash := consistencyCheckBlock.BlockHash()

	r.logger.Debugf("block for consistency check: height %d, hash %v", consistencyCheckHeight, consistencyCheckHash)

	// Given that two consecutive BTC headers are chained via hash functions,
	// generating a header that can be in two different positions in two different BTC header chains
	// is as hard as breaking the hash function.
	// So as long as the block exists on Lorenzo, it has to be at the same position as in Lorenzo as well.
	res, err := r.lorenzoClient.ContainsBTCBlock(&consistencyCheckHash) // TODO: this API has error. Find out why
	if err != nil {
		return err
	}
	if !res.Contains {
		err = fmt.Errorf("BTC main chain is inconsistent with Lorenzo header chain: k-deep block in Lorenzo header chain: %v", consistencyCheckHash)
		panic(err)
	}
	return nil
}

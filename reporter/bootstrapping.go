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

const (
	FetchBTCBlocksBatchSize = 100
)

var (
	bootstrapAttempts      = uint(60)
	bootstrapAttemptsAtt   = retry.Attempts(bootstrapAttempts)
	bootstrapRetryInterval = retry.Delay(30 * time.Second)
	bootstrapDelayType     = retry.DelayType(retry.FixedDelay)
	bootstrapErrReportType = retry.LastErrorOnly(true)
)

func (r *Reporter) bootstrap() error {
	if err := r.waitUntilBTCSync(); err != nil {
		return err
	}
	commonHeight, commonHash, err := r.findCommonBlock()
	if err != nil {
		return err
	}
	if r.waitLorenzoCatchUp(commonHeight, commonHash) != nil {
		return err
	}

	lorenzoTipResp, err := r.lorenzoClient.BTCHeaderChainTip()
	if err != nil {
		return err
	}
	lorenzoTipHash := lorenzoTipResp.Header.Hash.ToChainhash()
	r.lorenzoBtcTip, _, err = r.btcClient.GetBlockByHash(lorenzoTipHash)
	if err != nil {
		return err
	}
	if r.lorenzoBtcTip == nil {
		err := fmt.Errorf("lorenzo tip block is not in btc chain. hash:%s", lorenzoTipHash.String())
		panic(err)
	}

	return nil
}

func (r *Reporter) findCommonBlock() (uint64, *chainhash.Hash, error) {
	tipRes, err := r.lorenzoClient.BTCHeaderChainTip()
	if err != nil {
		return 0, nil, err
	}

	// Find the base height of Lorenzo header chain
	baseRes, err := r.lorenzoClient.BTCBaseHeader()
	if err != nil {
		return 0, nil, err
	}

	var commonHeight uint64
	if tipRes.Header.Height >= baseRes.Header.Height+r.btcConfirmationDepth {
		commonHeight = tipRes.Header.Height - r.btcConfirmationDepth
	} else {
		commonHeight = baseRes.Header.Height
	}

	ib, _, err := r.btcClient.GetBlockByHeight(commonHeight)
	if err != nil {
		return 0, nil, err
	}
	btcBlockHash := ib.Header.BlockHash()
	lorenzoContainsBtcBlockResp, err := r.lorenzoClient.ContainsBTCBlock(&btcBlockHash)
	if err != nil {
		return 0, nil, err
	}
	if !lorenzoContainsBtcBlockResp.Contains {
		err = fmt.Errorf("BTC chain btcConfirmationsDepth block is not in lorenzo header chain. hash:%s, height: %d", btcBlockHash.String(), commonHeight)
		panic(err)
	}

	return commonHeight, &btcBlockHash, nil
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

func (r *Reporter) bootstrapWithRetries() {
	// if we are exiting, we need to cancel this process
	ctx, cancel := r.reporterQuitCtx()
	defer cancel()
	if err := retry.Do(func() error {
		return r.bootstrap()
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

func (r *Reporter) waitLorenzoCatchUp(commonHeight uint64, commonHash *chainhash.Hash) error {
	_, btcTip, err := r.btcClient.GetBestBlock()
	if err != nil {
		return err
	}
	if commonHeight+r.delayBlocks >= btcTip {
		return nil
	}

	overCh := make(chan struct{})
	errorCh := make(chan error)
	ibCh := make(chan []*types.IndexedBlock, 10)
	batchSize := uint64(FetchBTCBlocksBatchSize)
	r.logger.Infof("lorenzo begin catch up. from (%d) to (%d)", commonHeight, btcTip-r.delayBlocks)

	go func() {
		for h := commonHeight + 1; h < btcTip-r.delayBlocks; h++ {
			endHeight := h + batchSize - 1
			if endHeight > btcTip-r.delayBlocks {
				endHeight = btcTip - r.delayBlocks - 1
			}

			startFetch := time.Now()
			ibs, err := r.btcClient.FindRangeBlocksByHeight(h, endHeight)
			r.logger.Infof("fetch block from %d to %d, time used: %v", h, endHeight, time.Since(startFetch))
			if err != nil {
				errorCh <- err
				return
			}

			ibCh <- ibs
			h = endHeight
		}

		// fetch block over
		close(overCh)
	}()

	preHash := commonHash
	signer := r.lorenzoClient.MustGetAddr()
	for {
		select {
		case ibs := <-ibCh:
			if ibs[0].Header.PrevBlock.IsEqual(preHash) == false {
				r.logger.Panicf("height(%d) PrevBlock(%s) is not lorenzo tip(%s)",
					ibs[0].Height, ibs[0].Header.PrevBlock.String(), preHash.String())
			}

			_, err = r.ProcessHeaders(signer, ibs)
			if err != nil {
				panic(err)
			}
			currentHash := ibs[len(ibs)-1].Header.BlockHash()
			preHash = &currentHash
		case err := <-errorCh:
			return err
		case <-overCh:
			return nil
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

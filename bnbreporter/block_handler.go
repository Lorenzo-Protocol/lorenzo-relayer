package bnbreporter

import (
	"context"
	"fmt"
	"time"

	"github.com/Lorenzo-Protocol/lorenzo/v3/x/bnblightclient/types"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/v2/bnbclient/bnbtypes"
)

func (r *BNBReporter) mainLoop() {
	r.logger.Infof("=======BNB reporter start syncheaders=========")

	networkErrorTimeSleep := time.Millisecond * 300
	blockSleepTime := time.Second
	for {
		select {
		case <-r.quit:
			return
		default:
		}

		bnbTip, err := r.client.LatestHeader()
		if err != nil {
			r.logger.Errorf("failed to get BNB current height: %v", err)
			time.Sleep(networkErrorTimeSleep)
			continue
		}

		if r.delayBlocks+r.lorenzoTip.Number.Uint64()+1 > bnbTip.Number.Uint64() {
			r.logger.Debugf("delay blocks: %d, lorenzoTip: %d, bnbTip: %d",
				r.delayBlocks, r.lorenzoTip.Number.Uint64(), bnbTip.Number.Uint64())
			time.Sleep(blockSleepTime)
			continue
		}

		start := r.lorenzoTip.Number.Uint64() + 1
		end := bnbTip.Number.Uint64() - r.delayBlocks
		if end-start+1 > FetchBNBHeaderBatchSize {
			end = start + FetchBNBHeaderBatchSize - 1
		}

		newHeaders, err := r.client.RangeHeaders(start, end)
		if err != nil {
			r.logger.Errorf("failed to get BNB headers: %v", err)
			time.Sleep(networkErrorTimeSleep)
			continue
		}
		if err := r.handleHeaders(newHeaders); err != nil {
			r.logger.Warnf("failed to handle headers: %v", err)
			if err := r.boostrap(); err != nil {
				r.logger.Errorf("failed to bootstrap: %v", err)
			}
			continue
		}

		// update lorenzoTip after successfully handling the header
		r.lorenzoTip = newHeaders[len(newHeaders)-1]
	}
}

func (r *BNBReporter) handleHeader(newHeader *bnbtypes.Header) error {
	if newHeader == nil {
		return nil
	}

	startTime := time.Now()
	if newHeader.Number.Uint64() != r.lorenzoTip.Number.Uint64()+1 {
		return fmt.Errorf("newHeader number %d is not the next block of lorenzoTip number %d", newHeader.Number.Uint64(), r.lorenzoTip.Number.Uint64())
	}
	if r.lorenzoTip.Hash() != newHeader.ParentHash {
		err := fmt.Errorf("BNB chain is inconsistent with Lorenzo chain: k-deep(%d) block in Lorenzo header chain: %s", r.delayBlocks, newHeader.Hash().Hex())
		return err
	}

	lorenzoBNBHeaders, err := ConvertBNBHeaderToLorenzoBNBHeaders([]*bnbtypes.Header{newHeader})
	if err != nil {
		return err
	}
	_, err = r.lorenzoClient.BNBUploadHeaders(context.Background(), &types.MsgUploadHeaders{
		Signer:  r.lorenzoClient.MustGetAddr(),
		Headers: lorenzoBNBHeaders,
	})
	if err != nil {
		return err
	}

	r.logger.Infof("uploaded BNB header to lorenzo,height: %d, hash:%s. TimeUsed:%v",
		newHeader.Number.Uint64(), newHeader.Hash().Hex(), time.Since(startTime))
	return nil
}

func (r *BNBReporter) handleHeaders(newHeaders []*bnbtypes.Header) error {
	if len(newHeaders) == 0 {
		return nil
	}
	defer func(startTime time.Time) {
		r.logger.Infof("Upload block height %d to %d, from headerHash:%s. Time used: %v", newHeaders[0].Number.Uint64(),
			newHeaders[len(newHeaders)-1].Number.Uint64(), newHeaders[0].Hash().Hex(), time.Since(startTime))
	}(time.Now())

	if newHeaders[0].Number.Uint64() != r.lorenzoTip.Number.Uint64()+1 {
		return fmt.Errorf("newHeader number %d is not the next block of lorenzoTip number %d", newHeaders[0].Number.Uint64(), r.lorenzoTip.Number.Uint64())
	}
	if newHeaders[0].ParentHash != r.lorenzoTip.Hash() {
		err := fmt.Errorf("BNB chain is inconsistent with Lorenzo chain: k-deep(%d) block in Lorenzo header chain: %s", r.delayBlocks, newHeaders[0].Hash().Hex())
		return err
	}

	lorenzoBNBHeaders, err := ConvertBNBHeaderToLorenzoBNBHeaders(newHeaders)
	if err != nil {
		return err
	}
	_, err = r.lorenzoClient.BNBUploadHeaders(context.Background(), &types.MsgUploadHeaders{
		Signer:  r.lorenzoClient.MustGetAddr(),
		Headers: lorenzoBNBHeaders,
	})
	if err != nil {
		return err
	}

	r.lorenzoTip = newHeaders[len(newHeaders)-1]
	return nil
}

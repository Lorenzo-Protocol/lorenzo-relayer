package reporter

import (
	"context"
	"fmt"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/Lorenzo-Protocol/lorenzo/v3/types/retry"
	btclctypes "github.com/Lorenzo-Protocol/lorenzo/v3/x/btclightclient/types"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/v2/types"
)

func chunkBy[T any](items []T, chunkSize int) (chunks [][]T) {
	for chunkSize < len(items) {
		items, chunks = items[chunkSize:], append(chunks, items[0:chunkSize:chunkSize])
	}
	return append(chunks, items)
}

// getHeaderMsgsToSubmit creates a set of MsgInsertHeaders messages corresponding to headers that
// should be submitted to Lorenzo from a given set of indexed blocks
func (r *Reporter) getHeaderMsgsToSubmit(signer string, ibs []*types.IndexedBlock) ([]*btclctypes.MsgInsertHeaders, error) {
	var (
		startPoint  = -1
		ibsToSubmit []*types.IndexedBlock
		err         error
	)

	// find the first header that is not contained in Lorenzo header chain, then submit since this header
	for i, header := range ibs {
		blockHash := header.BlockHash()
		var res *btclctypes.QueryContainsBytesResponse
		err = retry.Do(r.retrySleepTime, r.maxRetrySleepTime, func() error {
			res, err = r.lorenzoClient.ContainsBTCBlock(&blockHash)
			return err
		})
		if err != nil {
			return nil, err
		}
		if !res.Contains {
			startPoint = i
			break
		}
	}

	// all headers are duplicated, no need to submit
	if startPoint == -1 {
		r.logger.Info("All headers are duplicated, no need to submit")
		return []*btclctypes.MsgInsertHeaders{}, nil
	}

	// wrap the headers to MsgInsertHeaders msgs from the subset of indexed blocks
	ibsToSubmit = ibs[startPoint:]

	blockChunks := chunkBy(ibsToSubmit, int(r.Cfg.MaxHeadersInMsg))

	headerMsgsToSubmit := []*btclctypes.MsgInsertHeaders{}

	for _, ibChunk := range blockChunks {
		msgInsertHeaders := types.NewMsgInsertHeaders(signer, ibChunk)
		headerMsgsToSubmit = append(headerMsgsToSubmit, msgInsertHeaders)
	}

	return headerMsgsToSubmit, nil
}

func (r *Reporter) submitHeaderMsgs(msg *btclctypes.MsgInsertHeaders) error {
	// submit the headers
	err := retry.Do(r.retrySleepTime, r.maxRetrySleepTime, func() error {
		res, err := r.lorenzoClient.InsertHeaders(context.Background(), msg)
		if err != nil {
			return err
		}
		r.logger.Infof("Successfully submitted %d headers to Lorenzo with response code %v", len(msg.Headers), res.Code)
		return nil
	})
	if err != nil {
		r.metrics.FailedHeadersCounter.Add(float64(len(msg.Headers)))
		return fmt.Errorf("failed to submit headers: %w", err)
	}

	// update metrics
	r.metrics.SuccessfulHeadersCounter.Add(float64(len(msg.Headers)))
	r.metrics.SecondsSinceLastHeaderGauge.Set(0)
	for _, header := range msg.Headers {
		r.metrics.NewReportedHeaderGaugeVec.WithLabelValues(header.Hash().String()).SetToCurrentTime()
	}

	return err
}

// ProcessHeaders extracts and reports headers from a list of blocks
// It returns the number of headers that need to be reported (after deduplication)
func (r *Reporter) ProcessHeaders(signer string, ibs []*types.IndexedBlock) (int, error) {
	defer func(start time.Time) {
		r.logger.Infof("Processed block height %d to %d, time used: %v", ibs[0].Height, ibs[len(ibs)-1].Height, time.Since(start))
	}(time.Now())

	// get a list of MsgInsertHeader msgs with headers to be submitted
	headerMsgsToSubmit, err := r.getHeaderMsgsToSubmit(signer, ibs)
	if err != nil {
		return 0, fmt.Errorf("failed to find headers to submit: %w", err)
	}
	// skip if no header to submit
	if len(headerMsgsToSubmit) == 0 {
		r.logger.Info("No new headers to submit")
		return 0, nil
	}

	var numSubmitted int
	// submit each chunk of headers
	for _, msgs := range headerMsgsToSubmit {
		if err := r.submitHeaderMsgs(msgs); err != nil {
			return 0, fmt.Errorf("failed to submit headers: %w", err)
		}
		numSubmitted += len(msgs.Headers)
	}

	return numSubmitted, err
}

func calculateBranchWork(branch []*types.IndexedBlock) sdkmath.Uint {
	var currenWork = sdkmath.ZeroUint()
	for _, h := range branch {
		headerWork := btclctypes.CalcHeaderWork(h.Header)
		currenWork = btclctypes.CumulativeWork(headerWork, currenWork)
	}
	return currenWork
}

// push msg to channel c, or quit if quit channel is closed
func PushOrQuit[T any](c chan<- T, msg T, quit <-chan struct{}) {
	select {
	case c <- msg:
	case <-quit:
	}
}

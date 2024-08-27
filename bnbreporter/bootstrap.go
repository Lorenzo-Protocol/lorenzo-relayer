package bnbreporter

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Lorenzo-Protocol/lorenzo/v3/x/bnblightclient/types"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/v2/bnbclient/bnbtypes"
)

var errLatestBNBHeaderNotFound = errors.New("latested header not found")

const baseBNBHeaderHeightDepth = 100

const FetchBNBHeaderBatchSize = 100

func (r *BNBReporter) boostrap() error {
	lorenzoBNBHeader, err := r.lorenzoClient.BNBLatestHeader()
	if err != nil {
		if strings.Contains(err.Error(), errLatestBNBHeaderNotFound.Error()) {
			return r.initLorenzoBNBBaseHeader()
		} else {
			return err
		}
	}

	bnbHeader, err := ConvertLorenzoBNBResponseToHeader(lorenzoBNBHeader)
	if err != nil {
		return err
	}

	r.lorenzoTip = bnbHeader
	return nil
}

func (r *BNBReporter) initLorenzoBNBBaseHeader() error {
	baseHeader, err := r.client.HeaderByNumber(r.cfg.BaseHeight)
	if err != nil {
		return err
	}

	// upload baseHeader to Lorenzo
	lorenzoBNBHeaders, err := ConvertBNBHeaderToLorenzoBNBHeaders([]*bnbtypes.Header{baseHeader})
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
	r.logger.Infof("uploaded base BNB header to lorenzo,height: %d, hash:%s",
		baseHeader.Number.Uint64(), baseHeader.Hash().Hex())

	return r.boostrap()
}

func (r *BNBReporter) WaitBNBCatchUp() error {
	bnbTipNumber, err := r.client.BlockNumber()
	if err != nil {
		return err
	}
	if bnbTipNumber > r.lorenzoTip.Number.Uint64() {
		return nil
	}
	defer func(starTime time.Time) {
		r.logger.Infof("Wait BNB tip: %d catch up to Lorenzo tip: %d, time used: %v",
			bnbTipNumber, r.lorenzoTip.Number.Uint64(), time.Since(starTime))
	}(time.Now())

	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		bnbTipNumber, err := r.client.BlockNumber()
		if err != nil {
			return err
		}
		if bnbTipNumber > r.lorenzoTip.Number.Uint64() {
			break
		}
	}
	return nil
}

func (r *BNBReporter) WaitLorenzoCatchUp() error {
	bnbTip, err := r.client.LatestHeader()
	if err != nil {
		return err
	}
	if r.lorenzoTip.Number.Uint64()+r.delayBlocks >= bnbTip.Number.Uint64() {
		return nil
	}
	catchUpToNumber := bnbTip.Number.Uint64() - r.delayBlocks
	defer func(starTime time.Time) {
		r.logger.Infof("Wait Lorenzo tip: %d catch up to BNB  close tip: %d, BNB tip:%d, time used: %v",
			r.lorenzoTip.Number.Uint64(), catchUpToNumber, bnbTip.Number.Uint64(), time.Since(starTime))
	}(time.Now())

	batchHeaderCh := make(chan []*bnbtypes.Header, 10)
	go func() {
		defer close(batchHeaderCh)
		for i := r.lorenzoTip.Number.Uint64() + 1; i <= catchUpToNumber; i += FetchBNBHeaderBatchSize {
			select {
			case <-r.quit:
				return
			default:
			}

			end := i + FetchBNBHeaderBatchSize - 1
			if end > catchUpToNumber {
				end = catchUpToNumber
			}
			headers, err := r.client.RangeHeaders(i, end)
			if err != nil {
				r.logger.Warnf("failed to get BNB headers from %d to %d: %v", i, end, err)
				return
			}
			batchHeaderCh <- headers
			time.Sleep(time.Second)
		}
	}()
	for {
		select {
		case <-r.quit:
			return nil
		default:
		}

		headers, ok := <-batchHeaderCh
		if !ok && len(headers) == 0 {
			break
		}
		if err := r.handleHeaders(headers); err != nil {
			panic(err)
		}
	}

	return nil
}

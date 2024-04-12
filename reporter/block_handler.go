package reporter

import (
	"time"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/types"
)

const (
	BlockEventCheckInterval = time.Minute
)

// mainLoop handles connected and disconnected blocks from the BTC client.
func (r *Reporter) mainLoop() {
	defer r.wg.Done()
	quit := r.quitChan()

	for {
		select {
		case <-quit:
			// We have been asked to stop
			return
		default:
		}

		_, btcTip, err := r.btcClient.GetBestBlock()
		if err != nil {
			continue
		}
		lorenzoBtcTip := uint64(r.lorenzoBtcTip.Height)
		if lorenzoBtcTip+r.delayBlocks > btcTip {
			time.Sleep(BlockEventCheckInterval)
			continue
		}

		ib, _, err := r.btcClient.GetBlockByHeight(lorenzoBtcTip + 1)
		lorenzoBtcTipBlockHash := r.lorenzoBtcTip.Header.BlockHash()
		if ib.Header.PrevBlock.IsEqual(&lorenzoBtcTipBlockHash) == false {
			r.logger.Warnf("BTC chain reorg detected, restart bootstrap process. Height:%d", ib.Height)
			r.bootstrapWithRetries()
			continue
		}

		_, err = r.ProcessHeaders(r.lorenzoClient.MustGetAddr(), []*types.IndexedBlock{ib})
		if err != nil {
			r.logger.Warnf("Failed to submit header: %v, restart bootstrap process", err)
			r.bootstrapWithRetries()
			continue
		}

		r.lorenzoBtcTip = ib
	}
}

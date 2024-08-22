package bnbreporter

import (
	"sync"

	"go.uber.org/zap"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/v2/bnbclient"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/v2/bnbclient/bnbtypes"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/v2/config"
)

const DefaultBNBDelayBlocks = 15

type BNBReporter struct {
	cfg           *config.BNBReporterConfig
	logger        *zap.SugaredLogger
	delayBlocks   uint64
	lorenzoClient LorenzoClient
	client        bnbclient.BNBClient

	wg         sync.WaitGroup
	quit       chan struct{}
	lorenzoTip *bnbtypes.Header // Last BNB BlockNumber reported to Lorenzo
}

func New(parentLogger *zap.Logger, lorenzoClient LorenzoClient, cfg *config.BNBReporterConfig) (*BNBReporter, error) {
	logger := parentLogger.With(zap.String("module", "BNB-reporter")).Sugar()

	client, err := bnbclient.New(cfg.RpcUrl)
	if err != nil {
		return nil, err
	}

	if cfg.DelayBlocks == 0 {
		cfg.DelayBlocks = DefaultBNBDelayBlocks
	}

	return &BNBReporter{
		cfg:           cfg,
		logger:        logger,
		delayBlocks:   cfg.DelayBlocks,
		lorenzoClient: lorenzoClient,
		client:        client,
		quit:          make(chan struct{}),
	}, nil
}

func (r *BNBReporter) Start() {
	select {
	case <-r.quit:
		r.logger.Info("BNB reporter already stopped")
		return
	default:
	}

	if err := r.boostrap(); err != nil {
		panic(err)
	}

	if err := r.WaitLorenzoCatchUp(); err != nil {
		panic(err)
	}
	if err := r.WaitBNBCatchUp(); err != nil {
		panic(err)
	}

	// Start the reporter
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.mainLoop()
	}()
}

func (r *BNBReporter) Stop() {
	select {
	case <-r.quit:
	default:
		close(r.quit)
	}
}

func (r *BNBReporter) WaitForShutdown() {
	r.wg.Wait()
}

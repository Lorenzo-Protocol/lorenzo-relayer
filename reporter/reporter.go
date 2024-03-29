package reporter

import (
	"sync"
	"time"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/btcclient"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/config"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/metrics"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/types"
	"go.uber.org/zap"
)

type Reporter struct {
	Cfg    *config.ReporterConfig
	logger *zap.SugaredLogger

	btcClient     btcclient.BTCClient
	lorenzoClient LorenzoClient

	// retry attributes
	retrySleepTime    time.Duration
	maxRetrySleepTime time.Duration

	// Internal states of the reporter
	btcCache                      *types.BTCCache
	reorgList                     *reorgList
	btcConfirmationDepth          uint64
	checkpointFinalizationTimeout uint64
	metrics                       *metrics.ReporterMetrics
	wg                            sync.WaitGroup
	started                       bool
	quit                          chan struct{}
	quitMu                        sync.Mutex
}

func New(
	cfg *config.ReporterConfig,
	parentLogger *zap.Logger,
	btcClient btcclient.BTCClient,
	lorenzoClient LorenzoClient,
	retrySleepTime,
	maxRetrySleepTime time.Duration,
	metrics *metrics.ReporterMetrics,
) (*Reporter, error) {
	logger := parentLogger.With(zap.String("module", "reporter")).Sugar()

	return &Reporter{
		Cfg:               cfg,
		logger:            logger,
		retrySleepTime:    retrySleepTime,
		maxRetrySleepTime: maxRetrySleepTime,
		btcClient:         btcClient,
		lorenzoClient:     lorenzoClient,
		reorgList:         newReorgList(),
		//TODO: get from config file
		btcConfirmationDepth:          10,
		checkpointFinalizationTimeout: 100,
		metrics:                       metrics,
		quit:                          make(chan struct{}),
	}, nil
}

// Start starts the goroutines necessary to manage a vigilante.
func (r *Reporter) Start() {
	r.quitMu.Lock()
	select {
	case <-r.quit:
		// Restart the vigilante goroutines after shutdown finishes.
		r.WaitForShutdown()
		r.quit = make(chan struct{})
	default:
		// Ignore when the vigilante is still running.
		if r.started {
			r.quitMu.Unlock()
			return
		}
		r.started = true
	}
	r.quitMu.Unlock()

	if err := r.waitLorenzoCatchUpCloseToBTCTip(); err != nil {
		panic(err)
	}

	r.bootstrapWithRetries(false)

	r.wg.Add(1)
	go r.blockEventHandler()

	// start record time-related metrics
	r.metrics.RecordMetrics()

	r.logger.Infof("Successfully started the vigilant reporter")
}

// quitChan atomically reads the quit channel.
func (r *Reporter) quitChan() <-chan struct{} {
	r.quitMu.Lock()
	c := r.quit
	r.quitMu.Unlock()
	return c
}

// Stop signals all vigilante goroutines to shutdown.
func (r *Reporter) Stop() {
	r.quitMu.Lock()
	quit := r.quit
	r.quitMu.Unlock()

	select {
	case <-quit:
	default:
		// closing the `quit` channel will trigger all select case `<-quit`,
		// and thus making all handler routines to break the for loop.
		close(quit)
	}
}

// ShuttingDown returns whether the vigilante is currently in the process of shutting down or not.
func (r *Reporter) ShuttingDown() bool {
	select {
	case <-r.quitChan():
		return true
	default:
		return false
	}
}

// WaitForShutdown blocks until all vigilante goroutines have finished executing.
func (r *Reporter) WaitForShutdown() {
	// TODO: let Lorenzo client WaitForShutDown
	r.wg.Wait()
}

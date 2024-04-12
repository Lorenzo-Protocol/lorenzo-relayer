package btcclient

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/config"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/netparams"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/types"
	"github.com/btcsuite/btcd/rpcclient"
)

// NewQueryClient creates a new BTC client that subscribes to newly connected/disconnected blocks
// used by vigilant reporter
func NewQueryClient(cfg *config.BTCConfig, retrySleepTime, maxRetrySleepTime time.Duration, parentLogger *zap.Logger) (*Client, error) {
	client := &Client{}
	params, err := netparams.GetBTCParams(cfg.NetParams)
	if err != nil {
		return nil, err
	}
	client.Cfg = cfg
	client.Params = params
	logger := parentLogger.With(zap.String("module", "btcclient"))
	client.logger = logger.Sugar()

	client.retrySleepTime = retrySleepTime
	client.maxRetrySleepTime = maxRetrySleepTime

	connCfg := &rpcclient.ConnConfig{
		Host:         cfg.Endpoint,
		HTTPPostMode: true,
		User:         cfg.Username,
		Pass:         cfg.Password,
		DisableTLS:   cfg.DisableClientTLS,
	}

	if cfg.BtcBackend == types.Btcd {
		connCfg.Endpoint = "ws" // websocket
		connCfg.Certificates = cfg.ReadCAFile()
	}

	rpcClient, err := rpcclient.New(connCfg, nil)
	if err != nil {
		return nil, err
	}

	backend, err := rpcClient.BackendVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to get BTC backend: %v", err)
	}
	client.logger.Infof("Connected to BTC backendVersion: %d", backend)
	client.Client = rpcClient

	client.logger.Info("Successfully created the BTC client and connected to the BTC server")
	return client, nil
}

package btcclient

import (
	"fmt"
	"time"

	"github.com/Lorenzo-Protocol/lorenzo/v3/types/retry"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	"go.uber.org/zap"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/v2/config"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/v2/netparams"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/v2/types"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/v2/zmq"
)

// NewWithBlockSubscriber creates a new BTC client that subscribes to newly connected/disconnected blocks
// used by vigilant reporter
func NewWithBlockSubscriber(cfg *config.BTCConfig, retrySleepTime, maxRetrySleepTime time.Duration, parentLogger *zap.Logger) (*Client, error) {
	client := &Client{}
	params, err := netparams.GetBTCParams(cfg.NetParams)
	if err != nil {
		return nil, err
	}
	client.blockEventChan = make(chan *types.BlockEvent, 10000) // TODO: parameterise buffer size
	client.Cfg = cfg
	client.Params = params
	logger := parentLogger.With(zap.String("module", "btcclient"))
	client.logger = logger.Sugar()

	client.retrySleepTime = retrySleepTime
	client.maxRetrySleepTime = maxRetrySleepTime

	switch cfg.BtcBackend {
	case types.Bitcoind:
		// TODO Currently we are not using Params field of rpcclient.ConnConfig due to bug in btcd
		// when handling signet.
		connCfg := &rpcclient.ConnConfig{
			Host:         cfg.Endpoint,
			HTTPPostMode: true,
			User:         cfg.Username,
			Pass:         cfg.Password,
			DisableTLS:   cfg.DisableClientTLS,
		}

		rpcClient, err := rpcclient.New(connCfg, nil)
		if err != nil {
			return nil, err
		}

		// ensure we are using bitcoind as Bitcoin node, as zmq is only supported by bitcoind
		backend, err := rpcClient.BackendVersion()
		if err != nil {
			return nil, fmt.Errorf("failed to get BTC backend: %v", err)
		}
		switch backend.(type) {
		case *rpcclient.BitcoindVersion, rpcclient.BitcoindVersion:
		default:
			return nil, fmt.Errorf("zmq is only supported by bitcoind, but got %v", backend)
		}

		zmqClient, err := zmq.New(logger, cfg.ZmqSeqEndpoint, client.blockEventChan, rpcClient)
		if err != nil {
			return nil, err
		}

		client.zmqClient = zmqClient
		client.Client = rpcClient
	case types.Btcd:
		notificationHandlers := rpcclient.NotificationHandlers{
			OnFilteredBlockConnected: func(height int32, header *wire.BlockHeader, txs []*btcutil.Tx) {
				client.logger.Debugf("Block %v at height %d has been connected at time %v", header.BlockHash(), height, header.Timestamp)
				client.blockEventChan <- types.NewBlockEvent(types.BlockConnected, height, header)
			},
			OnFilteredBlockDisconnected: func(height int32, header *wire.BlockHeader) {
				client.logger.Debugf("Block %v at height %d has been disconnected at time %v", header.BlockHash(), height, header.Timestamp)
				client.blockEventChan <- types.NewBlockEvent(types.BlockDisconnected, height, header)
			},
		}

		certificates, err := cfg.ReadCAFile()
		if err != nil {
			return nil, err
		}
		// TODO Currently we are not using Params field of rpcclient.ConnConfig due to bug in btcd
		// when handling signet.
		connCfg := &rpcclient.ConnConfig{
			Host:         cfg.Endpoint,
			Endpoint:     "ws", // websocket
			User:         cfg.Username,
			Pass:         cfg.Password,
			DisableTLS:   cfg.DisableClientTLS,
			Certificates: certificates,
		}

		rpcClient, err := rpcclient.New(connCfg, &notificationHandlers)
		if err != nil {
			return nil, err
		}

		// ensure we are using btcd as Bitcoin node, since Websocket-based subscriber is only available in btcd
		backend, err := rpcClient.BackendVersion()
		if err != nil {
			return nil, fmt.Errorf("failed to get BTC backend: %v", err)
		}
		switch backend.(type) {
		case *rpcclient.BtcdVersion, rpcclient.BtcdVersion:
		default:
			return nil, fmt.Errorf("websocket is only supported by btcd, but got %v", backend)
		}

		client.Client = rpcClient
	}

	client.logger.Info("Successfully created the BTC client and connected to the BTC server")

	return client, nil
}

func (c *Client) subscribeBlocksByWebSocket() error {
	if err := c.NotifyBlocks(); err != nil {
		return err
	}
	c.logger.Info("Successfully subscribed to newly connected/disconnected blocks via WebSocket")
	return nil
}

func (c *Client) mustSubscribeBlocksByWebSocket() {
	if err := retry.Do(c.retrySleepTime, c.maxRetrySleepTime, func() error {
		return c.subscribeBlocksByWebSocket()
	}); err != nil {
		panic(err)
	}
}

func (c *Client) mustSubscribeBlocksByZmq() {
	if err := c.zmqClient.SubscribeSequence(); err != nil {
		panic(err)
	}
}

func (c *Client) MustSubscribeBlocks() {
	switch c.Cfg.BtcBackend {
	case types.Btcd:
		c.mustSubscribeBlocksByWebSocket()
	case types.Bitcoind:
		c.mustSubscribeBlocksByZmq()
	}
}

func (c *Client) BlockEventChan() <-chan *types.BlockEvent {
	return c.blockEventChan
}

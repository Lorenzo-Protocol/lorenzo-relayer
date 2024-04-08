package config

import (
	"errors"
	"os"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/types"
)

// BTCConfig defines configuration for the Bitcoin client
type BTCConfig struct {
	DisableClientTLS  bool                      `mapstructure:"no-client-tls"`
	CAFile            string                    `mapstructure:"ca-file"`
	Endpoint          string                    `mapstructure:"endpoint"`
	NetParams         string                    `mapstructure:"net-params"`
	Username          string                    `mapstructure:"username"`
	Password          string                    `mapstructure:"password"`
	ReconnectAttempts int                       `mapstructure:"reconnect-attempts"`
	BtcBackend        types.SupportedBtcBackend `mapstructure:"btc-backend"`
	ZmqSeqEndpoint    string                    `mapstructure:"zmq-seq-endpoint"`
}

func (cfg *BTCConfig) Validate() error {
	if cfg.ReconnectAttempts < 0 {
		return errors.New("reconnect-attempts must be non-negative")
	}

	if _, ok := types.GetValidNetParams()[cfg.NetParams]; !ok {
		return errors.New("invalid net params")
	}

	if _, ok := types.GetValidBtcBackends()[cfg.BtcBackend]; !ok {
		return errors.New("invalid btc backend")
	}

	if cfg.BtcBackend == types.Bitcoind {
		// TODO: implement regex validation for zmq endpoint
		if cfg.ZmqSeqEndpoint == "" {
			return errors.New("zmq seq endpoint cannot be empty")
		}
	}

	return nil
}

const (
	// Config for polling jittner in bitcoind client, with polling enabled
	DefaultRpcBtcNodeHost = "127.0.01:18556"
	DefaultBtcNodeRpcUser = "rpcuser"
	DefaultBtcNodeRpcPass = "rpcpass"
	DefaultZmqSeqEndpoint = "tcp://127.0.0.1:29000"
)

func DefaultBTCConfig() BTCConfig {
	return BTCConfig{
		DisableClientTLS:  false,
		CAFile:            defaultBtcCAFile,
		Endpoint:          DefaultRpcBtcNodeHost,
		BtcBackend:        types.Btcd,
		NetParams:         types.BtcSimnet.String(),
		Username:          DefaultBtcNodeRpcUser,
		Password:          DefaultBtcNodeRpcPass,
		ReconnectAttempts: 3,
		ZmqSeqEndpoint:    DefaultZmqSeqEndpoint,
	}
}

func (cfg *BTCConfig) ReadCAFile() []byte {
	if cfg.DisableClientTLS {
		return nil
	}

	// Read certificate file if TLS is not disabled.
	certs, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		// If there's an error reading the CA file, continue
		// with nil certs and without the client connection.
		return nil
	}

	return certs
}

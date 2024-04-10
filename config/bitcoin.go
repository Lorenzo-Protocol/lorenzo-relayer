package config

import (
	"errors"
	"os"

	"github.com/lightningnetwork/lnd/lnwallet/chainfee"

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
	TxFeeMin          chainfee.SatPerKVByte     `mapstructure:"tx-fee-min"`       // minimum tx fee, sat/kvb
	TxFeeMax          chainfee.SatPerKVByte     `mapstructure:"tx-fee-max"`       // maximum tx fee, sat/kvb
	DefaultFee        chainfee.SatPerKVByte     `mapstructure:"default-fee"`      // default BTC tx fee in case estimation fails, sat/kvb
	EstimateMode      string                    `mapstructure:"estimate-mode"`    // the BTC tx fee estimate mode, which is only used by bitcoind, must be either ECONOMICAL or CONSERVATIVE
	TargetBlockNum    int64                     `mapstructure:"target-block-num"` // this implies how soon the tx is estimated to be included in a block, e.g., 1 means the tx is estimated to be included in the next block
	WalletEndpoint    string                    `mapstructure:"wallet-endpoint"`
	WalletName        string                    `mapstructure:"wallet-name"`
	WalletPassword    string                    `mapstructure:"wallet-password"`
	WalletCAFile      string                    `mapstructure:"wallet-ca-file"`
	WalletLockTime    int64                     `mapstructure:"wallet-lock-time"` // time duration in which the wallet remains unlocked, in seconds

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

func (cfg *BTCConfig) ReadWalletCAFile() []byte {
	if cfg.DisableClientTLS {
		// Chain server RPC TLS is disabled
		return nil
	}

	// Read certificate file if TLS is not disabled.
	certs, err := os.ReadFile(cfg.WalletCAFile)
	if err != nil {
		// If there's an error reading the CA file, continue
		// with nil certs and without the client connection.
		return nil
	}
	return certs
}

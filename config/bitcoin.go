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

	return nil
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

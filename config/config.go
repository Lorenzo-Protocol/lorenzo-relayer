package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	lrzcfg "github.com/Lorenzo-Protocol/lorenzo-sdk/config"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

const (
	defaultConfigFilename = "vigilante.yml"
)

var (
	defaultBtcCAFile  = filepath.Join(btcutil.AppDataDir("btcd", false), "rpc.cert")
	defaultAppDataDir = btcutil.AppDataDir("lorenzo-vigilante", false)
	defaultConfigFile = filepath.Join(defaultAppDataDir, defaultConfigFilename)
)

// Config defines the server's top level configuration
type Config struct {
	Common   CommonConfig         `mapstructure:"common"`
	BTC      BTCConfig            `mapstructure:"btc"`
	Lorenzo  lrzcfg.LorenzoConfig `mapstructure:"lorenzo"`
	Metrics  MetricsConfig        `mapstructure:"metrics"`
	Reporter ReporterConfig       `mapstructure:"reporter"`
}

func (cfg *Config) Validate() error {
	if err := cfg.Common.Validate(); err != nil {
		return fmt.Errorf("invalid config in common: %w", err)
	}

	if err := cfg.BTC.Validate(); err != nil {
		return fmt.Errorf("invalid config in btc: %w", err)
	}

	if err := cfg.Lorenzo.Validate(); err != nil {
		return fmt.Errorf("invalid config in lorenzo: %w", err)
	}

	if err := cfg.Metrics.Validate(); err != nil {
		return fmt.Errorf("invalid config in metrics: %w", err)
	}

	if err := cfg.Reporter.Validate(); err != nil {
		return fmt.Errorf("invalid config in reporter: %w", err)
	}

	return nil
}

func (cfg *Config) CreateLogger() (*zap.Logger, error) {
	return cfg.Common.CreateLogger()
}

func DefaultConfigFile() string {
	return defaultConfigFile
}

// New returns a fully parsed Config object from a given file directory
func New(configFile string) (Config, error) {
	if _, err := os.Stat(configFile); err == nil { // the given file exists, parse it
		viper.SetConfigFile(configFile)
		if err := viper.ReadInConfig(); err != nil {
			return Config{}, err
		}
		var cfg Config
		if err := viper.Unmarshal(&cfg); err != nil {
			return Config{}, err
		}
		if err := cfg.Validate(); err != nil {
			return Config{}, err
		}
		return cfg, err
	} else if errors.Is(err, os.ErrNotExist) { // the given config file does not exist, return error
		return Config{}, fmt.Errorf("no config file found at %s", configFile)
	} else { // other errors
		return Config{}, err
	}
}

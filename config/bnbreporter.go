package config

import (
	"errors"
	"fmt"
)

type BNBReporterConfig struct {
	RpcUrl      string `mapstructure:"rpc_url"`
	DelayBlocks uint64 `mapstructure:"delay_blocks"`
	BaseHeight  uint64 `mapstructure:"base_height"`
}

func (cfg *BNBReporterConfig) Validate() error {
	if cfg.RpcUrl == "" {
		return fmt.Errorf("rpc url cannot be empty")
	}
	if cfg.BaseHeight == 0 {
		return errors.New("BNB base height cannot be 0")
	}
	if cfg.DelayBlocks == 0 {
		return errors.New("BNB delay blocks cannot be 0")
	}

	return nil
}

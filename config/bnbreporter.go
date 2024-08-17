package config

import "fmt"

type BNBReporterConfig struct {
	RpcUrl      string `mapstructure:"rpc_url"`
	DelayBlocks uint64 `mapstructure:"delay_blocks"`
	BaseHeight  uint64 `mapstructure:"base_height"`
}

func (cfg *BNBReporterConfig) Validate() error {
	if cfg.RpcUrl == "" {
		return fmt.Errorf("rpc url cannot be empty")
	}
	return nil
}

package cmd

import (
	"fmt"
	"strconv"

	client_config "github.com/Lorenzo-Protocol/lorenzo-sdk/config"
	lrzqc "github.com/Lorenzo-Protocol/lorenzo-sdk/query"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/spf13/cobra"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/btcclient"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/btcclient/sender"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/config"
)

// returns the CLI commands for the send btc
func SendBTCCmd() *cobra.Command {
	var targetAddress string
	var amount string
	var cfgFile = ""

	cmd := &cobra.Command{
		Use:   "send-btc <amount>",
		Short: "send BTC to Lorenzo vault",
		Run: func(_ *cobra.Command, _ []string) {
			var (
				err           error
				cfg           config.Config
				lorenzoClient *lrzqc.QueryClient
			)

			// get the config from the given file or the default file
			cfg, err = config.New(cfgFile)
			if err != nil {
				panic(fmt.Errorf("failed to load config: %w", err))
			}
			rootLogger, err := cfg.CreateLogger()
			if err != nil {
				panic(fmt.Errorf("failed to create logger: %w", err))
			}

			// create Lorenzo client. Note that requests from Lorenzo client are ad hoc
			queryCfg := &client_config.LorenzoQueryConfig{
				RPCAddr: cfg.Lorenzo.RPCAddr,
				Timeout: cfg.Lorenzo.Timeout,
			}
			err = queryCfg.Validate()
			if err != nil {
				panic(fmt.Errorf("invalid config for the query client: %w", err))
			}

			lorenzoClient, err = lrzqc.New(queryCfg)
			if err != nil {
				panic(fmt.Errorf("failed to open Lorenzo client: %w", err))
			}
			btcStakingParams, err := lorenzoClient.QueryBTCStakingParams()
			if err != nil {
				panic(fmt.Errorf("failed to get btcstaking params: %w", err))
			}
			// TODO: btc chain cfg
			vaultAddress, err := btcutil.DecodeAddress(btcStakingParams.Params.BtcReceivingAddr, nil)
			if err != nil {
				panic(fmt.Errorf("failed to parse vault address: %w", err))
			}

			// create BTC wallet and connect to BTC server
			btcWallet, err := btcclient.NewWallet(&cfg.BTC, rootLogger)
			if err != nil {
				panic(fmt.Errorf("failed to open BTC client: %w", err))
			}
			btcCfg := btcWallet.GetBTCConfig()
			est, err := sender.NewFeeEstimator(btcCfg)
			if err != nil {
				panic(fmt.Errorf("failed to create fee estimator: %w", err))
			}

			sender := sender.New(
				btcWallet,
				vaultAddress,
				est,
				rootLogger,
			)
			amount_int, err := strconv.ParseInt(amount, 10, 64)
			if err != nil {
				panic(fmt.Errorf("failed to parse amount: %w", err))
			}
			tx, err := sender.SendBTCtoLorenzoVault([]byte(targetAddress), btcutil.Amount(amount_int))
			if err != nil {
				panic(fmt.Errorf("failed to send btc: %w", err))
			}
			fmt.Println("success, txid: %w", tx.TxId)
		},
	}
	cmd.Flags().StringVar(&targetAddress, "target-address", "", "target address on lorenzo chain.")
	cmd.Flags().StringVar(&amount, "amount", "", "amount, an integer number, unit is sats.")
	cmd.Flags().StringVar(&cfgFile, "config", config.DefaultConfigFile(), "config file")
	return cmd
}

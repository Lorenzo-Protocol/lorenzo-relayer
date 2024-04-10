package cmd

import (
	"fmt"
	"strconv"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/btcclient"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/btcclient/sender"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/config"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/netparams"
	client_config "github.com/Lorenzo-Protocol/lorenzo-sdk/config"
	lrzqc "github.com/Lorenzo-Protocol/lorenzo-sdk/query"
)

// returns the CLI commands for the send btc
func SendBTCCmd() *cobra.Command {
	var cfgFile = ""

	cmd := &cobra.Command{
		Use:   "send-btc <target-address> <amount>",
		Short: "send BTC to Lorenzo vault",
		Args:  cobra.ExactArgs(2),
		Run: func(_ *cobra.Command, args []string) {
			var (
				err           error
				cfg           config.Config
				lorenzoClient *lrzqc.QueryClient
			)

			amount, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				panic(fmt.Errorf("failed to parse amount: %w", err))
			}
			if !common.IsHexAddress(args[0]) {
				panic(fmt.Errorf("invalid target address: %s", args[0]))
			}
			targetAddress := common.HexToAddress(args[0])

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

			btcParams, err := netparams.GetBTCParams(cfg.BTC.NetParams)
			if err != nil {
				panic(fmt.Errorf("failed to parse btc params: %w", err))
			}
			vaultAddress, err := btcutil.DecodeAddress(btcStakingParams.Params.BtcReceivingAddr, btcParams)
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
			panic(fmt.Errorf("vaultAddress: %w, target: %w, amount: %w ", vaultAddress, targetAddress, amount))

			sender := sender.New(
				btcWallet,
				vaultAddress,
				est,
				rootLogger,
			)
			tx, err := sender.SendBTCtoLorenzoVault(targetAddress.Bytes(), btcutil.Amount(amount))
			if err != nil {
				panic(fmt.Errorf("failed to send btc: %w", err))
			}
			fmt.Println("success, txid: %w", tx.TxId)
		},
	}
	cmd.Flags().StringVar(&cfgFile, "config", config.DefaultConfigFile(), "config file")
	return cmd
}

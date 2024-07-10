package cmd

import (
	"fmt"

	lrzclient "github.com/Lorenzo-Protocol/lorenzo-sdk/client"
	"github.com/spf13/cobra"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/btcclient"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/config"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/metrics"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/reporter"
)

// GetReporterCmd returns the CLI commands for the reporter
func GetReporterCmd() *cobra.Command {
	var lorenzoKeyDir string
	var cfgFile = ""

	cmd := &cobra.Command{
		Use:   "reporter",
		Short: "Lrzrelayer reporter",
		Run: func(_ *cobra.Command, _ []string) {
			var (
				err              error
				cfg              config.Config
				btcClient        *btcclient.Client
				lorenzoClient    *lrzclient.Client
				vigilantReporter *reporter.Reporter
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

			// apply the flags from CLI
			if len(lorenzoKeyDir) != 0 {
				cfg.Lorenzo.KeyDirectory = lorenzoKeyDir
			}

			// create BTC client and connect to BTC server
			// Note that vigilant reporter needs to subscribe to new BTC blocks
			btcClient, err = btcclient.NewWithBlockSubscriber(&cfg.BTC, cfg.Common.RetrySleepTime, cfg.Common.MaxRetrySleepTime, rootLogger)
			if err != nil {
				panic(fmt.Errorf("failed to open BTC client: %w", err))
			}

			// create Lorenzo client. Note that requests from Lorenzo client are ad hoc
			lorenzoClient, err = lrzclient.New(&cfg.Lorenzo, nil)
			if err != nil {
				panic(fmt.Errorf("failed to open Lorenzo client: %w", err))
			}

			// register reporter metrics
			reporterMetrics := metrics.NewReporterMetrics()

			// create reporter
			vigilantReporter, err = reporter.New(
				&cfg.Reporter,
				rootLogger,
				btcClient,
				lorenzoClient,
				cfg.Common.RetrySleepTime,
				cfg.Common.MaxRetrySleepTime,
				reporterMetrics,
			)
			if err != nil {
				panic(fmt.Errorf("failed to create rlzrelayer reporter: %w", err))
			}

			// start normal-case execution
			vigilantReporter.Start()

			// start Prometheus metrics server
			addr := fmt.Sprintf("%s:%d", cfg.Metrics.Host, cfg.Metrics.ServerPort)
			metrics.Start(addr, reporterMetrics.Registry)

			// SIGINT handling stuff
			addInterruptHandler(func() {
				rootLogger.Info("Stopping reporter...")
				vigilantReporter.Stop()
				vigilantReporter.WaitForShutdown()
				rootLogger.Info("Reporter shutdown")
			})
			addInterruptHandler(func() {
				rootLogger.Info("Stopping BTC client...")
				btcClient.Stop()
				btcClient.WaitForShutdown()
				rootLogger.Info("BTC client shutdown")
			})

			<-interruptHandlersDone
			rootLogger.Info("Shutdown complete")
		},
	}
	cmd.Flags().StringVar(&lorenzoKeyDir, "lorenzo-key-dir", "", "Directory of the Lorenzo key")
	cmd.Flags().StringVar(&cfgFile, "config", config.DefaultConfigFile(), "config file")
	return cmd
}

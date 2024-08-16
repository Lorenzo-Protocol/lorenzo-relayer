package cmd

import (
	"fmt"

	lrzclient "github.com/Lorenzo-Protocol/lorenzo-sdk/v3/client"
	"github.com/spf13/cobra"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/v2/bnbreporter"
	"github.com/Lorenzo-Protocol/lorenzo-relayer/v2/config"
)

func GetBNBReporterCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bnbreporter",
		Short: "Lrzrelayer BNB reporter",
		Run:   bnbReporterAction,
	}
	cmd.Flags().String("config", config.DefaultConfigFile(), "config file")

	return cmd
}

func bnbReporterAction(cmd *cobra.Command, args []string) {
	cfgFile, _ := cmd.Flags().GetString("config")
	cfg, err := config.New(cfgFile)
	if err != nil {
		panic(err)
	}
	rootLogger, err := cfg.CreateLogger()
	if err != nil {
		panic(fmt.Errorf("failed to create logger: %w", err))
	}

	// create Lorenzo client. Note that requests from Lorenzo client are ad hoc
	lorenzoClient, err := lrzclient.New(&cfg.Lorenzo, nil)
	if err != nil {
		panic(fmt.Errorf("failed to open Lorenzo client: %w", err))
	}

	bnbReporter, err := bnbreporter.New(rootLogger, lorenzoClient, cfg.BNBReporter.DelayBlocks, cfg.BNBReporter.RpcUrl)
	if err != nil {
		panic(fmt.Errorf("failed to create BNB reporter: %w", err))
	}
	bnbReporter.Start()

	addInterruptHandler(func() {
		rootLogger.Info("Stopping BNB reporter...")
		bnbReporter.Stop()
		bnbReporter.WaitForShutdown()
		rootLogger.Info("Reporter BNB shutdown")
	})
	<-interruptHandlersDone
	rootLogger.Info("Shutdown complete")
}

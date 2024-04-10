package cmd

import (
	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "lrzrelayer",
		Short: "Lorenzo relayer",
	}
	rootCmd.AddCommand(
		GetReporterCmd(),
		SendBTCCmd(),
	)

	return rootCmd
}

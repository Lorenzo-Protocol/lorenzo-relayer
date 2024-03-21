package cmd

import (
	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "vigilante",
		Short: "Lorenzo vigilante",
	}
	rootCmd.AddCommand(
		GetReporterCmd(),
	)

	return rootCmd
}

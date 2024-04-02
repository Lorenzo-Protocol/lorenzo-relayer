package main

import (
	"fmt"
	"os"

	"github.com/Lorenzo-Protocol/lorenzo/app/params"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/cmd/lrzrelayer/cmd"
)

func main() {
	params.SetAddressPrefixes()

	rootCmd := cmd.NewRootCmd()

	if err := rootCmd.Execute(); err != nil {
		switch e := err.(type) {
		// TODO: dedicated error codes for lrzrelayer
		default:
			fmt.Print(e.Error())
			os.Exit(1)
		}
	}
}

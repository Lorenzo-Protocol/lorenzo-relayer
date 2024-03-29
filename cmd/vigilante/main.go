package main

import (
	"fmt"
	"os"

	"github.com/Lorenzo-Protocol/lorenzo-relayer/cmd/vigilante/cmd"
	"github.com/Lorenzo-Protocol/lorenzo/app/params"
)

// TODO: init log

func main() {
	params.SetAddressPrefixes()

	rootCmd := cmd.NewRootCmd()

	if err := rootCmd.Execute(); err != nil {
		switch e := err.(type) {
		// TODO: dedicated error codes for vigilantes
		default:
			fmt.Print(e.Error())
			os.Exit(1)
		}
	}
}

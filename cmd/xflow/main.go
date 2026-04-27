package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/xtra/xflow/internal/cli"
)

func main() {
	rootCmd := cli.NewRootCmd()
	rootCmd.SilenceErrors = true

	if err := rootCmd.Execute(); err != nil {
		var cliErr *cli.CLIError
		if errors.As(err, &cliErr) {
			verbose, _ := rootCmd.PersistentFlags().GetBool("verbose")
			fmt.Fprintln(os.Stderr, cli.FormatError(cliErr, verbose))
			os.Exit(cliErr.ExitCode)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

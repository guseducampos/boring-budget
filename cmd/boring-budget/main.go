package main

import (
	"fmt"
	"os"

	"boring-budget/internal/cli"
	"boring-budget/internal/cli/output"
)

func main() {
	output.ResetProcessExitCode()

	if err := cli.NewRootCmd().Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		code := output.CurrentProcessExitCode()
		if code > 0 {
			os.Exit(code)
		}
		os.Exit(1)
	}

	code := output.CurrentProcessExitCode()
	if code > 0 {
		os.Exit(code)
	}
}

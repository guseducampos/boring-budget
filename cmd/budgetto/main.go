package main

import (
	"fmt"
	"os"

	"budgetto/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

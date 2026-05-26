package main

import (
	"os"

	"github.com/helopony/bikebook-cli/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}

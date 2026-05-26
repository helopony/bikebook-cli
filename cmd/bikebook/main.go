package main

import (
	"os"

	"github.com/helopony/bikebook-cli/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}

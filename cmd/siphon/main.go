package main

import (
	"os"

	"siphon/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}

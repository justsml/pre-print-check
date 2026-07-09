package main

import (
	"os"

	"github.com/justsml/pre-print-check/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}

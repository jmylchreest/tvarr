// Package main is the entry point for the tvarr application.
package main

import (
	"os"

	"github.com/jmylchreest/tvarr/cmd/tvarr/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

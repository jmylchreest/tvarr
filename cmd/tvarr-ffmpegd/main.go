// Package main is the entry point for the tvarr-ffmpegd daemon.
//
// tvarr-ffmpegd is a distributed transcoding daemon that connects to a
// tvarr coordinator to provide transcoding capacity. It reports its
// hardware capabilities (GPU encoders, session limits) and accepts
// bidirectional streaming of ES samples for transcoding.
package main

import (
	"os"

	"github.com/jmylchreest/tvarr/cmd/tvarr-ffmpegd/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

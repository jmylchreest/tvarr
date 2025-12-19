// Package types defines shared types for the tvarr-ffmpegd distributed transcoding system.
//
// This package is part of the public API and can be imported by third-party clients
// to build custom coordinators or daemons.
//
// Core types:
//   - Daemon: Represents a registered transcoding worker
//   - Capabilities: Hardware and software capabilities of a daemon
//   - TranscodeJob: An active transcoding session
//   - SystemStats: System metrics reported by daemons
//   - ESSample: Elementary stream sample for video/audio transport
//
// Example usage:
//
//	import "github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
//
//	daemon := &types.Daemon{
//	    ID:   types.DaemonID("my-daemon-1"),
//	    Name: "GPU Transcoder",
//	    State: types.DaemonStateConnected,
//	    Capabilities: &types.Capabilities{
//	        VideoEncoders: []string{"libx264", "h264_nvenc"},
//	        MaxConcurrentJobs: 4,
//	    },
//	}
package types

// Version is the types package version.
const Version = "1.0.0"

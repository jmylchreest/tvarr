// Package util provides shared utility functions.
package util

import (
	"fmt"
	"os"
	"os/exec"
)

// FindBinary searches for an executable binary by name.
// Search order:
//  1. Environment variable (if envVar is non-empty and set)
//  2. ./name (current directory, useful for development)
//  3. name on PATH (via exec.LookPath)
//
// Each path is verified to exist and be executable before being returned.
// Returns the path to the binary or an error if not found.
func FindBinary(name string, envVar string) (string, error) {
	// 1. Check environment variable
	if envVar != "" {
		if envPath := os.Getenv(envVar); envPath != "" {
			if isExecutable(envPath) {
				return envPath, nil
			}
		}
	}

	// 2. Check current directory
	localPath := "./" + name
	if isExecutable(localPath) {
		return localPath, nil
	}

	// 3. Find on PATH (LookPath already verifies executability)
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("binary %s not found", name)
}

// isExecutable checks if a file exists and is executable by the current user.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	// Check it's not a directory
	if info.IsDir() {
		return false
	}
	// Check executable bit (any of owner/group/other)
	mode := info.Mode()
	return mode&0111 != 0
}

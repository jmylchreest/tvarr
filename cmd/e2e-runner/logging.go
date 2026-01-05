package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const (
	// LogDir is the directory where log files are stored.
	LogDir = "logs"
	// StdoutLogFile is the filename for captured stdout.
	StdoutLogFile = "stdout.log"
	// StderrLogFile is the filename for captured stderr.
	StderrLogFile = "stderr.log"
)

// logCapture captures log output while also writing to the original writer.
type logCapture struct {
	buffer bytes.Buffer
	mu     sync.Mutex
	writer io.Writer
}

// newLogCapture creates a new logCapture that tees output to the given writer.
func newLogCapture(w io.Writer) *logCapture {
	return &logCapture{writer: w}
}

// Write implements io.Writer, writing to both the buffer and the underlying writer.
func (lc *logCapture) Write(p []byte) (n int, err error) {
	lc.mu.Lock()
	lc.buffer.Write(p)
	lc.mu.Unlock()
	return lc.writer.Write(p)
}

// Contains checks if the captured output contains a specific string.
func (lc *logCapture) Contains(s string) bool {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	return bytes.Contains(lc.buffer.Bytes(), []byte(s))
}

// String returns the captured output as a string.
func (lc *logCapture) String() string {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	return lc.buffer.String()
}

// LogFiles holds references to log file handles for stdout/stderr capture.
type LogFiles struct {
	StdoutFile *os.File
	StderrFile *os.File
	logDir     string
}

// SetupLogFiles creates the log directory and opens log files for capturing
// tvarr server stdout and stderr output.
// Returns LogFiles which should be closed via Close() when done.
func SetupLogFiles() (*LogFiles, error) {
	// Get working directory to create logs folder relative to it
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	logDir := filepath.Join(wd, LogDir)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	// Open stdout log file
	stdoutPath := filepath.Join(logDir, StdoutLogFile)
	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return nil, err
	}

	// Open stderr log file
	stderrPath := filepath.Join(logDir, StderrLogFile)
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		stdoutFile.Close()
		return nil, err
	}

	return &LogFiles{
		StdoutFile: stdoutFile,
		StderrFile: stderrFile,
		logDir:     logDir,
	}, nil
}

// Close closes the log files.
func (lf *LogFiles) Close() {
	if lf.StdoutFile != nil {
		lf.StdoutFile.Close()
	}
	if lf.StderrFile != nil {
		lf.StderrFile.Close()
	}
}

// Dir returns the log directory path.
func (lf *LogFiles) Dir() string {
	return lf.logDir
}

// Package storage provides sandboxed file operations for tvarr.
// All file operations are restricted to configured directories to prevent
// path traversal and other security issues.
package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Sandbox provides sandboxed file operations within a base directory.
// It prevents path traversal attacks by ensuring all paths resolve within the sandbox.
type Sandbox struct {
	baseDir string
}

// NewSandbox creates a new Sandbox rooted at the given base directory.
// The base directory is created if it doesn't exist.
func NewSandbox(baseDir string) (*Sandbox, error) {
	// Get absolute path
	absPath, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("getting absolute path: %w", err)
	}

	// Create the directory if it doesn't exist
	if err := os.MkdirAll(absPath, 0750); err != nil {
		return nil, fmt.Errorf("creating base directory: %w", err)
	}

	return &Sandbox{baseDir: absPath}, nil
}

// BaseDir returns the absolute path to the sandbox base directory.
func (s *Sandbox) BaseDir() string {
	return s.baseDir
}

// ResolvePath resolves a relative path within the sandbox.
// Returns an error if the path would escape the sandbox or is an absolute path.
func (s *Sandbox) ResolvePath(relativePath string) (string, error) {
	// Reject absolute paths outright
	if filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("path escapes sandbox: %s (absolute paths not allowed)", relativePath)
	}

	// Clean the path to remove . and .. components
	cleanPath := filepath.Clean(relativePath)

	// Join with base directory
	fullPath := filepath.Join(s.baseDir, cleanPath)

	// Get absolute path
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("getting absolute path: %w", err)
	}

	// Ensure the path is within the sandbox
	if !strings.HasPrefix(absPath, s.baseDir+string(filepath.Separator)) && absPath != s.baseDir {
		return "", fmt.Errorf("path escapes sandbox: %s", relativePath)
	}

	return absPath, nil
}

// Exists checks if a path exists within the sandbox.
func (s *Sandbox) Exists(relativePath string) (bool, error) {
	path, err := s.ResolvePath(relativePath)
	if err != nil {
		return false, err
	}

	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking path: %w", err)
	}
	return true, nil
}

// MkdirAll creates a directory and all parent directories within the sandbox.
func (s *Sandbox) MkdirAll(relativePath string) error {
	path, err := s.ResolvePath(relativePath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(path, 0750); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	return nil
}

// WriteFile writes data to a file within the sandbox.
func (s *Sandbox) WriteFile(relativePath string, data []byte) error {
	path, err := s.ResolvePath(relativePath)
	if err != nil {
		return err
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0640); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}
	return nil
}

// ReadFile reads a file from within the sandbox.
func (s *Sandbox) ReadFile(relativePath string) ([]byte, error) {
	path, err := s.ResolvePath(relativePath)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}
	return data, nil
}

// OpenFile opens a file within the sandbox with the given flags and permissions.
func (s *Sandbox) OpenFile(relativePath string, flag int, perm os.FileMode) (*os.File, error) {
	path, err := s.ResolvePath(relativePath)
	if err != nil {
		return nil, err
	}

	// Ensure parent directory exists for write operations
	if flag&(os.O_CREATE|os.O_WRONLY|os.O_RDWR) != 0 {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0750); err != nil {
			return nil, fmt.Errorf("creating parent directory: %w", err)
		}
	}

	file, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	return file, nil
}

// Remove removes a file or empty directory within the sandbox.
func (s *Sandbox) Remove(relativePath string) error {
	path, err := s.ResolvePath(relativePath)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("removing path: %w", err)
	}
	return nil
}

// RemoveAll removes a path and all its contents within the sandbox.
func (s *Sandbox) RemoveAll(relativePath string) error {
	path, err := s.ResolvePath(relativePath)
	if err != nil {
		return err
	}

	// Extra safety: don't allow removing the base directory itself
	if path == s.baseDir {
		return fmt.Errorf("cannot remove sandbox base directory")
	}

	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("removing path: %w", err)
	}
	return nil
}

// Rename renames/moves a file within the sandbox.
func (s *Sandbox) Rename(oldPath, newPath string) error {
	oldAbs, err := s.ResolvePath(oldPath)
	if err != nil {
		return fmt.Errorf("resolving old path: %w", err)
	}

	newAbs, err := s.ResolvePath(newPath)
	if err != nil {
		return fmt.Errorf("resolving new path: %w", err)
	}

	// Ensure parent directory of new path exists
	dir := filepath.Dir(newAbs)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	if err := os.Rename(oldAbs, newAbs); err != nil {
		return fmt.Errorf("renaming file: %w", err)
	}
	return nil
}

// AtomicWrite writes data to a file atomically within the sandbox.
// It writes to a temporary file first, then renames it to the target.
// This ensures the file is either completely written or not at all.
func (s *Sandbox) AtomicWrite(relativePath string, data []byte) error {
	targetPath, err := s.ResolvePath(relativePath)
	if err != nil {
		return err
	}

	// Ensure parent directory exists
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	// Create a unique temporary file name
	tempName := fmt.Sprintf(".%s.%s.tmp", filepath.Base(relativePath), randomHex(8))
	tempPath := filepath.Join(dir, tempName)

	// Write to temporary file
	if err := os.WriteFile(tempPath, data, 0640); err != nil {
		return fmt.Errorf("writing temporary file: %w", err)
	}

	// Rename to target (atomic on most filesystems)
	if err := os.Rename(tempPath, targetPath); err != nil {
		// Clean up temp file on failure
		os.Remove(tempPath)
		return fmt.Errorf("renaming to target: %w", err)
	}

	return nil
}

// AtomicWriteReader writes data from a reader to a file atomically within the sandbox.
func (s *Sandbox) AtomicWriteReader(relativePath string, r io.Reader) error {
	targetPath, err := s.ResolvePath(relativePath)
	if err != nil {
		return err
	}

	// Ensure parent directory exists
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	// Create a unique temporary file name
	tempName := fmt.Sprintf(".%s.%s.tmp", filepath.Base(relativePath), randomHex(8))
	tempPath := filepath.Join(dir, tempName)

	// Create temporary file
	tempFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return fmt.Errorf("creating temporary file: %w", err)
	}

	// Copy data to temporary file
	_, err = io.Copy(tempFile, r)
	closeErr := tempFile.Close()

	if err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("writing to temporary file: %w", err)
	}
	if closeErr != nil {
		os.Remove(tempPath)
		return fmt.Errorf("closing temporary file: %w", closeErr)
	}

	// Rename to target (atomic on most filesystems)
	if err := os.Rename(tempPath, targetPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("renaming to target: %w", err)
	}

	return nil
}

// CreateTemp creates a new temporary file in the sandbox.
// The caller is responsible for closing and removing the file.
func (s *Sandbox) CreateTemp(dir, pattern string) (*os.File, error) {
	if dir == "" {
		dir = "temp"
	}

	absDir, err := s.ResolvePath(dir)
	if err != nil {
		return nil, err
	}

	// Ensure directory exists
	if err := os.MkdirAll(absDir, 0750); err != nil {
		return nil, fmt.Errorf("creating temp directory: %w", err)
	}

	file, err := os.CreateTemp(absDir, pattern)
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	return file, nil
}

// TempDir returns the path to a temporary directory within the sandbox.
// The directory is created if it doesn't exist.
func (s *Sandbox) TempDir() (string, error) {
	tempDir, err := s.ResolvePath("temp")
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(tempDir, 0750); err != nil {
		return "", fmt.Errorf("creating temp directory: %w", err)
	}

	return tempDir, nil
}

// List returns a list of entries in a directory within the sandbox.
func (s *Sandbox) List(relativePath string) ([]os.DirEntry, error) {
	path, err := s.ResolvePath(relativePath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("reading directory: %w", err)
	}
	return entries, nil
}

// Walk walks the file tree within the sandbox, calling fn for each file or directory.
func (s *Sandbox) Walk(relativePath string, fn filepath.WalkFunc) error {
	path, err := s.ResolvePath(relativePath)
	if err != nil {
		return err
	}

	return filepath.Walk(path, func(walkPath string, info os.FileInfo, err error) error {
		// Convert absolute path back to relative for the callback
		relPath, relErr := filepath.Rel(s.baseDir, walkPath)
		if relErr != nil {
			relPath = walkPath
		}
		return fn(relPath, info, err)
	})
}

// Stat returns file info for a path within the sandbox.
func (s *Sandbox) Stat(relativePath string) (os.FileInfo, error) {
	path, err := s.ResolvePath(relativePath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("getting file info: %w", err)
	}
	return info, nil
}

// Size returns the size of a file within the sandbox.
func (s *Sandbox) Size(relativePath string) (int64, error) {
	info, err := s.Stat(relativePath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// randomHex generates a random hex string of the specified length.
func randomHex(n int) string {
	bytes := make([]byte, n/2+1)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to less random but still unique
		return fmt.Sprintf("%d", os.Getpid())
	}
	return hex.EncodeToString(bytes)[:n]
}

// SubSandbox creates a new Sandbox within a subdirectory of this sandbox.
func (s *Sandbox) SubSandbox(relativePath string) (*Sandbox, error) {
	path, err := s.ResolvePath(relativePath)
	if err != nil {
		return nil, err
	}

	return NewSandbox(path)
}

// AtomicPublish atomically publishes a file from an external absolute path
// to a location within the sandbox. It first tries a direct rename (efficient
// if same filesystem), then falls back to copy-then-rename for cross-filesystem
// scenarios.
func (s *Sandbox) AtomicPublish(srcAbsPath, destRelativePath string) error {
	targetPath, err := s.ResolvePath(destRelativePath)
	if err != nil {
		return err
	}

	// Ensure parent directory exists
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	// Try direct rename first (atomic if same filesystem)
	if err := os.Rename(srcAbsPath, targetPath); err == nil {
		return nil
	}

	// Fall back to copy-then-rename for cross-filesystem scenarios
	return s.atomicCopyPublish(srcAbsPath, targetPath)
}

// atomicCopyPublish copies a file then renames it for atomicity.
func (s *Sandbox) atomicCopyPublish(srcAbsPath, targetPath string) error {
	dir := filepath.Dir(targetPath)
	tempName := fmt.Sprintf(".%s.%s.tmp", filepath.Base(targetPath), randomHex(8))
	tempPath := filepath.Join(dir, tempName)

	// Open source file
	srcFile, err := os.Open(srcAbsPath)
	if err != nil {
		return fmt.Errorf("opening source file: %w", err)
	}
	defer srcFile.Close()

	// Create temp destination file
	tempFile, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	// Copy data
	_, err = io.Copy(tempFile, srcFile)
	closeErr := tempFile.Close()

	if err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("copying to temp file: %w", err)
	}
	if closeErr != nil {
		os.Remove(tempPath)
		return fmt.Errorf("closing temp file: %w", closeErr)
	}

	// Atomic rename (temp and dest are now on same filesystem)
	if err := os.Rename(tempPath, targetPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("renaming to target: %w", err)
	}

	return nil
}

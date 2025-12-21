package util

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindBinary(t *testing.T) {
	t.Run("finds executable binary via environment variable", func(t *testing.T) {
		// Create a temp file and make it executable
		tmpFile, err := os.CreateTemp("", "test-binary-*")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		tmpFile.Close()
		require.NoError(t, os.Chmod(tmpFile.Name(), 0755))

		t.Setenv("TEST_BINARY_PATH", tmpFile.Name())

		path, err := FindBinary("nonexistent-binary", "TEST_BINARY_PATH")
		require.NoError(t, err)
		assert.Equal(t, tmpFile.Name(), path)
	})

	t.Run("env var takes priority over PATH", func(t *testing.T) {
		// Create an executable temp file for the env var path
		tmpFile, err := os.CreateTemp("", "test-binary-*")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		tmpFile.Close()
		require.NoError(t, os.Chmod(tmpFile.Name(), 0755))

		t.Setenv("TEST_BINARY_PATH", tmpFile.Name())

		// "ls" exists on PATH, but env var should take priority
		path, err := FindBinary("ls", "TEST_BINARY_PATH")
		require.NoError(t, err)
		assert.Equal(t, tmpFile.Name(), path)
	})

	t.Run("finds binary on PATH when no env var", func(t *testing.T) {
		// "ls" should exist on any Unix system
		path, err := FindBinary("ls", "")
		require.NoError(t, err)
		assert.NotEmpty(t, path)
		assert.Contains(t, path, "ls")
	})

	t.Run("returns error when binary not found", func(t *testing.T) {
		path, err := FindBinary("definitely-nonexistent-binary-12345", "")
		assert.Error(t, err)
		assert.Empty(t, path)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("ignores env var if file does not exist", func(t *testing.T) {
		t.Setenv("TEST_BINARY_PATH", "/nonexistent/path/to/binary")

		// Should fall through to PATH lookup for "ls"
		path, err := FindBinary("ls", "TEST_BINARY_PATH")
		require.NoError(t, err)
		assert.NotEqual(t, "/nonexistent/path/to/binary", path)
		assert.Contains(t, path, "ls")
	})

	t.Run("ignores env var if file is not executable", func(t *testing.T) {
		// Create a temp file but don't make it executable
		tmpFile, err := os.CreateTemp("", "test-binary-*")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		tmpFile.Close()
		require.NoError(t, os.Chmod(tmpFile.Name(), 0644)) // readable but not executable

		t.Setenv("TEST_BINARY_PATH", tmpFile.Name())

		// Should fall through to PATH lookup for "ls"
		path, err := FindBinary("ls", "TEST_BINARY_PATH")
		require.NoError(t, err)
		assert.NotEqual(t, tmpFile.Name(), path)
		assert.Contains(t, path, "ls")
	})

	t.Run("ignores directory even if executable", func(t *testing.T) {
		// Create a temp directory
		tmpDir, err := os.MkdirTemp("", "test-binary-dir-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		t.Setenv("TEST_BINARY_PATH", tmpDir)

		// Should fall through to PATH lookup for "ls"
		path, err := FindBinary("ls", "TEST_BINARY_PATH")
		require.NoError(t, err)
		assert.NotEqual(t, tmpDir, path)
		assert.Contains(t, path, "ls")
	})
}

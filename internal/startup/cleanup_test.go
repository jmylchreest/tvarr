package startup

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// T009-TEST: Test CleanupOrphanedTempDirs
func TestCleanupOrphanedTempDirs(t *testing.T) {
	t.Run("removes old tvarr-proxy directories", func(t *testing.T) {
		logger := newTestLogger()

		// Create a temp base directory for the test
		baseDir, err := os.MkdirTemp("", "cleanup-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(baseDir)

		// Create an old orphaned directory (older than 1 hour)
		oldDir := filepath.Join(baseDir, "tvarr-proxy-01HZ1234567890ABCDEF")
		require.NoError(t, os.Mkdir(oldDir, 0755))

		// Create a dummy file in the old dir first
		dummyFile := filepath.Join(oldDir, "dummy.txt")
		require.NoError(t, os.WriteFile(dummyFile, []byte("test"), 0644))

		// Set modification time to 2 hours ago AFTER creating the file
		// (creating the file would update the dir mtime)
		oldTime := time.Now().Add(-2 * time.Hour)
		require.NoError(t, os.Chtimes(oldDir, oldTime, oldTime))

		// Run cleanup
		count, err := CleanupOrphanedTempDirs(logger, baseDir, 1*time.Hour)
		require.NoError(t, err)

		// Verify the old directory was removed
		assert.Equal(t, 1, count)
		_, err = os.Stat(oldDir)
		assert.True(t, os.IsNotExist(err), "old directory should be removed")
	})

	t.Run("preserves recent tvarr-proxy directories", func(t *testing.T) {
		logger := newTestLogger()

		// Create a temp base directory for the test
		baseDir, err := os.MkdirTemp("", "cleanup-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(baseDir)

		// Create a recent directory (less than 1 hour old)
		recentDir := filepath.Join(baseDir, "tvarr-proxy-01HZ0987654321FEDCBA")
		require.NoError(t, os.Mkdir(recentDir, 0755))

		// Set modification time to 30 minutes ago
		recentTime := time.Now().Add(-30 * time.Minute)
		require.NoError(t, os.Chtimes(recentDir, recentTime, recentTime))

		// Run cleanup
		count, err := CleanupOrphanedTempDirs(logger, baseDir, 1*time.Hour)
		require.NoError(t, err)

		// Verify the recent directory was NOT removed
		assert.Equal(t, 0, count)
		_, err = os.Stat(recentDir)
		assert.NoError(t, err, "recent directory should be preserved")
	})

	t.Run("ignores non-tvarr directories", func(t *testing.T) {
		logger := newTestLogger()

		// Create a temp base directory for the test
		baseDir, err := os.MkdirTemp("", "cleanup-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(baseDir)

		// Create an old non-tvarr directory
		otherDir := filepath.Join(baseDir, "some-other-dir")
		require.NoError(t, os.Mkdir(otherDir, 0755))

		// Set modification time to 2 hours ago
		oldTime := time.Now().Add(-2 * time.Hour)
		require.NoError(t, os.Chtimes(otherDir, oldTime, oldTime))

		// Run cleanup
		count, err := CleanupOrphanedTempDirs(logger, baseDir, 1*time.Hour)
		require.NoError(t, err)

		// Verify the non-tvarr directory was NOT removed
		assert.Equal(t, 0, count)
		_, err = os.Stat(otherDir)
		assert.NoError(t, err, "non-tvarr directory should be preserved")
	})

	t.Run("handles non-existent directory gracefully", func(t *testing.T) {
		logger := newTestLogger()

		// Run cleanup on non-existent directory
		count, err := CleanupOrphanedTempDirs(logger, "/nonexistent/path/12345", 1*time.Hour)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("cleans up multiple old directories", func(t *testing.T) {
		logger := newTestLogger()

		// Create a temp base directory for the test
		baseDir, err := os.MkdirTemp("", "cleanup-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(baseDir)

		// Create multiple old directories
		oldDirs := []string{
			"tvarr-proxy-01HZ1111111111111111",
			"tvarr-proxy-01HZ2222222222222222",
			"tvarr-proxy-01HZ3333333333333333",
		}

		oldTime := time.Now().Add(-2 * time.Hour)
		for _, dir := range oldDirs {
			dirPath := filepath.Join(baseDir, dir)
			require.NoError(t, os.Mkdir(dirPath, 0755))
			require.NoError(t, os.Chtimes(dirPath, oldTime, oldTime))
		}

		// Run cleanup
		count, err := CleanupOrphanedTempDirs(logger, baseDir, 1*time.Hour)
		require.NoError(t, err)

		// Verify all old directories were removed
		assert.Equal(t, 3, count)
		for _, dir := range oldDirs {
			dirPath := filepath.Join(baseDir, dir)
			_, err = os.Stat(dirPath)
			assert.True(t, os.IsNotExist(err), "directory %s should be removed", dir)
		}
	})
}

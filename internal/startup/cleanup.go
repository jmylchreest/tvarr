// Package startup provides utilities for application startup tasks.
package startup

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
)

// TempDirPrefix is the prefix used for tvarr proxy temp directories.
const TempDirPrefix = "tvarr-proxy-"

// CleanupOrphanedTempDirs removes orphaned temporary directories that are older
// than the specified maxAge. It looks for directories matching the pattern
// "tvarr-proxy-*" in the specified base directory.
//
// Returns the number of directories removed and any error encountered.
func CleanupOrphanedTempDirs(logger *slog.Logger, baseDir string, maxAge time.Duration) (int, error) {
	// Check if the base directory exists
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		logger.Debug("base directory does not exist, skipping cleanup",
			"path", baseDir,
		)
		return 0, nil
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		logger.Error("failed to read directory for cleanup",
			"path", baseDir,
			"error", err,
		)
		return 0, err
	}

	cutoff := time.Now().Add(-maxAge)
	var removed int

	for _, entry := range entries {
		// Only process directories
		if !entry.IsDir() {
			continue
		}

		// Only process directories matching our prefix
		if !strings.HasPrefix(entry.Name(), TempDirPrefix) {
			continue
		}

		dirPath := filepath.Join(baseDir, entry.Name())

		// Get file info for modification time
		info, err := entry.Info()
		if err != nil {
			logger.Warn("failed to get directory info",
				"path", dirPath,
				"error", err,
			)
			continue
		}

		// Check if directory is older than cutoff
		if info.ModTime().After(cutoff) {
			logger.Debug("preserving recent temp directory",
				"path", dirPath,
				"age", time.Since(info.ModTime()).Round(time.Second),
			)
			continue
		}

		// Remove the orphaned directory
		if err := os.RemoveAll(dirPath); err != nil {
			logger.Warn("failed to remove orphaned temp directory",
				"path", dirPath,
				"error", err,
			)
			continue
		}

		logger.Info("removed orphaned temp directory",
			"path", dirPath,
			"age", time.Since(info.ModTime()).Round(time.Second),
		)
		removed++
	}

	return removed, nil
}

// DefaultCleanupAge is the default maximum age for orphaned temp directories (1 hour).
const DefaultCleanupAge = 1 * time.Hour

// CleanupSystemTempDirs cleans up orphaned tvarr temp directories from the system
// temp directory using the default cleanup age.
func CleanupSystemTempDirs(logger *slog.Logger) (int, error) {
	return CleanupOrphanedTempDirs(logger, os.TempDir(), DefaultCleanupAge)
}

// RecoverStaleProxyStatuses resets any proxies stuck in "generating" status back
// to "failed". This handles the case where the server crashed or was restarted
// while a proxy generation pipeline was in progress. Without this recovery, proxies
// would remain permanently stuck in "generating" status in the database since the
// in-memory pipeline state is lost on restart.
//
// Returns the number of proxies recovered and any error encountered.
func RecoverStaleProxyStatuses(ctx context.Context, logger *slog.Logger, proxyRepo repository.StreamProxyRepository) (int, error) {
	proxies, err := proxyRepo.GetAll(ctx)
	if err != nil {
		logger.Error("failed to get proxies for stale status recovery",
			"error", err,
		)
		return 0, err
	}

	var recovered int
	for _, proxy := range proxies {
		if proxy.Status != models.StreamProxyStatusGenerating {
			continue
		}

		logger.Warn("recovering stale proxy status",
			"proxy_id", proxy.ID.String(),
			"proxy_name", proxy.Name,
			"status", proxy.Status,
		)

		if err := proxyRepo.UpdateStatus(ctx, proxy.ID, models.StreamProxyStatusFailed, "interrupted by server restart"); err != nil {
			logger.Error("failed to recover stale proxy status",
				"proxy_id", proxy.ID.String(),
				"proxy_name", proxy.Name,
				"error", err,
			)
			continue
		}

		recovered++
	}

	return recovered, nil
}

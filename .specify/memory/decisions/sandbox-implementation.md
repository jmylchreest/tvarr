# Decision: Sandbox Implementation

**Date**: 2025-11-29  
**Status**: Under Review

## Context

The original Rust m3u-proxy used a sandbox crate that provided:
1. Confined IO operations (all paths resolved within a base directory)
2. File expiry/TTL with automatic cleanup
3. Path traversal prevention

## Current Implementation

Located at `internal/storage/sandbox.go`, the current implementation provides:
- Path traversal prevention
- Sandboxed file operations (read, write, atomic write)
- Directory operations
- Temp file management
- Sub-sandbox creation

**Missing**: File expiry/TTL functionality

## Research: Existing Go Packages

No well-maintained Go package was found that combines:
- Sandboxed file operations with path traversal prevention
- File expiry/TTL with automatic cleanup

Related packages found:
- [ttlcache](https://pkg.go.dev/github.com/ReneKroon/ttlcache) - In-memory cache with TTL, not file-based
- [go-lru-ttl-cache](https://pkg.go.dev/github.com/alexions/go-lru-ttl-cache) - LRU cache with TTL
- [goswift](https://github.com/leoantony72/goswift) - In-memory cache with disk persistence

None of these provide sandboxed file operations with expiry.

## Options

### Option A: Extend Current Implementation
Add TTL tracking to the existing `internal/storage/sandbox.go`:
- Metadata file or embedded database for tracking file creation times
- Background goroutine for cleanup
- Keep in `internal/storage/`

**Pros**: Simple, integrated, fits current patterns  
**Cons**: Not reusable outside tvarr

### Option B: Create Separate `pkg/sandbox` Package
Move sandbox to a public package with full features:
- Sandboxed IO with path traversal prevention
- File expiry/TTL with configurable cleanup intervals
- Optional metadata persistence

**Pros**: Reusable, potentially open-sourceable  
**Cons**: More work, need to consider API stability

### Option C: Use afero + Custom TTL Layer
Use [afero](https://github.com/spf13/afero) for filesystem abstraction:
- afero.BasePathFs for sandboxing
- Custom wrapper for TTL tracking

**Pros**: Well-tested filesystem abstraction  
**Cons**: afero doesn't have built-in TTL, still need custom implementation

## Recommendation

**Option B** is recommended for future work:
1. Keep current `internal/storage/sandbox.go` for immediate needs
2. Plan `pkg/sandbox` as a Phase 2 enhancement with full TTL support
3. Design with potential open-source release in mind

## TTL Implementation Sketch

```go
type TTLSandbox struct {
    *Sandbox
    ttlIndex    map[string]time.Time  // path -> expiry
    mu          sync.RWMutex
    cleanupTick time.Duration
    stopCleanup chan struct{}
}

func (s *TTLSandbox) WriteFileWithTTL(path string, data []byte, ttl time.Duration) error
func (s *TTLSandbox) Touch(path string, ttl time.Duration) error  // Extend TTL
func (s *TTLSandbox) StartCleanup(interval time.Duration)
func (s *TTLSandbox) StopCleanup()
```

## Action Items

- [ ] Document current sandbox limitations
- [ ] Add TTL support when needed (Phase 4: Logo Caching)
- [ ] Consider `pkg/sandbox` extraction for reusability

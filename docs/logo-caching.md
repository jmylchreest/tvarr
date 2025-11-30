# Logo Caching System

This document describes the logo caching system in tvarr and how it compares to the original m3u-proxy Rust implementation.

## Overview

The logo caching system provides persistent storage and fast in-memory indexing for channel logos. Logos are downloaded from remote URLs and stored with JSON metadata files for persistence.

**Key Design Principles:**
- File-based storage (no database dependency)
- In-memory index for O(1) lookups
- **Deterministic IDs**: Same URL always produces same ID (SHA256 hash)
- Sharded directory structure for scalability
- JSON metadata alongside image files
- Deduplication: Logos shared across channels are only downloaded once

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        LogoService                               │
│  - CacheLogo(ctx, url) -> downloads and caches                  │
│  - Contains(url) -> fast existence check                        │
│  - GetCachedLogo(url) -> retrieve metadata                      │
│  - LoadIndex(ctx) -> rebuild index from disk                    │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                       LogoIndexer                                │
│  In-Memory Index (HashMap-based)                                │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │  byULID     │  │  byURLHash  │  │   byURL     │             │
│  │ map[string] │  │ map[string] │  │ map[string] │             │
│  └─────────────┘  └─────────────┘  └─────────────┘             │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                        LogoCache                                 │
│  File System Storage                                            │
│  - StoreWithMetadata(meta, imageData)                           │
│  - LoadMetadata(ulid)                                           │
│  - ScanLogos() -> rebuild index                                 │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                     File System                                  │
│  {base_dir}/                                                    │
│  ├── 01/                    # Shard (first 2 chars of ULID)     │
│  │   ├── 01J5ABC....png     # Logo image                        │
│  │   ├── 01J5ABC....json    # Metadata                          │
│  │   └── ...                                                    │
│  ├── 02/                                                        │
│  └── ...                                                        │
└─────────────────────────────────────────────────────────────────┘
```

## Data Structures

### CachedLogoMetadata

The metadata structure stored as JSON alongside each logo image:

```go
type CachedLogoMetadata struct {
    ID          string    `json:"id"`           // Deterministic identifier (SHA256 of URL)
    OriginalURL string    `json:"original_url"` // Source URL
    URLHash     string    `json:"url_hash"`     // Same as ID for URL-sourced logos
    ContentType string    `json:"content_type"` // MIME type (e.g., "image/png")
    FileSize    int64     `json:"file_size"`    // Size in bytes
    Width       int       `json:"width"`        // Image dimensions
    Height      int       `json:"height"`
    CreatedAt   time.Time `json:"created_at"`   // When cached
    SourceHint  string    `json:"source_hint"`  // Optional context
}
```

**Key Property**: The `ID` is deterministic - `NewCachedLogoMetadata(url)` always produces the same ID for the same URL. This ensures that logos shared across multiple channels are stored only once.

### LogoIndexer

In-memory index with three parallel HashMaps:

```go
type LogoIndexer struct {
    cache     *storage.LogoCache
    mu        sync.RWMutex                        // Thread-safe access
    byID      map[string]*CachedLogoMetadata      // Primary index (by deterministic ID)
    byURLHash map[string]*CachedLogoMetadata      // Hash-based lookup
    byURL     map[string]*CachedLogoMetadata      // Direct URL lookup
}
```

### IndexStats

Statistics for monitoring cache health:

```go
type IndexStats struct {
    TotalLogos int   // Number of cached logos
    TotalSize  int64 // Total storage size in bytes
}
```

## Storage Format

### Directory Structure

Logos are stored in a sharded directory structure using the first 2 characters of the ID (which is a SHA256 hash):

```
logos/
├── a1/
│   ├── a1b2c3d4e5f6...abc.png
│   ├── a1b2c3d4e5f6...abc.json
│   └── ...
├── b2/
│   └── ...
└── ...
```

**Benefits:**
- Distributes files across directories (~256 possible shards for hex prefixes)
- Avoids filesystem performance issues with large directories
- Deterministic: Same URL always stored in same location

### File Naming

- **Image files**: `{id}.{ext}` (ID is SHA256 hash of URL, extension based on content type)
- **Metadata files**: `{id}.json`

Example:
```
a1b2c3d4e5f6789012345678901234567890abcdef123456789012345678901234.png   # Logo image
a1b2c3d4e5f6789012345678901234567890abcdef123456789012345678901234.json  # Metadata
```

### Metadata JSON Format

```json
{
  "id": "a1b2c3d4e5f6789012345678901234567890abcdef123456789012345678901234",
  "original_url": "https://example.com/channel/logo.png",
  "url_hash": "a1b2c3d4e5f6789012345678901234567890abcdef123456789012345678901234",
  "content_type": "image/png",
  "file_size": 12345,
  "width": 256,
  "height": 256,
  "created_at": "2024-01-15T10:30:00Z",
  "source_hint": "channel:ESPN"
}
```

## Lookup Mechanisms

### 1. By URL (Primary)

Most common lookup - O(1) via `byURL` map:

```go
func (idx *LogoIndexer) GetByURL(url string) *CachedLogoMetadata {
    idx.mu.RLock()
    defer idx.mu.RUnlock()
    return idx.byURL[url]
}
```

### 2. By URL Hash

For cases where only the hash is available:

```go
func (idx *LogoIndexer) GetByURLHash(hash string) *CachedLogoMetadata {
    idx.mu.RLock()
    defer idx.mu.RUnlock()
    return idx.byURLHash[hash]
}
```

### 3. By ID

For direct access by identifier:

```go
func (idx *LogoIndexer) GetByID(id string) *CachedLogoMetadata {
    idx.mu.RLock()
    defer idx.mu.RUnlock()
    return idx.byID[id]
}
```

### 4. Contains Check

Fast existence check without returning data:

```go
func (idx *LogoIndexer) Contains(url string) bool {
    idx.mu.RLock()
    defer idx.mu.RUnlock()
    _, ok := idx.byURL[url]
    return ok
}
```

## Pipeline Integration

The logo caching system integrates with the data pipeline via the `logocaching` stage:

```go
type LogoCacher interface {
    CacheLogo(ctx context.Context, logoURL string) (*storage.CachedLogoMetadata, error)
    Contains(logoURL string) bool
}

type Stage struct {
    shared.BaseStage
    cacher LogoCacher
    logger *slog.Logger
    stats  Stats
}
```

### Pipeline Execution Flow

1. **Collect unique logo URLs** from all channels
2. **Check existence** in index (skip already cached)
3. **Download and cache** new logos
4. **Track statistics** (processed, cached, errors)

```go
func (s *Stage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
    // Collect unique logo URLs
    logoURLs := make(map[string]struct{})
    for _, ch := range state.Channels {
        if ch.TvgLogo != "" {
            logoURLs[ch.TvgLogo] = struct{}{}
        }
    }

    // Process each unique URL
    for logoURL := range logoURLs {
        if s.cacher.Contains(logoURL) {
            s.stats.AlreadyCached++
            continue
        }

        _, err := s.cacher.CacheLogo(ctx, logoURL)
        if err != nil {
            s.stats.Errors++
            continue
        }
        s.stats.NewlyCached++
    }

    return result, nil
}
```

## Index Persistence

### Startup: Loading from Disk

On application startup, the index is rebuilt by scanning the filesystem:

```go
func (idx *LogoIndexer) LoadFromDisk(ctx context.Context) error {
    // Scan all metadata files
    logos, err := idx.cache.ScanLogos()
    if err != nil {
        return err
    }

    // Rebuild index
    idx.mu.Lock()
    defer idx.mu.Unlock()

    for _, meta := range logos {
        idx.byULID[meta.ULID] = meta
        idx.byURLHash[meta.URLHash] = meta
        idx.byURL[meta.OriginalURL] = meta
    }

    return nil
}
```

### Runtime: Index Updates

Index is updated immediately when logos are cached:

```go
func (idx *LogoIndexer) Add(meta *CachedLogoMetadata) {
    idx.mu.Lock()
    defer idx.mu.Unlock()

    idx.byULID[meta.ULID] = meta
    idx.byURLHash[meta.URLHash] = meta
    idx.byURL[meta.OriginalURL] = meta
}
```

## Thread Safety

All index operations are protected by `sync.RWMutex`:

- **Read operations** (`GetByURL`, `Contains`, `Stats`): Use `RLock()` for concurrent reads
- **Write operations** (`Add`, `Remove`, `Clear`): Use `Lock()` for exclusive access

This allows high concurrency for reads while ensuring consistency for writes.

---

## Comparison with m3u-proxy (Rust)

### Feature Comparison

| Feature | tvarr (Go) | m3u-proxy (Rust) |
|---------|------------|------------------|
| **Storage** | File-based only | File-based + Database |
| **Index** | 3 HashMaps | 3 HashMaps + LRU cache |
| **ID Generation** | SHA256 of URL (deterministic) | SHA256 of normalized URL |
| **Sharding** | ID prefix (2 chars) | None (flat directory) |
| **Memory Optimization** | Full strings | Hash-based (u64) |
| **Dimension Encoding** | Full int | 12-bit encoded |
| **Search Types** | URL, Hash, ID | URL, Channel Name, Group |
| **Relevance Scoring** | No | Yes |
| **Maintenance/Cleanup** | No (permanent) | LRU-based cleanup |
| **File Watching** | No | No |
| **Circuit Breaker** | No | Yes |

### Architectural Differences

#### 1. Identifier Strategy

**tvarr**: Uses SHA256 hash of URL (deterministic)
```go
// Same URL always produces same ID
urlHash := sha256(url)  // 64-char hex string
meta := NewCachedLogoMetadata(url)  // ID = urlHash
```

**m3u-proxy**: Uses SHA256 hash of normalized URL
```rust
let cache_id = sha256(normalize_url(url));  // 64-char hex string
```

**Key Similarity:** Both use deterministic IDs derived from the URL, ensuring:
- Same URL always maps to same file
- Logos shared across channels are stored only once
- No duplicate downloads for same logo

**Minor Difference:** m3u-proxy normalizes URLs (removes scheme, sorts params) before hashing. tvarr hashes the URL as-is. For most use cases this is equivalent since logo URLs are typically static.

#### 2. Memory Optimization

**tvarr**: Stores full strings in index
```go
byURL map[string]*CachedLogoMetadata  // Full URL strings
```
- Simple implementation
- ~100-200 bytes per entry
- For 10K logos: ~1-2 MB

**m3u-proxy**: Uses 64-bit hashes
```rust
url_hash: u64,           // xxHash64 of URL
channel_name_hash: u64,  // xxHash64 of name
```
- More complex but memory-efficient
- ~40-50 bytes per entry
- For 10K logos: ~400-500 KB
- 50-75% memory savings

#### 3. Directory Structure

**tvarr**: Sharded by ULID prefix
```
logos/
├── 01/
│   └── 01J5ABC....png
├── 02/
└── ...
```

**m3u-proxy**: Flat directory
```
cached_logos/
├── abc123....png
├── def456....png
└── ...
```

**Trade-offs:**
- Sharding: Better filesystem performance with many files
- Flat: Simpler, fewer directory operations

#### 4. Search Capabilities

**tvarr**: URL-centric lookups only
```go
GetByURL(url string)
GetByURLHash(hash string)
GetByULID(ulid string)
Contains(url string)
```

**m3u-proxy**: Multi-criteria search with relevance scoring
```rust
pub struct LogoCacheQuery {
    pub original_url: Option<String>,
    pub channel_name: Option<String>,
    pub channel_group: Option<String>,
}
// Plus substring search with relevance scoring
```

#### 5. Maintenance Strategy

**tvarr**: No automatic cleanup (permanent storage)
- Logos stored indefinitely
- Manual cleanup if needed
- Simpler implementation

**m3u-proxy**: LRU-based maintenance
```rust
max_cache_size_mb: 1024  // 1GB limit
max_age_days: 30         // 30-day TTL
```
- Automatic cleanup of old/unused logos
- Prevents unbounded growth

#### 6. HTTP Client Features

**tvarr**: Standard HTTP client
```go
httpClient: http.DefaultClient
```

**m3u-proxy**: Circuit breaker pattern
```rust
CircuitBreakerProfileConfig {
    timeout_ms: 30000,
    failure_threshold: 5,
    recovery_timeout_ms: 60000,
}
```

### What tvarr Keeps from m3u-proxy

1. **In-memory index pattern**: Fast HashMap-based lookups
2. **File-based storage**: JSON metadata alongside images
3. **Deterministic IDs**: SHA256 hash of URL ensures same URL = same file
4. **Thread-safe index**: Mutex-protected concurrent access
5. **Pipeline integration**: Bulk logo caching during data processing
6. **Deduplication**: Logos shared across channels stored only once

### What tvarr Simplifies

1. **No database**: Pure file-based storage
2. **No LRU cleanup**: Permanent storage
3. **No relevance scoring**: Simple exact-match lookups
4. **No dimension encoding**: Full integer storage
5. **No circuit breaker**: Standard HTTP client
6. **No URL normalization**: Hash URL directly (sufficient for typical logo URLs)

### What tvarr Adds

1. **Sharded directories**: Better scalability for large logo counts
2. **Source hints**: Optional context about logo origin
3. **Uploaded logo support**: ULID-based IDs for logos without URLs

## Performance Characteristics

### Memory Usage

For 10,000 cached logos:
- **Index entries**: ~10,000 × 3 maps × pointer size
- **Metadata objects**: ~10,000 × 200 bytes = ~2 MB
- **Total estimate**: ~3-5 MB

### Lookup Performance

| Operation | Time Complexity | Notes |
|-----------|-----------------|-------|
| Contains | O(1) | HashMap lookup |
| GetByURL | O(1) | HashMap lookup |
| GetByHash | O(1) | HashMap lookup |
| GetByULID | O(1) | HashMap lookup |
| LoadFromDisk | O(n) | Scans all metadata files |
| Add | O(1) | HashMap insert |
| Remove | O(1) | HashMap delete |

### Disk I/O

- **Cache hit**: No disk I/O (in-memory index)
- **Cache miss + download**: 1 HTTP request + 2 file writes (image + metadata)
- **Index rebuild**: 1 file read per cached logo

## Configuration

The logo cache directory is configurable via application settings:

```go
// Logo cache path from configuration
cache, err := storage.NewLogoCache(config.Storage.LogoCachePath)
```

## Future Enhancements

Potential improvements (not currently implemented):

1. **Filesystem watching**: Auto-sync index when files change externally
2. **LRU cleanup**: Optional size/age-based maintenance
3. **Channel name index**: Search logos by channel name
4. **Content-based deduplication**: Hash image content to avoid duplicates
5. **Dimension extraction**: Store actual image dimensions
6. **Format conversion**: Support WebP or other formats

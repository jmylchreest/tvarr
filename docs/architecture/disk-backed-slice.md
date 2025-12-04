# Disk-Backed Slice (pkg/diskslice)

## Overview

The `diskslice` package provides a transparent disk-backed slice implementation for Go. It behaves like a normal Go slice for small datasets but automatically spills to disk-backed storage when a configurable memory threshold is exceeded.

This is part of tvarr's memory efficiency strategy for handling large M3U playlists and XMLTV program guides that may contain millions of entries.

## Problem Statement

tvarr processes IPTV data through a pipeline that may handle:
- Channels: Thousands to tens of thousands of entries
- Programs: Hundreds of thousands to millions of EPG entries

The original implementation held all data in memory as Go slices. For very large datasets (e.g., 7-day EPG with 50,000 channels), this could exceed available RAM, causing OOM kills or severe performance degradation from swap thrashing.

## Design Goals

1. **Transparency**: Stage code should not need to know whether data is in memory or on disk
2. **Efficiency**: Small datasets should have near-zero overhead
3. **Scalability**: Large datasets should work without OOM, trading CPU for memory
4. **Simplicity**: Simple API that mimics slice operations
5. **Safety**: Automatic cleanup of temporary files

## Architecture

### Memory Mode (Default)

When data size is below the threshold, `DiskSlice[T]` behaves exactly like a Go slice:

```
┌─────────────────────────────────────┐
│           DiskSlice[T]              │
│  ┌─────────────────────────────┐    │
│  │     memItems []T            │    │
│  │   [item0, item1, item2...]  │    │
│  └─────────────────────────────┘    │
└─────────────────────────────────────┘
```

### Disk Mode (After Threshold)

When memory threshold is exceeded, all items are serialized to a JSONL file:

```
┌─────────────────────────────────────┐
│           DiskSlice[T]              │
│  ┌─────────────────────────────┐    │
│  │     offsets []int64         │    │ ← Only this stays in memory
│  │   [0, 145, 290, 435...]     │    │
│  └─────────────────────────────┘    │
│               │                      │
│               ▼                      │
│  ┌─────────────────────────────┐    │
│  │     /tmp/diskslice-xxx.jsonl │   │ ← Items on disk
│  │   {"id":1,"name":"a"...}     │   │
│  │   {"id":2,"name":"b"...}     │   │
│  │   {"id":3,"name":"c"...}     │   │
│  └─────────────────────────────┘    │
└─────────────────────────────────────┘
```

## API

### Creating a DiskSlice

```go
// With custom options
ds, err := diskslice.New[MyType](diskslice.Options{
    MemoryThreshold:   100 * 1024 * 1024, // 100MB
    TempDir:           "/tmp",
    EstimatedItemSize: 256,               // bytes per item estimate
    Name:              "programs",
})
if err != nil { ... }
defer ds.Close() // Always close to clean up temp files

// With defaults (500MB threshold)
ds, err := diskslice.NewWithDefaults[MyType]()
```

### Adding Items

```go
// Single item
err := ds.Append(item)

// Multiple items
err := ds.AppendSlice(items)
```

### Accessing Items

```go
// By index (random access)
item, err := ds.Get(index)

// Iteration (preferred - memory efficient)
err := ds.For(func(index int, item *MyType) bool {
    process(item)
    return true // continue iteration, false to stop
})

// Iterator pattern
iter, err := ds.NewIterator()
defer iter.Close()
for item := iter.Next(); item != nil; item = iter.Next() {
    process(item)
}
if iter.Err() != nil { ... }
```

### Metadata

```go
length := ds.Len()                      // Number of items
spilled := ds.IsSpilled()               // True if on disk
memUsage := ds.EstimatedMemoryUsage()   // Bytes in memory
```

### Converting Back to Slice

```go
// Warning: Loads all items into memory
slice, err := ds.ToSlice()
```

## Performance Characteristics

| Operation | Memory Mode | Disk Mode |
|-----------|------------|-----------|
| Append | O(1) amortized | O(n) for encoding |
| Get(i) | O(1) | O(1) seek + O(n) decode |
| For() | O(n) | O(n) sequential read |
| Len() | O(1) | O(1) |
| Memory | O(n) | O(1) + offset array |

### When to Use

- **Use DiskSlice**: When dataset size is unpredictable and may exceed memory
- **Use regular slice**: When dataset is guaranteed to be small (<10MB) or when maximum performance is required

## Configuration Guidelines

### Memory Threshold

The threshold should be set based on:
- Available system memory
- Number of concurrent operations
- Size of other in-memory structures

Recommended starting points:
- Desktop/development: 100MB
- Production (2GB RAM): 200MB
- Production (8GB RAM): 500MB

### Estimated Item Size

Used to predict when spilling will occur. Overestimating is safer than underestimating.

Typical values:
- Channel records: 200-500 bytes
- Program records: 500-2000 bytes

## Implementation Notes

### Serialization Format

Items are serialized as newline-delimited JSON (JSONL). This format:
- Allows sequential streaming reads
- Supports arbitrary Go structs
- Is human-readable for debugging
- Has acceptable encode/decode performance

Future optimization: Consider MessagePack or Protocol Buffers for higher performance.

### File Handling

- Temp files are created in the configured TempDir
- Files are named with a random suffix: `{name}-{random}.jsonl`
- Files are automatically deleted on Close()
- If the process crashes, files remain in TempDir (OS cleanup applies)

### Thread Safety

- `Append()` is thread-safe (uses mutex)
- `For()` and iteration are safe for concurrent reads after population is complete
- Mixed read/write is NOT safe

## Future Enhancements

### Potential Improvements

1. **Memory-mapped files**: Use `mmap` for disk mode to reduce syscall overhead
2. **Binary serialization**: Replace JSON with a more compact binary format
3. **Compression**: Add optional gzip compression for disk storage
4. **Sorted iteration**: Support sorted access without full in-memory sort
5. **Batch operations**: Optimize bulk insert/read operations

### Not Planned

- **Random write**: Modifying items after insertion is not supported
- **Delete**: Removing individual items is not supported
- **Persistence**: Files are temporary; use a database for persistence

## Related

- [Logo Caching](../logo-caching.md) - Another memory-conscious subsystem
- Task T066 in `specs/004-e2e-pipeline-validation/tasks.md` - Research task for this feature

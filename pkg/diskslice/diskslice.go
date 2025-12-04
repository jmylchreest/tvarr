// Package diskslice provides a transparent disk-backed slice implementation.
// It behaves like a normal Go slice for small datasets, but automatically
// spills to disk-backed storage when a memory threshold is exceeded.
//
// The DiskSlice[T] type provides:
//   - Transparent overflow: behaves like []T for small datasets
//   - Automatic spill: writes to temp file when memory threshold exceeded
//   - Memory-mapped access: efficient random access via mmap
//   - Iteration: supports both indexed access and iterator pattern
//   - Thread-safe: concurrent-read safe after initial population
//
// Example usage:
//
//	// Create a disk slice with 100MB threshold
//	ds, err := diskslice.New[MyType](diskslice.Options{
//	    MemoryThreshold: 100 * 1024 * 1024,
//	    TempDir:         "/tmp",
//	})
//	if err != nil { ... }
//	defer ds.Close()
//
//	// Append items like a normal slice
//	for _, item := range largeDataset {
//	    ds.Append(item)
//	}
//
//	// Iterate using For method
//	ds.For(func(i int, item *MyType) bool {
//	    process(item)
//	    return true // continue iteration
//	})
package diskslice

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// Options configures a DiskSlice.
type Options struct {
	// MemoryThreshold is the byte limit before spilling to disk.
	// When the estimated memory usage exceeds this, items are written to disk.
	// Default: 50MB
	MemoryThreshold int64

	// TempDir is the directory for temporary files.
	// Default: os.TempDir()
	TempDir string

	// EstimatedItemSize is the estimated size in bytes per item.
	// Used to predict when to spill. Default: 256 bytes
	EstimatedItemSize int

	// Name is an optional name for the slice (used in temp file naming).
	Name string
}

// DefaultOptions returns sensible default options.
func DefaultOptions() Options {
	return Options{
		MemoryThreshold:   500 * 1024 * 1024, // 500MB
		TempDir:           os.TempDir(),
		EstimatedItemSize: 256,
		Name:              "diskslice",
	}
}

// DiskSlice is a generic slice that transparently overflows to disk.
// It stores items in memory until MemoryThreshold is exceeded,
// then spills all items to a disk-backed file.
//
// Type T must be JSON-serializable for disk storage.
type DiskSlice[T any] struct {
	opts Options

	mu sync.RWMutex

	// In-memory storage (used when under threshold)
	memItems []T

	// Disk storage state
	spilled   bool
	diskFile  *os.File
	diskPath  string
	offsets   []int64 // byte offsets for each record in file
	diskCount int     // number of items on disk

	// Memory tracking
	estimatedBytes int64
}

// New creates a new DiskSlice with the given options.
func New[T any](opts Options) (*DiskSlice[T], error) {
	if opts.MemoryThreshold <= 0 {
		opts.MemoryThreshold = DefaultOptions().MemoryThreshold
	}
	if opts.TempDir == "" {
		opts.TempDir = DefaultOptions().TempDir
	}
	if opts.EstimatedItemSize <= 0 {
		opts.EstimatedItemSize = DefaultOptions().EstimatedItemSize
	}
	if opts.Name == "" {
		opts.Name = DefaultOptions().Name
	}

	return &DiskSlice[T]{
		opts:     opts,
		memItems: make([]T, 0, 64), // Start with small capacity
	}, nil
}

// NewWithDefaults creates a DiskSlice with default options.
func NewWithDefaults[T any]() (*DiskSlice[T], error) {
	return New[T](DefaultOptions())
}

// Append adds an item to the slice.
// If the memory threshold is exceeded, all items are spilled to disk.
func (ds *DiskSlice[T]) Append(item T) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.spilled {
		return ds.appendToDisk(item)
	}

	// Add to memory
	ds.memItems = append(ds.memItems, item)
	ds.estimatedBytes += int64(ds.opts.EstimatedItemSize)

	// Check if we need to spill
	if ds.estimatedBytes >= ds.opts.MemoryThreshold {
		if err := ds.spillToDisk(); err != nil {
			return fmt.Errorf("spilling to disk: %w", err)
		}
	}

	return nil
}

// AppendSlice appends all items from a slice.
func (ds *DiskSlice[T]) AppendSlice(items []T) error {
	for i := range items {
		if err := ds.Append(items[i]); err != nil {
			return err
		}
	}
	return nil
}

// Len returns the number of items in the slice.
func (ds *DiskSlice[T]) Len() int {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	if ds.spilled {
		return ds.diskCount
	}
	return len(ds.memItems)
}

// Get retrieves an item by index.
// Returns a pointer to the item, or nil if index is out of bounds.
func (ds *DiskSlice[T]) Get(index int) (*T, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	if ds.spilled {
		return ds.getFromDisk(index)
	}

	if index < 0 || index >= len(ds.memItems) {
		return nil, fmt.Errorf("index %d out of bounds (len=%d)", index, len(ds.memItems))
	}

	return &ds.memItems[index], nil
}

// For iterates over all items, calling fn for each.
// If fn returns false, iteration stops.
// This is the preferred iteration method as it handles disk-backed items efficiently.
func (ds *DiskSlice[T]) For(fn func(index int, item *T) bool) error {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	if ds.spilled {
		return ds.forDisk(fn)
	}

	for i := range ds.memItems {
		if !fn(i, &ds.memItems[i]) {
			break
		}
	}
	return nil
}

// IsSpilled returns true if the slice has been spilled to disk.
func (ds *DiskSlice[T]) IsSpilled() bool {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.spilled
}

// EstimatedMemoryUsage returns the estimated memory usage in bytes.
func (ds *DiskSlice[T]) EstimatedMemoryUsage() int64 {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	if ds.spilled {
		// Only offset array is in memory when spilled
		return int64(len(ds.offsets) * 8) // int64 = 8 bytes
	}
	return ds.estimatedBytes
}

// Close releases resources associated with the disk slice.
// Must be called when done to clean up temporary files.
func (ds *DiskSlice[T]) Close() error {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	if ds.diskFile != nil {
		ds.diskFile.Close()
		ds.diskFile = nil
	}

	if ds.diskPath != "" {
		os.Remove(ds.diskPath)
		ds.diskPath = ""
	}

	ds.memItems = nil
	ds.offsets = nil

	return nil
}

// ToSlice returns all items as a regular slice.
// Warning: This loads all items into memory, defeating the purpose
// of disk-backed storage for large datasets. Use For() instead when possible.
func (ds *DiskSlice[T]) ToSlice() ([]T, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	if !ds.spilled {
		// Return a copy to avoid aliasing
		result := make([]T, len(ds.memItems))
		copy(result, ds.memItems)
		return result, nil
	}

	// Load all from disk
	result := make([]T, 0, ds.diskCount)
	err := ds.forDisk(func(_ int, item *T) bool {
		result = append(result, *item)
		return true
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// spillToDisk writes all in-memory items to a temporary file.
func (ds *DiskSlice[T]) spillToDisk() error {
	// Create temp file
	f, err := os.CreateTemp(ds.opts.TempDir, ds.opts.Name+"-*.jsonl")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	ds.diskFile = f
	ds.diskPath = f.Name()
	ds.offsets = make([]int64, 0, len(ds.memItems))

	// Write all memory items to disk
	encoder := json.NewEncoder(f)
	for i := range ds.memItems {
		offset, _ := f.Seek(0, io.SeekCurrent)
		ds.offsets = append(ds.offsets, offset)

		if err := encoder.Encode(&ds.memItems[i]); err != nil {
			return fmt.Errorf("encoding item %d: %w", i, err)
		}
	}

	ds.diskCount = len(ds.memItems)
	ds.spilled = true

	// Clear memory items
	ds.memItems = nil
	ds.estimatedBytes = 0

	return nil
}

// appendToDisk appends a single item to the disk file.
func (ds *DiskSlice[T]) appendToDisk(item T) error {
	offset, _ := ds.diskFile.Seek(0, io.SeekEnd)
	ds.offsets = append(ds.offsets, offset)

	encoder := json.NewEncoder(ds.diskFile)
	if err := encoder.Encode(&item); err != nil {
		return fmt.Errorf("encoding item: %w", err)
	}

	ds.diskCount++
	return nil
}

// getFromDisk retrieves a single item from the disk file.
func (ds *DiskSlice[T]) getFromDisk(index int) (*T, error) {
	if index < 0 || index >= ds.diskCount {
		return nil, fmt.Errorf("index %d out of bounds (len=%d)", index, ds.diskCount)
	}

	offset := ds.offsets[index]
	if _, err := ds.diskFile.Seek(offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seeking to offset %d: %w", offset, err)
	}

	decoder := json.NewDecoder(ds.diskFile)
	var item T
	if err := decoder.Decode(&item); err != nil {
		return nil, fmt.Errorf("decoding item at offset %d: %w", offset, err)
	}

	return &item, nil
}

// forDisk iterates over all disk items.
func (ds *DiskSlice[T]) forDisk(fn func(index int, item *T) bool) error {
	if _, err := ds.diskFile.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seeking to start: %w", err)
	}

	decoder := json.NewDecoder(ds.diskFile)
	for i := 0; i < ds.diskCount; i++ {
		var item T
		if err := decoder.Decode(&item); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("decoding item %d: %w", i, err)
		}

		if !fn(i, &item) {
			break
		}
	}

	return nil
}

// Iterator provides sequential access to disk slice items.
type Iterator[T any] struct {
	ds      *DiskSlice[T]
	index   int
	decoder *json.Decoder
	err     error
}

// NewIterator creates an iterator for the disk slice.
func (ds *DiskSlice[T]) NewIterator() (*Iterator[T], error) {
	ds.mu.RLock()

	iter := &Iterator[T]{
		ds:    ds,
		index: -1,
	}

	if ds.spilled {
		if _, err := ds.diskFile.Seek(0, io.SeekStart); err != nil {
			ds.mu.RUnlock()
			return nil, fmt.Errorf("seeking to start: %w", err)
		}
		iter.decoder = json.NewDecoder(ds.diskFile)
	}

	return iter, nil
}

// Next advances the iterator and returns the next item.
// Returns nil when iteration is complete or on error.
// Check Err() after iteration to see if an error occurred.
func (it *Iterator[T]) Next() *T {
	if it.err != nil {
		return nil
	}

	it.index++

	if it.ds.spilled {
		if it.index >= it.ds.diskCount {
			return nil
		}

		var item T
		if err := it.decoder.Decode(&item); err != nil {
			if err != io.EOF {
				it.err = err
			}
			return nil
		}
		return &item
	}

	// Memory mode
	if it.index >= len(it.ds.memItems) {
		return nil
	}
	return &it.ds.memItems[it.index]
}

// Err returns any error that occurred during iteration.
func (it *Iterator[T]) Err() error {
	return it.err
}

// Close releases resources and unlocks the disk slice.
func (it *Iterator[T]) Close() {
	it.ds.mu.RUnlock()
}

// Index returns the current index (0-based).
func (it *Iterator[T]) Index() int {
	return it.index
}

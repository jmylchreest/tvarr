package diskslice_test

import (
	"os"
	"testing"

	"github.com/jmylchreest/tvarr/pkg/diskslice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestItem is a simple struct for testing.
type TestItem struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Value float64 `json:"value"`
}

func TestNew(t *testing.T) {
	t.Run("creates with defaults", func(t *testing.T) {
		ds, err := diskslice.NewWithDefaults[TestItem]()
		require.NoError(t, err)
		defer ds.Close()

		assert.Equal(t, 0, ds.Len())
		assert.False(t, ds.IsSpilled())
	})

	t.Run("creates with custom options", func(t *testing.T) {
		ds, err := diskslice.New[TestItem](diskslice.Options{
			MemoryThreshold:   1024 * 1024, // 1MB
			TempDir:           os.TempDir(),
			EstimatedItemSize: 100,
			Name:              "test-slice",
		})
		require.NoError(t, err)
		defer ds.Close()

		assert.Equal(t, 0, ds.Len())
	})
}

func TestAppend(t *testing.T) {
	t.Run("appends items in memory", func(t *testing.T) {
		ds, err := diskslice.NewWithDefaults[TestItem]()
		require.NoError(t, err)
		defer ds.Close()

		err = ds.Append(TestItem{ID: 1, Name: "first", Value: 1.0})
		require.NoError(t, err)

		err = ds.Append(TestItem{ID: 2, Name: "second", Value: 2.0})
		require.NoError(t, err)

		assert.Equal(t, 2, ds.Len())
		assert.False(t, ds.IsSpilled())
	})

	t.Run("spills to disk when threshold exceeded", func(t *testing.T) {
		ds, err := diskslice.New[TestItem](diskslice.Options{
			MemoryThreshold:   500, // Very small threshold
			EstimatedItemSize: 100, // Will trigger after 5 items
		})
		require.NoError(t, err)
		defer ds.Close()

		// Add enough items to exceed threshold
		for i := 0; i < 10; i++ {
			err := ds.Append(TestItem{ID: i, Name: "test", Value: float64(i)})
			require.NoError(t, err)
		}

		assert.Equal(t, 10, ds.Len())
		assert.True(t, ds.IsSpilled())
	})
}

func TestAppendSlice(t *testing.T) {
	ds, err := diskslice.NewWithDefaults[TestItem]()
	require.NoError(t, err)
	defer ds.Close()

	items := []TestItem{
		{ID: 1, Name: "a", Value: 1.0},
		{ID: 2, Name: "b", Value: 2.0},
		{ID: 3, Name: "c", Value: 3.0},
	}

	err = ds.AppendSlice(items)
	require.NoError(t, err)

	assert.Equal(t, 3, ds.Len())
}

func TestGet(t *testing.T) {
	t.Run("gets items from memory", func(t *testing.T) {
		ds, err := diskslice.NewWithDefaults[TestItem]()
		require.NoError(t, err)
		defer ds.Close()

		ds.Append(TestItem{ID: 1, Name: "first", Value: 1.5})
		ds.Append(TestItem{ID: 2, Name: "second", Value: 2.5})

		item, err := ds.Get(0)
		require.NoError(t, err)
		assert.Equal(t, 1, item.ID)
		assert.Equal(t, "first", item.Name)
		assert.Equal(t, 1.5, item.Value)

		item, err = ds.Get(1)
		require.NoError(t, err)
		assert.Equal(t, 2, item.ID)
	})

	t.Run("gets items from disk", func(t *testing.T) {
		ds, err := diskslice.New[TestItem](diskslice.Options{
			MemoryThreshold:   100,
			EstimatedItemSize: 50,
		})
		require.NoError(t, err)
		defer ds.Close()

		// Add items to trigger spill
		for i := 0; i < 10; i++ {
			ds.Append(TestItem{ID: i, Name: "item", Value: float64(i) * 1.1})
		}
		require.True(t, ds.IsSpilled())

		// Test random access
		item, err := ds.Get(5)
		require.NoError(t, err)
		assert.Equal(t, 5, item.ID)
		assert.InDelta(t, 5.5, item.Value, 0.01)

		item, err = ds.Get(0)
		require.NoError(t, err)
		assert.Equal(t, 0, item.ID)

		item, err = ds.Get(9)
		require.NoError(t, err)
		assert.Equal(t, 9, item.ID)
	})

	t.Run("returns error for out of bounds", func(t *testing.T) {
		ds, err := diskslice.NewWithDefaults[TestItem]()
		require.NoError(t, err)
		defer ds.Close()

		ds.Append(TestItem{ID: 1})

		_, err = ds.Get(-1)
		assert.Error(t, err)

		_, err = ds.Get(1)
		assert.Error(t, err)

		_, err = ds.Get(100)
		assert.Error(t, err)
	})
}

func TestFor(t *testing.T) {
	t.Run("iterates over memory items", func(t *testing.T) {
		ds, err := diskslice.NewWithDefaults[TestItem]()
		require.NoError(t, err)
		defer ds.Close()

		for i := 0; i < 5; i++ {
			ds.Append(TestItem{ID: i, Name: "test"})
		}

		var count int
		var sum int
		err = ds.For(func(index int, item *TestItem) bool {
			assert.Equal(t, count, index)
			sum += item.ID
			count++
			return true
		})
		require.NoError(t, err)

		assert.Equal(t, 5, count)
		assert.Equal(t, 0+1+2+3+4, sum)
	})

	t.Run("iterates over disk items", func(t *testing.T) {
		ds, err := diskslice.New[TestItem](diskslice.Options{
			MemoryThreshold:   100,
			EstimatedItemSize: 50,
		})
		require.NoError(t, err)
		defer ds.Close()

		for i := 0; i < 10; i++ {
			ds.Append(TestItem{ID: i})
		}
		require.True(t, ds.IsSpilled())

		var count int
		err = ds.For(func(index int, item *TestItem) bool {
			assert.Equal(t, count, item.ID)
			count++
			return true
		})
		require.NoError(t, err)
		assert.Equal(t, 10, count)
	})

	t.Run("stops early when fn returns false", func(t *testing.T) {
		ds, err := diskslice.NewWithDefaults[TestItem]()
		require.NoError(t, err)
		defer ds.Close()

		for i := 0; i < 10; i++ {
			ds.Append(TestItem{ID: i})
		}

		var count int
		ds.For(func(index int, item *TestItem) bool {
			count++
			return index < 4 // Stop after 5 items
		})

		assert.Equal(t, 5, count)
	})
}

func TestIterator(t *testing.T) {
	t.Run("iterates with Next pattern", func(t *testing.T) {
		ds, err := diskslice.NewWithDefaults[TestItem]()
		require.NoError(t, err)
		defer ds.Close()

		for i := 0; i < 5; i++ {
			ds.Append(TestItem{ID: i, Name: "iter"})
		}

		iter, err := ds.NewIterator()
		require.NoError(t, err)
		defer iter.Close()

		var count int
		for item := iter.Next(); item != nil; item = iter.Next() {
			assert.Equal(t, count, item.ID)
			assert.Equal(t, count, iter.Index())
			count++
		}

		assert.NoError(t, iter.Err())
		assert.Equal(t, 5, count)
	})

	t.Run("iterates disk items with Next", func(t *testing.T) {
		ds, err := diskslice.New[TestItem](diskslice.Options{
			MemoryThreshold:   100,
			EstimatedItemSize: 50,
		})
		require.NoError(t, err)
		defer ds.Close()

		for i := 0; i < 10; i++ {
			ds.Append(TestItem{ID: i})
		}
		require.True(t, ds.IsSpilled())

		iter, err := ds.NewIterator()
		require.NoError(t, err)
		defer iter.Close()

		var count int
		for item := iter.Next(); item != nil; item = iter.Next() {
			assert.Equal(t, count, item.ID)
			count++
		}

		assert.NoError(t, iter.Err())
		assert.Equal(t, 10, count)
	})
}

func TestToSlice(t *testing.T) {
	t.Run("returns copy from memory", func(t *testing.T) {
		ds, err := diskslice.NewWithDefaults[TestItem]()
		require.NoError(t, err)
		defer ds.Close()

		ds.Append(TestItem{ID: 1})
		ds.Append(TestItem{ID: 2})

		slice, err := ds.ToSlice()
		require.NoError(t, err)

		assert.Len(t, slice, 2)
		assert.Equal(t, 1, slice[0].ID)
		assert.Equal(t, 2, slice[1].ID)

		// Verify it's a copy
		slice[0].ID = 999
		item, _ := ds.Get(0)
		assert.Equal(t, 1, item.ID)
	})

	t.Run("loads all from disk", func(t *testing.T) {
		ds, err := diskslice.New[TestItem](diskslice.Options{
			MemoryThreshold:   100,
			EstimatedItemSize: 50,
		})
		require.NoError(t, err)
		defer ds.Close()

		for i := 0; i < 10; i++ {
			ds.Append(TestItem{ID: i})
		}
		require.True(t, ds.IsSpilled())

		slice, err := ds.ToSlice()
		require.NoError(t, err)

		assert.Len(t, slice, 10)
		for i, item := range slice {
			assert.Equal(t, i, item.ID)
		}
	})
}

func TestClose(t *testing.T) {
	t.Run("cleans up disk file", func(t *testing.T) {
		ds, err := diskslice.New[TestItem](diskslice.Options{
			MemoryThreshold:   100,
			EstimatedItemSize: 50,
		})
		require.NoError(t, err)

		// Force spill
		for i := 0; i < 10; i++ {
			ds.Append(TestItem{ID: i})
		}
		require.True(t, ds.IsSpilled())

		// Close should remove temp file
		err = ds.Close()
		assert.NoError(t, err)
	})

	t.Run("handles close on memory-only slice", func(t *testing.T) {
		ds, err := diskslice.NewWithDefaults[TestItem]()
		require.NoError(t, err)

		ds.Append(TestItem{ID: 1})

		err = ds.Close()
		assert.NoError(t, err)
	})
}

func TestEstimatedMemoryUsage(t *testing.T) {
	t.Run("tracks memory usage", func(t *testing.T) {
		ds, err := diskslice.New[TestItem](diskslice.Options{
			MemoryThreshold:   10000,
			EstimatedItemSize: 100,
		})
		require.NoError(t, err)
		defer ds.Close()

		assert.Equal(t, int64(0), ds.EstimatedMemoryUsage())

		ds.Append(TestItem{ID: 1})
		assert.Equal(t, int64(100), ds.EstimatedMemoryUsage())

		ds.Append(TestItem{ID: 2})
		assert.Equal(t, int64(200), ds.EstimatedMemoryUsage())
	})

	t.Run("reports offset array size when spilled", func(t *testing.T) {
		ds, err := diskslice.New[TestItem](diskslice.Options{
			MemoryThreshold:   100,
			EstimatedItemSize: 50,
		})
		require.NoError(t, err)
		defer ds.Close()

		for i := 0; i < 10; i++ {
			ds.Append(TestItem{ID: i})
		}
		require.True(t, ds.IsSpilled())

		// After spill, only offset array is in memory (10 items * 8 bytes = 80)
		assert.Equal(t, int64(80), ds.EstimatedMemoryUsage())
	})
}

func TestContinuedAppendAfterSpill(t *testing.T) {
	ds, err := diskslice.New[TestItem](diskslice.Options{
		MemoryThreshold:   100,
		EstimatedItemSize: 50,
	})
	require.NoError(t, err)
	defer ds.Close()

	// Add items to trigger spill
	for i := 0; i < 5; i++ {
		ds.Append(TestItem{ID: i})
	}
	require.True(t, ds.IsSpilled())
	assert.Equal(t, 5, ds.Len())

	// Continue appending after spill
	for i := 5; i < 10; i++ {
		err := ds.Append(TestItem{ID: i})
		require.NoError(t, err)
	}
	assert.Equal(t, 10, ds.Len())

	// Verify all items are accessible
	for i := 0; i < 10; i++ {
		item, err := ds.Get(i)
		require.NoError(t, err)
		assert.Equal(t, i, item.ID)
	}
}

func TestEmptySlice(t *testing.T) {
	ds, err := diskslice.NewWithDefaults[TestItem]()
	require.NoError(t, err)
	defer ds.Close()

	assert.Equal(t, 0, ds.Len())
	assert.False(t, ds.IsSpilled())

	slice, err := ds.ToSlice()
	require.NoError(t, err)
	assert.Empty(t, slice)

	var count int
	ds.For(func(i int, item *TestItem) bool {
		count++
		return true
	})
	assert.Equal(t, 0, count)
}

// Benchmark tests

func BenchmarkAppendMemory(b *testing.B) {
	ds, _ := diskslice.New[TestItem](diskslice.Options{
		MemoryThreshold: 1024 * 1024 * 1024, // 1GB - won't spill
	})
	defer ds.Close()

	item := TestItem{ID: 1, Name: "benchmark", Value: 123.456}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ds.Append(item)
	}
}

func BenchmarkForMemory(b *testing.B) {
	ds, _ := diskslice.New[TestItem](diskslice.Options{
		MemoryThreshold: 1024 * 1024 * 1024,
	})
	defer ds.Close()

	for i := 0; i < 10000; i++ {
		ds.Append(TestItem{ID: i})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ds.For(func(_ int, _ *TestItem) bool {
			return true
		})
	}
}

func BenchmarkForDisk(b *testing.B) {
	ds, _ := diskslice.New[TestItem](diskslice.Options{
		MemoryThreshold:   1024, // Force spill quickly
		EstimatedItemSize: 100,
	})
	defer ds.Close()

	for i := 0; i < 10000; i++ {
		ds.Append(TestItem{ID: i})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ds.For(func(_ int, _ *TestItem) bool {
			return true
		})
	}
}

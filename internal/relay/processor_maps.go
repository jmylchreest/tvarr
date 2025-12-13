// Package relay provides stream relay functionality.
package relay

import (
	"sync"
)

// ProcessorMap is a type-safe concurrent map for processors.
// It wraps sync.Map to provide type safety for CodecVariant keys
// and different processor value types.
// This eliminates the single mutex bottleneck that was causing
// deadlocks and contention in session stats collection.

// HLSTSProcessorMap is a concurrent map for HLS-TS processors.
type HLSTSProcessorMap struct {
	m sync.Map // map[CodecVariant]*HLSTSProcessor
}

// Load returns the processor for the given variant.
func (pm *HLSTSProcessorMap) Load(variant CodecVariant) (*HLSTSProcessor, bool) {
	v, ok := pm.m.Load(variant)
	if !ok {
		return nil, false
	}
	return v.(*HLSTSProcessor), true
}

// Store sets the processor for the given variant.
func (pm *HLSTSProcessorMap) Store(variant CodecVariant, processor *HLSTSProcessor) {
	pm.m.Store(variant, processor)
}

// LoadOrStore returns the existing processor or stores and returns the new one.
func (pm *HLSTSProcessorMap) LoadOrStore(variant CodecVariant, processor *HLSTSProcessor) (*HLSTSProcessor, bool) {
	v, loaded := pm.m.LoadOrStore(variant, processor)
	return v.(*HLSTSProcessor), loaded
}

// Delete removes the processor for the given variant.
func (pm *HLSTSProcessorMap) Delete(variant CodecVariant) {
	pm.m.Delete(variant)
}

// Range calls f for each processor. If f returns false, iteration stops.
func (pm *HLSTSProcessorMap) Range(f func(variant CodecVariant, processor *HLSTSProcessor) bool) {
	pm.m.Range(func(key, value any) bool {
		return f(key.(CodecVariant), value.(*HLSTSProcessor))
	})
}

// Len returns the number of processors (O(n) - iterates all entries).
func (pm *HLSTSProcessorMap) Len() int {
	count := 0
	pm.m.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// Clear removes all processors and returns them for cleanup.
func (pm *HLSTSProcessorMap) Clear() []*HLSTSProcessor {
	var processors []*HLSTSProcessor
	pm.m.Range(func(key, value any) bool {
		processors = append(processors, value.(*HLSTSProcessor))
		pm.m.Delete(key)
		return true
	})
	return processors
}

// HLSfMP4ProcessorMap is a concurrent map for HLS-fMP4 processors.
type HLSfMP4ProcessorMap struct {
	m sync.Map // map[CodecVariant]*HLSfMP4Processor
}

// Load returns the processor for the given variant.
func (pm *HLSfMP4ProcessorMap) Load(variant CodecVariant) (*HLSfMP4Processor, bool) {
	v, ok := pm.m.Load(variant)
	if !ok {
		return nil, false
	}
	return v.(*HLSfMP4Processor), true
}

// Store sets the processor for the given variant.
func (pm *HLSfMP4ProcessorMap) Store(variant CodecVariant, processor *HLSfMP4Processor) {
	pm.m.Store(variant, processor)
}

// LoadOrStore returns the existing processor or stores and returns the new one.
func (pm *HLSfMP4ProcessorMap) LoadOrStore(variant CodecVariant, processor *HLSfMP4Processor) (*HLSfMP4Processor, bool) {
	v, loaded := pm.m.LoadOrStore(variant, processor)
	return v.(*HLSfMP4Processor), loaded
}

// Delete removes the processor for the given variant.
func (pm *HLSfMP4ProcessorMap) Delete(variant CodecVariant) {
	pm.m.Delete(variant)
}

// Range calls f for each processor. If f returns false, iteration stops.
func (pm *HLSfMP4ProcessorMap) Range(f func(variant CodecVariant, processor *HLSfMP4Processor) bool) {
	pm.m.Range(func(key, value any) bool {
		return f(key.(CodecVariant), value.(*HLSfMP4Processor))
	})
}

// Len returns the number of processors (O(n) - iterates all entries).
func (pm *HLSfMP4ProcessorMap) Len() int {
	count := 0
	pm.m.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// Clear removes all processors and returns them for cleanup.
func (pm *HLSfMP4ProcessorMap) Clear() []*HLSfMP4Processor {
	var processors []*HLSfMP4Processor
	pm.m.Range(func(key, value any) bool {
		processors = append(processors, value.(*HLSfMP4Processor))
		pm.m.Delete(key)
		return true
	})
	return processors
}

// DASHProcessorMap is a concurrent map for DASH processors.
type DASHProcessorMap struct {
	m sync.Map // map[CodecVariant]*DASHProcessor
}

// Load returns the processor for the given variant.
func (pm *DASHProcessorMap) Load(variant CodecVariant) (*DASHProcessor, bool) {
	v, ok := pm.m.Load(variant)
	if !ok {
		return nil, false
	}
	return v.(*DASHProcessor), true
}

// Store sets the processor for the given variant.
func (pm *DASHProcessorMap) Store(variant CodecVariant, processor *DASHProcessor) {
	pm.m.Store(variant, processor)
}

// LoadOrStore returns the existing processor or stores and returns the new one.
func (pm *DASHProcessorMap) LoadOrStore(variant CodecVariant, processor *DASHProcessor) (*DASHProcessor, bool) {
	v, loaded := pm.m.LoadOrStore(variant, processor)
	return v.(*DASHProcessor), loaded
}

// Delete removes the processor for the given variant.
func (pm *DASHProcessorMap) Delete(variant CodecVariant) {
	pm.m.Delete(variant)
}

// Range calls f for each processor. If f returns false, iteration stops.
func (pm *DASHProcessorMap) Range(f func(variant CodecVariant, processor *DASHProcessor) bool) {
	pm.m.Range(func(key, value any) bool {
		return f(key.(CodecVariant), value.(*DASHProcessor))
	})
}

// Len returns the number of processors (O(n) - iterates all entries).
func (pm *DASHProcessorMap) Len() int {
	count := 0
	pm.m.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// Clear removes all processors and returns them for cleanup.
func (pm *DASHProcessorMap) Clear() []*DASHProcessor {
	var processors []*DASHProcessor
	pm.m.Range(func(key, value any) bool {
		processors = append(processors, value.(*DASHProcessor))
		pm.m.Delete(key)
		return true
	})
	return processors
}

// MPEGTSProcessorMap is a concurrent map for MPEG-TS processors.
type MPEGTSProcessorMap struct {
	m sync.Map // map[CodecVariant]*MPEGTSProcessor
}

// Load returns the processor for the given variant.
func (pm *MPEGTSProcessorMap) Load(variant CodecVariant) (*MPEGTSProcessor, bool) {
	v, ok := pm.m.Load(variant)
	if !ok {
		return nil, false
	}
	return v.(*MPEGTSProcessor), true
}

// Store sets the processor for the given variant.
func (pm *MPEGTSProcessorMap) Store(variant CodecVariant, processor *MPEGTSProcessor) {
	pm.m.Store(variant, processor)
}

// LoadOrStore returns the existing processor or stores and returns the new one.
func (pm *MPEGTSProcessorMap) LoadOrStore(variant CodecVariant, processor *MPEGTSProcessor) (*MPEGTSProcessor, bool) {
	v, loaded := pm.m.LoadOrStore(variant, processor)
	return v.(*MPEGTSProcessor), loaded
}

// Delete removes the processor for the given variant.
func (pm *MPEGTSProcessorMap) Delete(variant CodecVariant) {
	pm.m.Delete(variant)
}

// Range calls f for each processor. If f returns false, iteration stops.
func (pm *MPEGTSProcessorMap) Range(f func(variant CodecVariant, processor *MPEGTSProcessor) bool) {
	pm.m.Range(func(key, value any) bool {
		return f(key.(CodecVariant), value.(*MPEGTSProcessor))
	})
}

// Len returns the number of processors (O(n) - iterates all entries).
func (pm *MPEGTSProcessorMap) Len() int {
	count := 0
	pm.m.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// Clear removes all processors and returns them for cleanup.
func (pm *MPEGTSProcessorMap) Clear() []*MPEGTSProcessor {
	var processors []*MPEGTSProcessor
	pm.m.Range(func(key, value any) bool {
		processors = append(processors, value.(*MPEGTSProcessor))
		pm.m.Delete(key)
		return true
	})
	return processors
}

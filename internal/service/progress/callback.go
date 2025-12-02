package progress

// ProgressReporter is the interface passed to handlers for progress reporting.
// Implementations automatically apply throttling to prevent excessive updates.
// This interface allows handlers to report progress without knowing about
// the Progress Service internals.
type ProgressReporter interface {
	// ReportProgress reports progress (0.0 to 1.0) with an optional message.
	// Progress updates are throttled by the progress service (2 seconds).
	ReportProgress(progress float64, message string)

	// ReportItemProgress reports item-level progress with counts.
	// Progress updates are throttled by the progress service (2 seconds).
	ReportItemProgress(current, total int, itemName string)
}

// stageReporter implements ProgressReporter for a specific stage.
// It wraps a StageUpdater to provide the callback interface.
type stageReporter struct {
	updater *StageUpdater
}

// ReportProgress reports progress (0.0 to 1.0) with an optional message.
func (r *stageReporter) ReportProgress(progress float64, message string) {
	if r.updater != nil {
		r.updater.SetProgress(progress, message)
	}
}

// ReportItemProgress reports item-level progress with counts.
func (r *stageReporter) ReportItemProgress(current, total int, itemName string) {
	if r.updater != nil {
		r.updater.SetItemProgress(current, total, itemName)
	}
}

// NilReporter is a no-op ProgressReporter for when progress tracking is disabled.
type NilReporter struct{}

// ReportProgress is a no-op for NilReporter.
func (NilReporter) ReportProgress(float64, string) {}

// ReportItemProgress is a no-op for NilReporter.
func (NilReporter) ReportItemProgress(int, int, string) {}

// Verify interface compliance at compile time.
var _ ProgressReporter = (*stageReporter)(nil)
var _ ProgressReporter = NilReporter{}

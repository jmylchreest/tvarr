package service

import (
	"context"
	"sync"

	"github.com/jmylchreest/tvarr/internal/models"
)

// ingestionCanceller tracks in-flight ingestions so a source edit can stop one.
//
// An ingestion loads its source once at the start and keeps using that copy, so
// editing a source mid-run has no effect on it: a run started against a dead
// provider keeps talking to the dead provider for as long as it takes to fail,
// however many times the URL is corrected in the meantime. Worse, when it
// finally ends it writes its stale copy back over the edit.
//
// Cancelling on update makes the edit mean what the user intended -- stop using
// the old settings -- and lets a fresh run pick the new ones up.
type ingestionCanceller struct {
	mu      sync.Mutex
	cancels map[models.ULID]*ingestionRun
}

// ingestionRun identifies one run, so a finishing run only ever deregisters
// itself and never a newer one that has replaced it.
type ingestionRun struct {
	cancel context.CancelFunc
}

// track registers an in-flight ingestion and returns a function that
// deregisters it, for the caller to defer.
//
// Any ingestion already running for the source is cancelled first: two runs
// writing the same rows is never what is wanted.
func (c *ingestionCanceller) track(id models.ULID, cancel context.CancelFunc) func() {
	run := &ingestionRun{cancel: cancel}

	c.mu.Lock()
	if c.cancels == nil {
		c.cancels = make(map[models.ULID]*ingestionRun)
	}
	if previous, running := c.cancels[id]; running {
		previous.cancel()
	}
	c.cancels[id] = run
	c.mu.Unlock()

	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		// Only our own entry: a later run may already have replaced it.
		if c.cancels[id] == run {
			delete(c.cancels, id)
		}
	}
}

// cancel stops the ingestion running for a source, reporting whether there was
// one.
func (c *ingestionCanceller) cancel(id models.ULID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	run, running := c.cancels[id]
	if !running {
		return false
	}
	run.cancel()
	delete(c.cancels, id)
	return true
}

// running reports whether an ingestion is in flight for a source.
func (c *ingestionCanceller) running(id models.ULID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.cancels[id]
	return ok
}

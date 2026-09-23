package service

import (
	"context"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
)

// TestIngestionCancellerCancelsInFlightRun covers the behaviour a source edit
// depends on: a run started against the old settings must actually stop.
func TestIngestionCancellerCancelsInFlightRun(t *testing.T) {
	var c ingestionCanceller
	id := models.NewULID()

	ctx, cancel := context.WithCancel(context.Background())
	untrack := c.track(id, cancel)
	defer untrack()

	if !c.running(id) {
		t.Fatal("a tracked run is not reported as running")
	}

	if !c.cancel(id) {
		t.Fatal("cancel() did not find the in-flight run")
	}

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the run's context was not cancelled; it would keep using the old settings")
	}

	if c.running(id) {
		t.Error("a cancelled run is still reported as running")
	}
}

func TestIngestionCancellerIgnoresUnknownSource(t *testing.T) {
	var c ingestionCanceller
	if c.cancel(models.NewULID()) {
		t.Error("cancel() claimed to stop a run that was never started")
	}
}

// TestIngestionCancellerStartingAgainStopsThePrevious guards against two runs
// writing the same rows at once.
func TestIngestionCancellerStartingAgainStopsThePrevious(t *testing.T) {
	var c ingestionCanceller
	id := models.NewULID()

	firstCtx, firstCancel := context.WithCancel(context.Background())
	defer c.track(id, firstCancel)()

	secondCtx, secondCancel := context.WithCancel(context.Background())
	defer c.track(id, secondCancel)()

	select {
	case <-firstCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("starting a second run did not stop the first")
	}

	if secondCtx.Err() != nil {
		t.Error("the second run was cancelled by its own registration")
	}
}

// TestIngestionCancellerUntrackLeavesNewerRunAlone: a slow run finishing must
// not deregister the run that replaced it, or that one becomes uncancellable.
func TestIngestionCancellerUntrackLeavesNewerRunAlone(t *testing.T) {
	var c ingestionCanceller
	id := models.NewULID()

	_, oldCancel := context.WithCancel(context.Background())
	untrackOld := c.track(id, oldCancel)

	newCtx, newCancel := context.WithCancel(context.Background())
	untrackNew := c.track(id, newCancel)
	defer untrackNew()

	// The superseded run finishes and tidies up after itself.
	untrackOld()

	if !c.running(id) {
		t.Fatal("the newer run was deregistered by an older one finishing")
	}
	if !c.cancel(id) {
		t.Fatal("the newer run could no longer be cancelled")
	}
	if newCtx.Err() == nil {
		t.Error("cancelling did not reach the newer run")
	}
}

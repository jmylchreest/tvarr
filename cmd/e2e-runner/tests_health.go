//nolint:errcheck,gocognit,gocyclo,nestif,gocritic,godot,wrapcheck,gosec,revive,goprintffuncname,modernize // E2E test runner uses relaxed linting
package main

import (
	"context"
)

// runHealthTests runs health check tests.
func (r *E2ERunner) runHealthTests(ctx context.Context) {
	r.runTestWithInfo("Health Check", "GET /health - verify API is responding", func() error {
		return r.client.HealthCheck(ctx)
	})
}

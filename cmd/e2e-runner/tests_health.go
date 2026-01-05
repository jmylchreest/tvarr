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

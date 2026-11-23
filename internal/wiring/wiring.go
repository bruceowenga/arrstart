// Package wiring defines the shared interface every app wiring client
// implements (PRD §10.5, §12.1): WaitHealthy → Configure → Verify.
// One package per app lives under internal/wiring/<app>. No Buildarr
// dependency; only the narrow set of API calls each app needs.
package wiring

import "context"

// Client is the interface every app wiring client must implement.
// Configure receives an app-specific config struct; the concrete type is
// documented on each implementation.
type Client interface {
	// WaitHealthy blocks until the app's API responds (or ctx is cancelled).
	// It does not authenticate — a server that answers with any HTTP status
	// counts as healthy; authentication is handled by Configure.
	WaitHealthy(ctx context.Context) error
	// Configure authenticates and applies the app-specific cfg.
	Configure(ctx context.Context, cfg any) error
	// Verify re-authenticates and checks that the expected state is present.
	Verify(ctx context.Context) error
}

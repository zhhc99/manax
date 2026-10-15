//go:build darwin

package daemon

import (
	"context"
	"os/signal"
	"syscall"
)

// Run starts the daemon in the foreground.
func Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	return New().Serve(ctx)
}

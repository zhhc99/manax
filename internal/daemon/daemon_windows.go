//go:build windows

package daemon

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"golang.org/x/sys/windows/svc"
)

// Run starts the daemon. When invoked by the Windows service manager it runs
// as a service, otherwise it runs in the foreground.
func Run() error {
	if err := bindProcessLifetime(); err != nil {
		return err
	}
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if isService {
		return svc.Run("manax", &service{})
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return New().Serve(ctx)
}

type service struct{}

func (s *service) Execute(_ []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}

	d := New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Serve(ctx) }()

	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for c := range r {
		switch c.Cmd {
		case svc.Stop, svc.Shutdown:
			status <- svc.Status{State: svc.StopPending}
			cancel()
			<-done
			return false, 0
		}
	}
	cancel()
	<-done
	fmt.Fprintln(os.Stderr, "manax service stopped")
	return false, 0
}

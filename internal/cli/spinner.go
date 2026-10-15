package cli

import (
	"fmt"
	"os"
	"time"
)

func withSpinner(message string, work func() error) error {
	if !isTerminal(os.Stderr) {
		fmt.Fprintln(os.Stderr, message)
		return work()
	}
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	stop, stopped := make(chan struct{}), make(chan struct{})
	show := func(frame int) {
		fmt.Fprintf(os.Stderr, "\r\x1b[2K%s %s", frames[frame], message)
	}
	show(0)
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		frame := 0
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				frame = (frame + 1) % len(frames)
				show(frame)
			}
		}
	}()
	err := work()
	close(stop)
	<-stopped
	fmt.Fprint(os.Stderr, "\r\x1b[2K")
	return err
}

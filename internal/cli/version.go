package cli

import (
	"context"
	"fmt"

	"github.com/zhhc99/manax/internal/ipc"
	"github.com/zhhc99/manax/internal/locale"
)

func runVersion() error {
	fmt.Printf(locale.T("msg.version")+"\n", Version)
	if resp, err := ipc.PostStatus(context.Background(), ipc.StatusRequest{}); err == nil {
		if resp.KernelRunning && resp.KernelVersion != "" {
			fmt.Printf(locale.T("msg.version_kernel")+"\n", resp.KernelVersion)
		}
	}
	return nil
}

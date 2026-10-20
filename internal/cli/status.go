package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/zhhc99/manax/internal/config"
	"github.com/zhhc99/manax/internal/core"
	"github.com/zhhc99/manax/internal/ipc"
	"github.com/zhhc99/manax/internal/locale"
)

func runStatus(_ []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st, _ := config.LoadState()
	if st == nil {
		st = &config.State{Active: config.DirectConfigName}
	}
	resp, err := ipc.PostStatus(ctx, ipc.StatusRequest{User: config.CurrentUser()})

	rows := [][2]string{}
	configName := st.Active
	if configName == "" {
		configName = config.DirectConfigName
	}

	if err != nil {
		rows = append(rows,
			[2]string{locale.T("label.service"), serviceLabel(locale.T("msg.service_stopped"))},
			[2]string{locale.T("label.task"), config.LoginTaskName},
			[2]string{locale.T("label.kernel"), locale.T("msg.kernel_stopped")},
			[2]string{locale.T("label.config"), configName},
			[2]string{locale.T("label.mode"), formatMode(st.EffectiveMode())},
			[2]string{locale.T("label.system_proxy"), locale.T("msg.proxy_off")},
			[2]string{locale.T("label.tun"), locale.T("msg.tun_off")},
		)
		printStatus(rows)
		fmt.Println(locale.T("msg.install_hint"))
		return nil
	}

	if resp.Node != "" && !core.IsBuiltin(resp.Node) && time.Since(resp.DelayUpdated) >= time.Minute {
		delay, err := ipc.PostDelay(ctx, ipc.DelayRequest{Name: resp.Node})
		resp.DelayKnown = true
		resp.DelayOK = err == nil && delay.Err == ""
		resp.DelayMS = delay.Delay
	}
	service := serviceLabel(locale.T("msg.service_running"))
	kernel := locale.T("msg.kernel_stopped")
	if resp.KernelRunning {
		if resp.KernelVersion != "" {
			kernel = fmt.Sprintf(locale.T("msg.kernel_running"), resp.KernelVersion)
		} else {
			kernel = locale.T("msg.service_running")
		}
	}
	configName = resp.ConfigName
	mode := resp.Mode
	proxy := locale.T("msg.proxy_off")
	if resp.SystemProxy {
		proxy = paint(fmt.Sprintf(locale.T("msg.proxy_on"), resp.ProxyAddr), colorSuccess)
	}
	tunOn := locale.T("msg.tun_off")
	if resp.TUN {
		tunOn = paint(locale.T("msg.tun_on"), colorSuccess)
	}

	rows = append(rows,
		[2]string{locale.T("label.service"), service},
		[2]string{locale.T("label.task"), config.LoginTaskName},
		[2]string{locale.T("label.kernel"), kernel},
		[2]string{locale.T("label.config"), configName},
		[2]string{locale.T("label.mode"), formatMode(mode)},
		[2]string{locale.T("label.system_proxy"), proxy},
		[2]string{locale.T("label.tun"), tunOn},
	)
	if resp.Node != "" {
		rows = append(rows, [2]string{locale.T("label.node"), formatNode(resp.Node, resp.DelayKnown, resp.DelayMS, resp.DelayOK)})
	}
	printStatus(rows)
	return nil
}

// serviceLabel prefixes a state with the concrete service name.
func serviceLabel(state string) string {
	if config.ServiceName == "" {
		return state
	}
	return config.ServiceName + " " + state
}

func formatNode(node string, known bool, ms int, ok bool) string {
	switch {
	case !known:
		return node
	case ok:
		role := colorSuccess
		if ms >= 1000 {
			role = colorWarning
		}
		return fmt.Sprintf("%s (%s)", node, paint(fmt.Sprintf(locale.T("msg.node_delay"), ms), role))
	default:
		return fmt.Sprintf("%s (%s)", node, paint(locale.T("msg.node_timeout"), colorError))
	}
}

func formatMode(mode string) string {
	switch mode {
	case "direct":
		return paint(mode, colorSuccess)
	case "rule":
		return paint(mode, colorWarning)
	case "global":
		return paint(mode, colorError)
	default:
		return mode
	}
}

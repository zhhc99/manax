package cli

import (
	"fmt"

	"github.com/zhhc99/manax/internal/config"
	"github.com/zhhc99/manax/internal/ipc"
	"github.com/zhhc99/manax/internal/locale"
)

func runMode(args []string) error {
	if len(args) == 0 {
		fmt.Println(subcommandHelp("mode"))
		return fmt.Errorf(locale.T("err.missing_arg"), "mode")
	}
	mode := args[0]
	switch mode {
	case "direct", "rule", "global":
	default:
		return fmt.Errorf(locale.T("err.invalid_mode"), mode)
	}
	resp, err := ipc.PostMode(ipc.ModeRequest{Mode: mode})
	if err != nil {
		return err
	}
	if resp.Err != "" {
		return fmt.Errorf(locale.T("err.daemon"), resp.Err)
	}
	st, _ := config.LoadState()
	st.Mode = mode
	if err := st.Save(); err != nil {
		return err
	}
	fmt.Printf(locale.T("msg.mode_set")+"\n", mode)
	return nil
}

func runSystem(args []string) error {
	on, err := parseOnOff(args, "system")
	if err != nil {
		return err
	}
	resp, err := ipc.PostSystem(ipc.SystemRequest{On: on, User: config.CurrentUser()})
	if err != nil {
		return err
	}
	if resp.Err != "" {
		return fmt.Errorf(locale.T("err.daemon"), resp.Err)
	}
	st, _ := config.LoadState()
	st.SystemProxy = on
	if err := st.Save(); err != nil {
		return err
	}
	if on {
		fmt.Println(locale.T("msg.system_enabled"))
	} else {
		fmt.Println(locale.T("msg.system_disabled"))
	}
	return nil
}

func runTun(args []string) error {
	on, err := parseOnOff(args, "tun")
	if err != nil {
		return err
	}
	resp, err := ipc.PostTun(ipc.TunRequest{On: on})
	if err != nil {
		return err
	}
	if resp.Err != "" {
		return fmt.Errorf(locale.T("err.daemon"), resp.Err)
	}
	st, _ := config.LoadState()
	st.TUN = on
	if err := st.Save(); err != nil {
		return err
	}
	if on {
		fmt.Println(locale.T("msg.tun_enabled"))
	} else {
		fmt.Println(locale.T("msg.tun_disabled"))
	}
	return nil
}

func parseOnOff(args []string, helpKey string) (bool, error) {
	if len(args) == 0 {
		fmt.Println(subcommandHelp(helpKey))
		return false, fmt.Errorf(locale.T("err.missing_arg"), "on|off")
	}
	switch args[0] {
	case "on":
		return true, nil
	case "off":
		return false, nil
	default:
		return false, fmt.Errorf(locale.T("err.invalid_arg"), args[0])
	}
}

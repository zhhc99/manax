package cli

import (
	"fmt"
	"os"

	"github.com/zhhc99/manax/internal/config"
	"github.com/zhhc99/manax/internal/core"
	"github.com/zhhc99/manax/internal/ipc"
	"github.com/zhhc99/manax/internal/locale"
)

func runReload(_ []string) error {
	st, err := config.LoadState()
	stateReadable := err == nil
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		st = &config.State{Configs: map[string]config.Source{}}
	}

	if st.Active != "" && st.Active != config.DirectConfigName {
		path, pathErr := config.ConfigFile(st.Active)
		if pathErr == nil {
			if _, statErr := os.Stat(path); statErr == nil {
				resp, applyErr := postApply(path, st.Active, st)
				if applyErr == nil && resp.Err == "" {
					if err := testAll(); err != nil {
						fmt.Fprintln(os.Stderr, err)
					}
					return runStatus(nil)
				}
				if applyErr != nil {
					fmt.Fprintf(os.Stderr, "%v\n", applyErr)
				} else {
					fmt.Fprintf(os.Stderr, "%s\n", resp.Err)
				}
			}
		}
	}

	path, err := config.DirectConfigFile()
	if err != nil {
		return err
	}
	if err := core.EnsureDirectConfig(path); err != nil {
		return err
	}
	resp, err := postApply(path, config.DirectConfigName, st)
	if err != nil {
		return err
	}
	if resp.Err != "" {
		return fmt.Errorf(locale.T("err.daemon"), resp.Err)
	}
	if st.Active != config.DirectConfigName {
		fmt.Fprintln(os.Stderr, locale.T("msg.reload_direct_fallback"))
		st.Active = config.DirectConfigName
		if stateReadable {
			if err := st.Save(); err != nil {
				return err
			}
		}
	}
	return runStatus(nil)
}

func postApply(path, name string, st *config.State) (ipc.ApplyResponse, error) {
	var resp ipc.ApplyResponse
	err := withSpinner(locale.T("msg.config_loading"), func() error {
		var err error
		resp, err = ipc.PostApply(ipc.ApplyRequest{
			ConfigPath:  path,
			ConfigName:  name,
			Mode:        st.EffectiveMode(),
			SystemProxy: st.SystemProxy,
			TUN:         st.TUN,
			User:        config.CurrentUser(),
		})
		return err
	})
	return resp, err
}

// runStop stops the kernel and the system proxy without touching the recorded
// system proxy or TUN switches, so a later reload restores them.
func runStop(_ []string) error {
	resp, err := ipc.PostStop(ipc.StopRequest{})
	if err != nil {
		return err
	}
	if resp.Err != "" {
		return fmt.Errorf(locale.T("err.daemon"), resp.Err)
	}
	fmt.Println(locale.T("msg.stopped"))
	return nil
}

// applyConfig loads a named config into the daemon and records it as active.
func applyConfig(name string) error {
	var path string
	var err error
	if name == config.DirectConfigName {
		path, err = config.DirectConfigFile()
		if err == nil {
			err = core.EnsureDirectConfig(path)
		}
	} else {
		path, err = config.ConfigFile(name)
	}
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf(locale.T("err.config_not_found"), name)
	}
	st, err := config.LoadState()
	if err != nil {
		return err
	}
	resp, err := postApply(path, name, st)
	if err != nil {
		return err
	}
	if resp.Err != "" {
		return fmt.Errorf(locale.T("err.daemon"), resp.Err)
	}
	st.Active = name
	return st.Save()
}

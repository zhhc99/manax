package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zhhc99/manax/internal/config"
	"github.com/zhhc99/manax/internal/locale"
)

func runConf(args []string) error {
	if len(args) == 0 {
		fmt.Println(subcommandHelp("conf"))
		return nil
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		return runConfAdd(rest)
	case "list":
		return runConfList(rest)
	case "use":
		return runConfUse(rest)
	case "update":
		return runConfUpdate(rest)
	case "rm":
		return runConfRm(rest)
	default:
		fmt.Fprintf(os.Stderr, locale.T("err.unknown_subcmd")+"\n", sub, "conf", "conf")
		return ErrSilent
	}
}

func runConfAdd(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf(locale.T("err.missing_arg"), "conf add <name> <url-or-file>")
	}
	name, location := args[0], args[1]
	if err := validateName(name); err != nil {
		return err
	}
	isURL := strings.HasPrefix(location, "http://") || strings.HasPrefix(location, "https://")

	path, err := config.ConfigFile(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if isURL {
		if err := downloadConfig(name, location, path); err != nil {
			return err
		}
	} else {
		abs, err := filepath.Abs(location)
		if err != nil {
			return err
		}
		if err := copyTo(abs, path); err != nil {
			return err
		}
		location = abs
	}

	st, _ := config.LoadState()
	st.Configs[name] = config.Source{Location: location, URL: isURL, Updated: config.NowStamp()}
	if err := st.Save(); err != nil {
		return err
	}
	fmt.Printf(locale.T("msg.config_added")+"\n", name)
	return nil
}

func runConfList(_ []string) error {
	st, _ := config.LoadState()
	names := map[string]bool{config.DirectConfigName: true}
	for name := range st.Configs {
		names[name] = true
	}
	if dir, err := config.ConfigsDir(); err == nil {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".yaml") && !strings.HasPrefix(e.Name(), ".") {
					names[strings.TrimSuffix(e.Name(), ".yaml")] = true
				}
			}
		}
	}
	if len(names) == 0 {
		fmt.Println(locale.T("msg.no_configs"))
		return nil
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	for _, name := range sorted {
		marker := " "
		if name == st.Active {
			marker = "*"
		}
		src := st.Configs[name]
		if name == config.DirectConfigName {
			src.Location = locale.T("msg.direct_config_source")
		}
		fmt.Printf("%s %-16s %s\n", marker, name, src.Location)
	}
	return nil
}

func runConfUse(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf(locale.T("err.missing_arg"), "conf use <name>")
	}
	name := args[0]
	if err := applyConfig(name); err != nil {
		return err
	}
	fmt.Printf(locale.T("msg.config_using")+"\n", name)
	return nil
}

func runConfUpdate(args []string) error {
	st, _ := config.LoadState()
	var names []string
	if len(args) > 0 {
		name := args[0]
		if _, ok := st.Configs[name]; !ok {
			return fmt.Errorf(locale.T("err.config_not_found"), name)
		}
		names = append(names, name)
	} else {
		for name := range st.Configs {
			names = append(names, name)
		}
		sort.Strings(names)
	}
	if len(names) == 0 {
		fmt.Println(locale.T("msg.no_configs"))
		return nil
	}

	activeUpdated := false
	for _, name := range names {
		src := st.Configs[name]
		if src.Location == "" {
			fmt.Printf(locale.T("msg.config_update_skip")+"\n", name)
			continue
		}
		path, err := config.ConfigFile(name)
		if err != nil {
			return err
		}
		var uerr error
		if src.URL {
			uerr = downloadConfig(name, src.Location, path)
		} else {
			uerr = copyTo(src.Location, path)
		}
		if uerr != nil {
			fmt.Fprintf(os.Stderr, "%v\n", uerr)
			continue
		}
		src.Updated = config.NowStamp()
		st.Configs[name] = src
		fmt.Printf(locale.T("msg.config_updated")+"\n", name)
		if name == st.Active {
			activeUpdated = true
		}
	}
	if err := st.Save(); err != nil {
		return err
	}
	if activeUpdated {
		return applyConfig(st.Active)
	}
	return nil
}

func runConfRm(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf(locale.T("err.missing_arg"), "conf rm <name>")
	}
	name := args[0]
	if name == config.DirectConfigName {
		return fmt.Errorf(locale.T("err.invalid_arg"), name)
	}
	st, _ := config.LoadState()
	delete(st.Configs, name)
	if path, err := config.ConfigFile(name); err == nil {
		_ = os.Remove(path)
	}
	wasActive := st.Active == name
	if wasActive {
		st.Active = ""
	}
	if err := st.Save(); err != nil {
		return err
	}
	fmt.Printf(locale.T("msg.config_removed")+"\n", name)
	if wasActive {
		return runReload(nil)
	}
	return nil
}

func validateName(name string) error {
	if name == "" || strings.EqualFold(name, config.DirectConfigName) || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return fmt.Errorf(locale.T("err.invalid_arg"), name)
	}
	return nil
}

func downloadConfig(name, url, dst string) error {
	return withSpinner(fmt.Sprintf(locale.T("msg.config_downloading"), name), func() error {
		return downloadTo(url, dst)
	})
}

func downloadTo(url, dst string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf(locale.T("err.download"), err)
	}
	// Airports may serve a base64 node list to generic clients and a full Clash
	// config, with real rules and groups, to a Clash client. Identify as one to
	// get the richer config; genuinely node-only subscriptions still fall back
	// to the wrapper in BuildRuntimeConfig.
	req.Header.Set("User-Agent", "Clash.Meta")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf(locale.T("err.download"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(locale.T("err.download"), fmt.Sprintf("http %d", resp.StatusCode))
	}
	f, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf(locale.T("err.write"), err)
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf(locale.T("err.download"), err)
	}
	return nil
}

func copyTo(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf(locale.T("err.write"), err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf(locale.T("err.write"), err)
	}
	return nil
}

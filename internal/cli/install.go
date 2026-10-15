package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/zhhc99/manax/internal/config"
	"github.com/zhhc99/manax/internal/ipc"
	"github.com/zhhc99/manax/internal/locale"
	"github.com/zhhc99/manax/internal/service"
)

// runInstall resolves resources before elevation, then installs the system
// service and registers the current user's login task.
func runInstall(args []string) error {
	// Elevated helper spawned by the parent. Resource downloads and prompts
	// have already completed in the unprivileged process.
	if len(args) > 0 && args[0] == "--system-phase" {
		if len(args) != 2 {
			return fmt.Errorf("%s", locale.T("err.install_phase_args"))
		}
		if !config.IsElevated() {
			return fmt.Errorf("%s", locale.T("err.need_elevate"))
		}
		self, err := os.Executable()
		if err != nil {
			return err
		}
		return installSystemPhase(self, args[1])
	}
	if len(args) != 0 {
		return fmt.Errorf(locale.T("err.invalid_arg"), args[0])
	}

	fmt.Fprintln(os.Stderr, locale.T("msg.installing"))
	self, err := os.Executable()
	if err != nil {
		return err
	}
	var bundle string
	var cleanup func()
	if config.IsElevated() {
		// Running as root by hand (sudo or a root shell). Warn before doing
		// anything, because this installs for root, not for this user.
		fmt.Fprintln(os.Stderr, locale.T("msg.root_warning"))
		if !confirmRoot() {
			fmt.Fprintln(os.Stderr, locale.T("msg.aborted"))
			return ErrSilent
		}
	}
	bundle, cleanup, err = promptInstallResources()
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}
	if bundle != "" {
		dir, err := config.ConfigsDir()
		if err != nil {
			return err
		}
		if err := service.InstallGeoDataPackage(bundle, dir); err != nil {
			return err
		}
		fmt.Printf(locale.T("msg.installed_geodata")+"\n", dir)
	}

	if config.IsElevated() {
		if err := installSystemPhase(self, bundle); err != nil {
			return err
		}
	} else {
		fmt.Fprintln(os.Stderr, locale.T("msg.elevating"))
		if err := service.Elevate(self, []string{"install", "--system-phase", bundle}); err != nil {
			if errors.Is(err, service.ErrNeedSudo) {
				return fmt.Errorf("%s", locale.T("msg.need_sudo"))
			}
			// The elevated helper already reported the failure.
			return ErrSilent
		}
	}

	user := config.CurrentUser()
	if err := service.InstallLogin(user); err != nil {
		return err
	}
	fmt.Printf(locale.T("msg.installed_login")+"\n", user.Username)
	deadline := time.Now().Add(5 * time.Second)
	for !ipc.IsRunning() && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	return runReload(nil)
}

func installSystemPhase(self, bundle string) error {
	if err := service.InstallSystem(self, bundle); err != nil {
		return err
	}
	fmt.Printf(locale.T("msg.installed_binary")+"\n", config.InstallBin)
	fmt.Printf(locale.T("msg.installed_kernel")+"\n", config.KernelPath)
	fmt.Printf(locale.T("msg.installed_service")+"\n", config.ServiceName)
	return nil
}

func promptInstallResources() (string, func(), error) {
	if !isTerminal(os.Stdin) {
		return "", nil, fmt.Errorf("%s", locale.T("err.resource_no_tty"))
	}
	r := bufio.NewReader(os.Stdin)
	geoDir, err := config.ConfigsDir()
	if err != nil {
		return "", nil, err
	}
	if service.HasInstallResources(geoDir) {
		fmt.Fprint(os.Stderr, locale.T("msg.install_resources_found"))
		switch strings.ToLower(readLine(r)) {
		case "", "y", "yes":
			return "", nil, nil
		case "n", "no":
		default:
			return "", nil, fmt.Errorf("%s", locale.T("err.resource_choice"))
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	checkErr := checkGitHubWithSpinner(ctx)
	stop()
	if checkErr != nil {
		if errors.Is(checkErr, context.Canceled) {
			return "", nil, ErrSilent
		}
		fmt.Fprintf(os.Stderr, locale.T("msg.github_unavailable")+"\n", checkErr)
		return promptResourcePackage(r)
	}

	fmt.Fprintln(os.Stderr, locale.T("msg.github_available"))
	fmt.Fprint(os.Stderr, locale.T("msg.github_download_prompt"))
	switch strings.ToLower(readLine(r)) {
	case "", "y", "yes":
		dir, err := os.MkdirTemp("", "manax-install-")
		if err != nil {
			return "", nil, err
		}
		bundle := filepath.Join(dir, "resources.zip")
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		err = service.BuildGitHubResourcePackage(ctx, bundle)
		stop()
		if err == nil {
			fmt.Fprintln(os.Stderr, locale.T("msg.resource_validating"))
			err = service.ValidateResourcePackage(bundle)
		}
		if err == nil {
			return bundle, func() { _ = os.RemoveAll(dir) }, nil
		}
		_ = os.RemoveAll(dir)
		if errors.Is(err, context.Canceled) {
			return "", nil, ErrSilent
		}
		fmt.Fprintf(os.Stderr, locale.T("msg.resource_fallback")+"\n", err)
		return promptResourcePackage(r)
	case "n", "no":
		return promptResourcePackage(r)
	default:
		return "", nil, fmt.Errorf("%s", locale.T("err.resource_choice"))
	}
}

func checkGitHubWithSpinner(ctx context.Context) error {
	return withSpinner(locale.T("msg.github_checking"), func() error {
		return service.CheckGitHubConnectivity(ctx)
	})
}

func promptResourcePackage(r *bufio.Reader) (string, func(), error) {
	fmt.Fprintln(os.Stderr, locale.T("msg.resource_package_prompt"))
	fmt.Fprint(os.Stderr, "> ")
	location := readLine(r)
	if location == "" {
		return "", nil, fmt.Errorf("%s", locale.T("err.resource_location"))
	}
	if strings.HasPrefix(location, "http://") || strings.HasPrefix(location, "https://") {
		dir, err := os.MkdirTemp("", "manax-install-")
		if err != nil {
			return "", nil, err
		}
		bundle := filepath.Join(dir, "resources.zip")
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		err = service.DownloadResourcePackage(ctx, location, bundle)
		stop()
		if err == nil {
			fmt.Fprintln(os.Stderr, locale.T("msg.resource_validating"))
			err = service.ValidateResourcePackage(bundle)
		}
		if err != nil {
			_ = os.RemoveAll(dir)
			if errors.Is(err, context.Canceled) {
				return "", nil, ErrSilent
			}
			return "", nil, err
		}
		return bundle, func() { _ = os.RemoveAll(dir) }, nil
	}
	path, err := filepath.Abs(location)
	if err != nil {
		return "", nil, err
	}
	fmt.Fprintln(os.Stderr, locale.T("msg.resource_validating"))
	if err := service.ValidateResourcePackage(path); err != nil {
		return "", nil, err
	}
	return path, func() {}, nil
}

func readLine(r *bufio.Reader) string {
	s, _ := r.ReadString('\n')
	return strings.TrimSpace(s)
}

// confirmRoot asks for an explicit y/N confirmation. The default is N.
func confirmRoot() bool {
	if !isTerminal(os.Stdin) {
		return false
	}
	fmt.Fprint(os.Stderr, locale.T("msg.root_confirm"))
	r := bufio.NewReader(os.Stdin)
	// Only the exact lowercase "y" is accepted.
	return readLine(r) == "y"
}

func runUninstall(_ []string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	if !config.IsElevated() {
		if err := service.Elevate(self, []string{"uninstall"}); err != nil {
			if errors.Is(err, service.ErrNeedSudo) {
				return fmt.Errorf("%s", locale.T("msg.need_sudo"))
			}
			return ErrSilent
		}
		if dir, err := config.UserDir(); err == nil {
			fmt.Printf(locale.T("msg.kept_config")+"\n", dir)
		}
		return nil
	}
	fmt.Println(locale.T("msg.uninstalling"))
	service.RemoveAllLogins()
	return service.UninstallSystem()
}

package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/zhhc99/manax/internal/daemon"
	"github.com/zhhc99/manax/internal/locale"
)

// Version is set via -ldflags at build time.
var Version = "dev"

// ErrSilent signals that a user-facing error was already printed.
var ErrSilent = errors.New("silent exit")

// Run dispatches a manax command.
func Run(args []string) error {
	if len(args) == 0 {
		fmt.Println(locale.T("help.root"))
		return nil
	}

	cmd, rest := args[0], args[1:]

	switch cmd {
	case "status":
		return runStatus(rest)
	case "reload":
		return runReload(rest)
	case "stop":
		return runStop(rest)
	case "install":
		return runInstall(rest)
	case "uninstall":
		return runUninstall(rest)
	case "mode":
		return runMode(rest)
	case "system":
		return runSystem(rest)
	case "tun":
		return runTun(rest)
	case "conf":
		return runConf(rest)
	case "node":
		return runNode(rest)
	case "version":
		return runVersion()
	case "help", "--help", "-h":
		if len(rest) > 0 {
			fmt.Println(helpFor(rest[0]))
		} else {
			fmt.Println(locale.T("help.root"))
		}
		return nil
	case "daemon":
		return daemon.Run()
	default:
		fmt.Fprintf(os.Stderr, locale.T("err.unknown_cmd")+"\n", cmd)
		return ErrSilent
	}
}

func helpFor(cmd string) string {
	switch cmd {
	case "status", "reload", "stop", "mode", "system", "tun", "conf", "node", "version":
		return subcommandHelp(cmd)
	}
	key := "help." + cmd
	s := locale.T(key)
	if s == key {
		return fmt.Sprintf(locale.T("err.unknown_cmd"), cmd)
	}
	return s
}

func subcommandHelp(cmd string) string {
	section := map[string][2]string{
		"status":  {"status ", "status "},
		"reload":  {"reload ", "reload "},
		"stop":    {"stop ", "stop "},
		"mode":    {"mode <direct|rule|global>", "mode <direct|rule|global>"},
		"system":  {"system <on|off>", "system <on|off>"},
		"tun":     {"tun <on|off>", "tun <on|off>"},
		"conf":    {"conf add ", "conf rm "},
		"node":    {"node list", "node use "},
		"version": {"version ", "version "},
	}
	bounds := section[cmd]
	first, last := bounds[0], bounds[1]
	lines := strings.Split(locale.T("help.root"), "\n")
	start, end := -1, -1
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if start < 0 && strings.HasPrefix(line, first) {
			start = i
		}
		if start >= 0 && strings.HasPrefix(line, last) {
			end = i
			break
		}
	}
	return locale.T("help."+cmd) + "\n\n" + strings.Join(lines[start:end+1], "\n")
}

// printStatus prints aligned label/value rows.
func printStatus(rows [][2]string) {
	for _, row := range rows {
		fmt.Printf("%s%s\n", padLabel(row[0]), row[1])
	}
}

func padLabel(s string) string {
	const target = 10
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	if w >= target {
		return s + " "
	}
	return s + strings.Repeat(" ", target-w)
}

func runeWidth(r rune) int {
	switch {
	case unicode.Is(unicode.Han, r), unicode.Is(unicode.Hangul, r),
		unicode.Is(unicode.Hiragana, r), unicode.Is(unicode.Katakana, r),
		r >= 0x1F000 && r <= 0x1FAFF,
		r >= 0x2600 && r <= 0x27BF,
		r >= 0x3000 && r <= 0x303F,
		r >= 0xFF01 && r <= 0xFF60,
		r >= 0xFFE0 && r <= 0xFFE6:
		return 2
	default:
		return 1
	}
}

// cellWidth is the terminal cell width of s, treating a regional-indicator pair
// (a flag) as one two-cell glyph and ignoring zero-width modifiers.
func cellWidth(s string) int {
	rs := []rune(s)
	w := 0
	for i := 0; i < len(rs); i++ {
		switch r := rs[i]; {
		case r == '\u200d' || r == '\ufe0e' || r == '\ufe0f':
		case isRegional(r) && i+1 < len(rs) && isRegional(rs[i+1]):
			w += 2
			i++
		default:
			w += runeWidth(r)
		}
	}
	return w
}

func isRegional(r rune) bool { return r >= 0x1F1E6 && r <= 0x1F1FF }

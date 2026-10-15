package cli

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zhhc99/manax/internal/ipc"
	"github.com/zhhc99/manax/internal/locale"
)

const (
	delayTimeout = -1
	delayError   = -2

	delayTimeoutMS = 5000
	delayWorkers   = 32
)

func runNode(args []string) error {
	if len(args) == 0 {
		fmt.Println(subcommandHelp("node"))
		return nil
	}
	switch args[0] {
	case "list":
		return runNodeList(args[1:])
	case "test":
		return runNodeTest(args[1:])
	case "use":
		return runNodeUse(args[1:])
	default:
		fmt.Fprintf(os.Stderr, locale.T("err.unknown_subcmd")+"\n", args[0], "node", "node")
		return ErrSilent
	}
}

// runNodeList prints the nodes using the daemon's last measured latencies,
// testing once if nothing has been measured yet.
func runNodeList(_ []string) error {
	_, nodes, delays, err := resolveNodes()
	if err != nil {
		return err
	}
	if !anyKnown(nodes, delays) {
		delays = testNodes(nodes)
	}
	printNodes(nodes, delays)
	return nil
}

func anyKnown(nodes []string, delays map[string]int) bool {
	for _, n := range nodes {
		if _, ok := delays[n]; ok {
			return true
		}
	}
	return false
}

// runNodeTest measures every node in parallel, then prints the list.
func runNodeTest(_ []string) error {
	_, nodes, _, err := resolveNodes()
	if err != nil {
		return err
	}
	printNodes(nodes, testNodes(nodes))
	return nil
}

// testAll measures the current nodes without printing the list.
func testAll() error {
	_, nodes, _, err := resolveNodes()
	if err != nil {
		return err
	}
	testNodes(nodes)
	return nil
}

func runNodeUse(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf(locale.T("err.missing_arg"), "node use <node>")
	}
	n, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf(locale.T("err.invalid_arg"), args[0])
	}
	g, nodes, _, err := resolveNodes()
	if err != nil {
		return err
	}
	if n < 1 || n > len(nodes) {
		return fmt.Errorf(locale.T("err.node_index"), n)
	}
	node := nodes[n-1]
	sel, err := ipc.PostSelect(ipc.SelectRequest{Group: g.Name, Node: node})
	if err != nil {
		return err
	}
	if sel.Err != "" {
		return fmt.Errorf(locale.T("err.daemon"), sel.Err)
	}
	fmt.Printf(locale.T("msg.node_selected")+"\n", node)
	return nil
}

// testNodes tests all nodes in parallel and returns name -> latency. A single
// status line is drawn on stderr while testing, so stdout stays clean.
func testNodes(nodes []string) map[string]int {
	results := make([]int, len(nodes))
	for i := range results {
		results[i] = delayError
	}

	var mu sync.Mutex
	done := 0
	tty := isTerminal(os.Stderr)
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	frame := 0
	stop, stopped := make(chan struct{}), make(chan struct{})
	if tty {
		go func() {
			defer close(stopped)
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					mu.Lock()
					frame++
					footer(frames[frame%len(frames)], done, len(nodes))
					mu.Unlock()
				}
			}
		}()
	}

	sem := make(chan struct{}, delayWorkers)
	var wg sync.WaitGroup
	for i, name := range nodes {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r, err := ipc.PostDelay(ipc.DelayRequest{Name: name, Timeout: delayTimeoutMS})
			mu.Lock()
			switch {
			case err != nil:
				results[i] = delayError
			case r.Err != "":
				results[i] = delayTimeout
			default:
				results[i] = r.Delay
			}
			done++
			mu.Unlock()
		}(i, name)
	}
	wg.Wait()

	if tty {
		close(stop)
		<-stopped
		footer("", 0, 0)
	}
	delays := make(map[string]int, len(nodes))
	for i, name := range nodes {
		delays[name] = results[i]
	}
	return delays
}

// footer redraws the single status line; an empty spinner clears it.
func footer(spinner string, done, total int) {
	if spinner == "" {
		fmt.Fprint(os.Stderr, "\r\x1b[2K")
		return
	}
	fmt.Fprintf(os.Stderr, "\r\x1b[2K%s %s %d/%d", spinner, locale.T("msg.node_testing"), done, total)
}

func printNodes(nodes []string, delays map[string]int) {
	nums := len(strconv.Itoa(len(nodes)))
	nameW := 0
	for _, n := range nodes {
		if w := cellWidth(n); w > nameW {
			nameW = w
		}
	}
	unknown := 0
	for i, name := range nodes {
		result, ok := delays[name]
		if !ok {
			unknown++
			fmt.Printf("%0*d. %s%s  %s\n", nums, i+1, name, strings.Repeat(" ", nameW-cellWidth(name)), locale.T("msg.node_unknown"))
			continue
		}
		fmt.Printf("%0*d. %s%s  %s\n", nums, i+1, name, strings.Repeat(" ", nameW-cellWidth(name)), formatResult(result))
	}
	if unknown > 0 && isTerminal(os.Stderr) {
		fmt.Fprintf(os.Stderr, "%s\n", locale.T("msg.node_hint"))
	}
}

// resolveNodes returns the primary group, its sorted nodes, and the daemon's
// cached latencies. The kernel's group concept is hidden from users, so there
// is exactly one group to consider.
func resolveNodes() (ipc.NodeGroup, []string, map[string]int, error) {
	resp, err := ipc.PostNodes()
	if err != nil {
		return ipc.NodeGroup{}, nil, nil, err
	}
	if resp.Err != "" {
		return ipc.NodeGroup{}, nil, nil, fmt.Errorf(locale.T("err.daemon"), resp.Err)
	}
	g, err := pickGroup(resp.Groups)
	if err != nil {
		return ipc.NodeGroup{}, nil, nil, err
	}
	nodes := groupNodes(g, resp.Groups)
	if len(nodes) == 0 {
		return ipc.NodeGroup{}, nil, nil, fmt.Errorf("%s", locale.T("err.no_nodes"))
	}
	return g, nodes, resp.Delays, nil
}

// pickGroup returns the primary selector, skipping the kernel's built-in
// GLOBAL group.
func pickGroup(groups []ipc.NodeGroup) (ipc.NodeGroup, error) {
	for _, g := range groups {
		if g.Type == "Selector" && g.Name != "GLOBAL" && len(g.Nodes) > 0 {
			return g, nil
		}
	}
	for _, g := range groups {
		if g.Name != "GLOBAL" && len(g.Nodes) > 0 {
			return g, nil
		}
	}
	return ipc.NodeGroup{}, fmt.Errorf("%s", locale.T("err.no_nodes"))
}

// groupNodes returns a group's real nodes, sorted. Group names and built-ins
// are excluded.
func groupNodes(g ipc.NodeGroup, groups []ipc.NodeGroup) []string {
	groupNames := map[string]bool{}
	for _, x := range groups {
		groupNames[x.Name] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, n := range g.Nodes {
		if groupNames[n] || isSpecial(n) || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func isSpecial(name string) bool {
	switch name {
	case "DIRECT", "REJECT", "GLOBAL", "COMPATIBLE", "PASS", "PASS-RULE":
		return true
	}
	return false
}

func formatResult(v int) string {
	switch v {
	case delayTimeout:
		return paint(locale.T("msg.node_timeout"), colorError)
	case delayError:
		return paint(locale.T("msg.node_error"), colorError)
	default:
		role := colorSuccess
		if v >= 1000 {
			role = colorWarning
		}
		return paint(fmt.Sprintf(locale.T("msg.node_delay"), v), role)
	}
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

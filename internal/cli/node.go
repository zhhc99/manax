package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zhhc99/manax/internal/core"
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
	nodes, delays, err := resolveNodes()
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
	nodes, _, err := resolveNodes()
	if err != nil {
		return err
	}
	printNodes(nodes, testNodes(nodes))
	return nil
}

// testAll measures the current nodes without printing the list.
func testAll() error {
	nodes, _, err := resolveNodes()
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
	nodes, _, err := resolveNodes()
	if err != nil {
		return err
	}
	if n < 1 || n > len(nodes) {
		return fmt.Errorf(locale.T("err.node_index"), n)
	}
	node := nodes[n-1]
	sel, err := ipc.PostSelect(ipc.SelectRequest{Node: node})
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
			r, err := ipc.PostDelay(context.Background(), ipc.DelayRequest{Name: name, Timeout: delayTimeoutMS})
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

// resolveNodes returns all actual nodes, deduplicated and sorted.
func resolveNodes() ([]string, map[string]int, error) {
	resp, err := ipc.PostNodes()
	if err != nil {
		return nil, nil, err
	}
	if resp.Err != "" {
		return nil, nil, fmt.Errorf(locale.T("err.daemon"), resp.Err)
	}
	seen := map[string]bool{}
	var nodes []string
	for _, node := range resp.Nodes {
		if !core.IsBuiltin(node) && !seen[node] {
			seen[node] = true
			nodes = append(nodes, node)
		}
	}
	if len(nodes) == 0 {
		return nil, nil, fmt.Errorf("%s", locale.T("err.no_nodes"))
	}
	sort.Strings(nodes)
	return nodes, resp.Delays, nil
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

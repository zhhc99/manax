package service

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/zhhc99/manax/internal/locale"
)

// progressBody renders file and per-file download progress on stderr.
type progressBody struct {
	r            io.Reader
	total        int64
	done         int64
	files        int
	completed    int
	currentFile  string
	fileComplete bool
	last         time.Time
	rendered     bool
}

func newProgress(files int) *progressBody {
	return &progressBody{files: files}
}

func (p *progressBody) startFile(name string) {
	p.currentFile = name
	p.total, p.done = 0, 0
	p.fileComplete = false
	p.last = time.Time{}
	p.render(true)
}

// attach binds the response body once the response headers have arrived.
func (p *progressBody) attach(r io.Reader, total int64) {
	p.r = r
	p.total = total
}

func (p *progressBody) completeFile() {
	p.completed++
	p.fileComplete = true
	p.render(true)
}

func (p *progressBody) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	p.render(false)
	return n, err
}

func (p *progressBody) render(force bool) {
	tty := isTTY(os.Stderr)
	now := time.Now()
	if !force && p.rendered && now.Sub(p.last) < 100*time.Millisecond {
		return
	}
	if !tty && !force {
		return
	}
	p.last = now
	p.rendered = true

	percent := 0
	if p.total > 0 {
		frac := float64(p.done) / float64(p.total)
		if frac > 1 {
			frac = 1
		}
		percent = int(frac * 100)
	}
	if p.fileComplete {
		percent = 100
	}
	const width = 20
	filled := percent * width / 100
	bar := strings.Repeat("=", filled)
	if filled < width {
		bar += ">" + strings.Repeat(" ", width-filled-1)
	}
	line := fmt.Sprintf(locale.T("msg.resource_progress"), bar, percent, p.completed, p.files, truncateProgressURL(p.currentFile, progressColumns()-35))
	if tty {
		fmt.Fprintf(os.Stderr, "\r\x1b[2K%s", line)
	} else {
		fmt.Fprintln(os.Stderr, line)
	}
}

func (p *progressBody) finish() {
	if p.rendered && isTTY(os.Stderr) {
		fmt.Fprintln(os.Stderr)
	}
}

func truncateProgressURL(url string, width int) string {
	if width < 4 {
		return "..."
	}
	if len(url) <= width {
		return url
	}
	left := (width - 3) / 2
	right := width - 3 - left
	return url[:left] + "..." + url[len(url)-right:]
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

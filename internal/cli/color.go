package cli

import "os"

type colorRole uint8

const (
	colorSuccess colorRole = iota
	colorWarning
	colorError
)

const colorReset = "\x1b[0m"

var semanticColors = [...]string{
	"\x1b[32m", // success
	"\x1b[33m", // warning
	"\x1b[31m", // error
}

func paint(text string, role colorRole) string {
	if text == "" || !colorEnabled() {
		return text
	}
	return semanticColors[role] + text + colorReset
}

func colorEnabled() bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok || os.Getenv("TERM") == "dumb" {
		return false
	}
	return isTerminal(os.Stdout)
}

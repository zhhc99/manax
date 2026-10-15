package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/zhhc99/manax/internal/cli"
	"github.com/zhhc99/manax/internal/locale"
)

func main() {
	locale.Init()
	if err := cli.Run(os.Args[1:]); err != nil {
		if !errors.Is(err, cli.ErrSilent) {
			fmt.Fprintln(os.Stderr, "Error:", err)
		}
		os.Exit(1)
	}
}

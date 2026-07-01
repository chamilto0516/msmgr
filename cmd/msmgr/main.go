package main

import (
	"fmt"
	"os"

	"msmgr/internal/cli"
)

func main() {
	app := cli.NewApp(os.Stdout)
	if err := app.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

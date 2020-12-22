package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/grafana/prometheus-pulsar-remote-write/pkg/app"
)

func main() {
	a := app.New(nil)
	err := a.Run(context.Background(), os.Args[1:]...)

	var runErr = &app.RunError{}
	if err != nil {
		// only disable non run errors
		if !errors.As(err, runErr) {
			fmt.Fprintln(os.Stderr, err)
		}

		// always exit 1 for errors
		os.Exit(1)
	}
}

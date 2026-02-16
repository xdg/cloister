// Package main is the entry point for the cloister CLI.
package main

import (
	"errors"
	"os"

	"github.com/xdg/cloister/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		var exitErr *cmd.ExitCodeError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		os.Exit(1)
	}
}

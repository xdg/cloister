package main

import (
	"os"

	"github.com/xdg/cloister/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

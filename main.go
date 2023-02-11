package main

import (
	"os"

	"github.com/Honyrik/opa-go-service/cmd"
)

func main() {
	if err := cmd.RootCommand.Execute(); err != nil {
		os.Exit(1)
	}
}

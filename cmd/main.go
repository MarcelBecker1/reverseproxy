package main

import (
	"log/slog"
	"os"

	"github.com/MarcelBecker1/reverseproxy/cmd/tests"
)

func main() {
	argsWithoutProg := os.Args[1:]
	if len(argsWithoutProg) > 0 && argsWithoutProg[0] == "simple" {
		slog.Info("Running simple test of components")
		tests.SimpleTest()
		return
	}

	slog.Info("Starting real simulation")
	tests.RunSimulation()
}

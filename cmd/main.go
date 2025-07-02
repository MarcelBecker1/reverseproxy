package main

import (
	"log/slog"
	"os"
)

func main() {
	argsWithoutProg := os.Args[1:]
	if len(argsWithoutProg) > 0 && argsWithoutProg[0] == "simple" {
		slog.Info("Running simple test of components")
		SimpleTest()
		return
	}

	slog.Info("Starting real simulation")
	RunSimulation()
}

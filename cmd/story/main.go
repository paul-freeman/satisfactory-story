package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/paul-freeman/satisfactory-story/state"
)

type simulator interface {
	// Tick advances the simulation by one tick.
	Tick() error
}

func main() {
	var verbose bool
	flag.BoolVar(&verbose, "v", false, "Enable verbose logging")
	flag.Parse()

	l := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	s, err := state.New(l, 11)
	if err != nil {
		panic(fmt.Errorf("failed to create state: %w", err))
	}
	_ = s
}

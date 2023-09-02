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

	l := makeLogger(verbose)

	seed := int64(52)

	s, err := state.New(l, seed)
	if err != nil {
		panic(fmt.Sprintf("failed to create state: %v", err))
	}
	for i := 0; i < 200000; i++ {
		err = s.Tick(l)
		if err != nil {
			panic(fmt.Sprintf("failed to tick state: %v", err))
		}
	}
	s.ListFactories(l)
}

func makeLogger(verbose bool) *slog.Logger {
	if verbose {
		return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: removeTimeAndLevel,
	}))
}

func removeTimeAndLevel(_ []string, a slog.Attr) slog.Attr {
	if a.Key == "time" {
		return slog.Attr{}
	}
	if a.Key == "level" {
		return slog.Attr{}
	}
	return a
}

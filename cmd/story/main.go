package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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

	// Create state
	l := makeLogger(verbose)
	seed := int64(52)
	s, err := state.New(l, seed)
	if err != nil {
		panic(fmt.Sprintf("failed to create state: %v", err))
	}

	// Run simulation
	go func() {
		for {
			err := s.Tick(l)
			if err != nil {
				panic(fmt.Sprintf("failed to tick state: %v", err))
			}
		}
	}()

	// Setup HTTP server
	port := ":28100"
	http.HandleFunc("/json", s.Serve(l))
	go func() {
		fmt.Printf("Server running on %s\n", port)
		if err := http.ListenAndServe(port, nil); err != nil {
			panic(fmt.Sprintf("failed to start HTTP server: %v", err))
		}
	}()

	// Listen for Ctrl+C
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

	<-c
	fmt.Println("\nReceived Ctrl+C, shutting down.")
	os.Exit(0)
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

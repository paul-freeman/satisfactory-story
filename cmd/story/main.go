package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/paul-freeman/satisfactory-story/state"
	"github.com/paul-freeman/satisfactory-story/state/http"
)

func main() {
	var verbose bool
	flag.BoolVar(&verbose, "v", false, "Enable verbose logging")
	flag.Parse()

	// Create state
	l := makeLogger(verbose)
	seed := int64(152)
	s, err := state.New(l, seed)
	if err != nil {
		panic(fmt.Sprintf("failed to create state: %v", err))
	}

	// Start HTTP server
	go http.Serve(s, ":28100", l)

	// Listen for Ctrl+C
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

	<-c
	fmt.Println("\nReceived Ctrl+C, shutting down.")
	s.ListFactories(l)
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

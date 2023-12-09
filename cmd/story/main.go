package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/paul-freeman/satisfactory-story/state"
	"github.com/paul-freeman/satisfactory-story/state/http"
)

func main() {
	// Create state
	logLevel := new(slog.Level)
	l := makeLogger(logLevel)
	seed := int64(152)
	s, err := state.New(l, logLevel, seed)
	if err != nil {
		panic(fmt.Sprintf("failed to create state: %v", err))
	}

	// Start HTTP server
	go http.Serve(s, ":28100", l, logLevel)

	// Listen for Ctrl+C
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

	<-c
	fmt.Println("\nReceived Ctrl+C, shutting down.")
	s.ListFactories(l)
	os.Exit(0)
}

func makeLogger(logLevel *slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:       logLevel,
		ReplaceAttr: removeTimeAndLevel,
	}))
}

func removeTimeAndLevel(_ []string, a slog.Attr) slog.Attr {
	if a.Key == "time" {
		return slog.Attr{}
	}
	return a
}

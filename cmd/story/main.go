package main

import (
	"fmt"

	"github.com/paul-freeman/satisfactory-story/state"
)

type simulator interface {
	// Tick advances the simulation by one tick.
	Tick() error
}

func main() {
	s, err := state.New()
	if err != nil {
		panic(fmt.Errorf("failed to create state: %w", err))
	}
	_ = s
}

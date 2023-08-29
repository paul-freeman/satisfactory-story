package main

import (
	"fmt"

	"github.com/paul-freeman/satisfactory-story/recipes"
	"github.com/paul-freeman/satisfactory-story/resources"
)

type producer interface{}

type specifier interface{}

type state struct {
	producers []producer
	specs     []specifier
}

func NewState() (*state, error) {
	producers, err := resources.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create producers: %w", err)
	}
	recipes, err := recipes.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create recipes: %w", err)
	}
	return &state{
		producers: []producer{producers},
		specs:     []specifier{recipes},
	}, nil
}

func main() {
	s, err := NewState()
	if err != nil {
		panic(err)
	}
	fmt.Println(s)
}

package state

import (
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_state_Tick(t *testing.T) {
	t.Run("all resources should be in a recipe", func(t *testing.T) {
		l := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level:       slog.LevelInfo,
			ReplaceAttr: removeTime,
		}))
		testState, err := New(l, 11)
		assert.NoError(t, err, "failed to create state")
		for _, producer := range testState.producers {
			for _, product := range producer.Products() {
				if product.Name == "sam" || product.Name == "geyser" {
					continue
				}
				// Check that the product is in at least one recipe
				found := false
				for _, recipe := range testState.recipes {
					if recipe.Inputs().Contains(product.Name) {
						found = true
						break
					}
				}
				if !found {
					t.Fail()
					// Look for something similar for debugging
					for _, recipe := range testState.recipes {
						for _, input := range recipe.Inputs() {
							if strings.Contains(strings.ToLower(input.Name), strings.ToLower(product.Name)) {
								t.Fatalf("product %s not in any recipe: found %s instead", product.Name, input)
							}
						}
					}
					t.Fatalf("product %s not in any recipe", product.Name)
				}
			}
		}
	})
	t.Run("can run one tick", func(t *testing.T) {
		l := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level:       slog.LevelInfo,
			ReplaceAttr: removeTime,
		}))
		seed := int64(52)

		testState, err := New(l, seed)
		assert.NoError(t, err, "failed to create state")
		err = testState.Tick(l)
		assert.NoError(t, err, "failed to tick state")
	})
	t.Run("can run multiple ticks", func(t *testing.T) {
		l := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level:       slog.LevelInfo,
			ReplaceAttr: removeTime,
		}))
		seed := int64(52)

		testState, err := New(l, seed)
		assert.NoError(t, err, "failed to create state")
		for i := 0; i < 100; i++ {
			err = testState.Tick(l)
			assert.NoError(t, err, "failed to tick state")
		}
	})
}

func removeTime(_ []string, a slog.Attr) slog.Attr {
	if a.Key == "time" {
		return slog.Attr{}
	}
	return a
}

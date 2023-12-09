package state

import (
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/stretchr/testify/assert"
)

func Test_state_Tick(t *testing.T) {
	t.Run("all resources should be in a recipe", func(t *testing.T) {
		l := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level:       slog.LevelInfo,
			ReplaceAttr: removeTimeAndLevel,
		}))
		logLevel := new(slog.Level)
		testState, err := New(l, logLevel, 11)
		assert.NoError(t, err, "failed to create state")
		assert.NotEqual(t, 0, testState.xmin, "xmin should not be 0")
		assert.NotEqual(t, 0, testState.xmax, "xmax should not be 0")
		assert.NotEqual(t, 0, testState.ymin, "ymin should not be 0")
		assert.NotEqual(t, 0, testState.ymax, "ymax should not be 0")
		for _, producer := range testState.producers {
			for _, product := range producer.Products() {
				// TODO: What do these products do?
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
								t.Fatalf("product %s not in any recipe: found %s instead", product.Name, input.Key())
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
			ReplaceAttr: removeTimeAndLevel,
		}))
		seed := int64(52)

		logLevel := new(slog.Level)
		testState, err := New(l, logLevel, seed)
		assert.NoError(t, err, "failed to create state")
		err = testState.Tick(l)
		assert.NoError(t, err, "failed to tick state")
	})
	t.Run("can run multiple ticks", func(t *testing.T) {
		l := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level:       slog.LevelInfo,
			ReplaceAttr: removeTimeAndLevel,
		}))
		seed := int64(52)

		logLevel := new(slog.Level)
		testState, err := New(l, logLevel, seed)
		assert.NoError(t, err, "failed to create state")
		for i := 0; i < 1000; i++ {
			err = testState.Tick(l)
			assert.NoError(t, err, "failed to tick state")
		}
		for _, producer := range testState.producers {
			f, ok := producer.(*factory.Factory)
			if ok {
				l.Info(f.String(), slog.Float64("profit", f.Profit()))
			}
		}
	})
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

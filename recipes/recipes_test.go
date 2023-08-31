package recipes

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "Parses data",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New()
			assert.NoError(t, err, "New() error = %v", err)
			for _, recipe := range got {
				fmt.Printf("\n%s\n%v\n%v\n%f\n", recipe.DisplayName, recipe.InputProducts, recipe.OutputProducts, recipe.Duration)
			}
		})
	}
}

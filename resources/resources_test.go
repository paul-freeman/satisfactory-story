package resources

import (
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
			nodes := make(map[string]int)
			for _, resource := range got {
				_, ok := nodes[string(resource.Production.Name)]
				if ok {
					nodes[string(resource.Production.Name)] = nodes[string(resource.Production.Name)] + 1
				} else {
					nodes[string(resource.Production.Name)] = 1
				}
			}
		})
	}
}

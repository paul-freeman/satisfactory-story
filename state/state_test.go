package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_state_Tick(t *testing.T) {
	t.Run("default producer and spec counts should not change", func(t *testing.T) {
		testState, err := New(11)
		assert.NoError(t, err, "failed to create state")
		wantNumProducers := len(testState.producers)
		wantNumSpecs := len(testState.specs)

		err = testState.Tick()
		assert.NoError(t, err, "failed to tick state")
		assert.Equal(t, wantNumProducers, len(testState.producers), "number of producers changed")
		assert.Equal(t, wantNumSpecs, len(testState.specs), "number of specs changed")
	})
}

package state

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_state_Tick(t *testing.T) {
	t.Run("run some ticks", func(t *testing.T) {
		testState, err := New(11)
		assert.NoError(t, err, "failed to create state")
		for i := 0; i < 100; i++ {
			err = testState.Tick()
			assert.NoError(t, err, "failed to tick state")
		}
	})
}

package production

import "testing"

func Test_Wallet(t *testing.T) {
	t.Run("starts with the seed balance", func(t *testing.T) {
		w := NewWallet(100)
		if w.Cash() != 100 {
			t.Errorf("got %f, want 100", w.Cash())
		}
	})

	t.Run("Apply adds and subtracts from the balance", func(t *testing.T) {
		w := NewWallet(100)
		w.Apply(-30)
		w.Apply(10)
		if w.Cash() != 80 {
			t.Errorf("got %f, want 80", w.Cash())
		}
	})

	t.Run("InsolventFor is false until the balance has been negative long enough", func(t *testing.T) {
		w := NewWallet(10)
		w.Apply(-20) // balance now -10, 1 tick negative
		if w.InsolventFor(2) {
			t.Errorf("should not be insolvent after only 1 negative tick")
		}
		w.Apply(0) // still -10, 2 ticks negative
		if !w.InsolventFor(2) {
			t.Errorf("should be insolvent after 2 negative ticks")
		}
	})

	t.Run("a positive tick resets the negative streak", func(t *testing.T) {
		w := NewWallet(10)
		w.Apply(-20) // -10, 1 negative tick
		w.Apply(50)  // back to positive, streak resets
		w.Apply(-60) // -10, 1 negative tick again
		if w.InsolventFor(2) {
			t.Errorf("streak should have reset after the positive tick")
		}
	})
}

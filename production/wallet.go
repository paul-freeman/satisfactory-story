package production

// Wallet tracks a producer's cash balance and how many consecutive ticks
// it has been negative, so callers can detect sustained insolvency
// instead of culling on a single bad tick.
type Wallet struct {
	Balance       float64
	negativeTicks int
}

// NewWallet returns a Wallet funded with the given starting balance.
func NewWallet(seed float64) Wallet {
	return Wallet{Balance: seed}
}

// Apply adds delta (positive or negative) to the balance and updates the
// consecutive-negative-tick counter used by InsolventFor.
func (w *Wallet) Apply(delta float64) {
	w.Balance += delta
	if w.Balance < 0 {
		w.negativeTicks++
	} else {
		w.negativeTicks = 0
	}
}

// Cash returns the current balance.
func (w *Wallet) Cash() float64 {
	return w.Balance
}

// InsolventFor reports whether the balance has been continuously negative
// for at least the given number of ticks.
func (w *Wallet) InsolventFor(ticks int) bool {
	return w.negativeTicks >= ticks
}

// Adjust moves money without touching the consecutive-negative-ticks
// counter. Trades use Adjust (many per tick); the solvency step's single
// Apply per tick is what advances insolvency accounting.
func (w *Wallet) Adjust(delta float64) {
	w.Balance += delta
}

package production

// Inventory holds real goods: float quantities per product name. It is
// the physical state of the inventory economy -- everything a producer
// can sell is in an Inventory, and everything it consumes comes out of
// one.
type Inventory map[string]float64

// Get returns the quantity on hand (0 for absent products).
func (inv Inventory) Get(name string) float64 {
	return inv[name]
}

// Add puts qty units into stock.
func (inv Inventory) Add(name string, qty float64) {
	inv[name] += qty
}

// Take removes up to qty units and returns how much was actually taken,
// clamped at what is available. Stock never goes negative.
func (inv Inventory) Take(name string, qty float64) float64 {
	have := inv[name]
	if qty > have {
		qty = have
	}
	if qty < 0 {
		qty = 0
	}
	inv[name] = have - qty
	return qty
}

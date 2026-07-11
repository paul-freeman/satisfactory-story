package production

type Production struct {
	Name string
	Rate float64
}

func New(name string, amount float64, duration float64) Production {
	rate := 0.0
	if duration != 0 {
		rate = amount / duration
	}
	return Production{
		Name: name,
		Rate: rate,
	}
}

func (p Production) Key() string {
	return p.Name
}

// DefaultUnitPrice seeds a producer's ask/bid price for a product the
// first time it is quoted, before market adjustment takes over.
const DefaultUnitPrice = 1.0

// MinUnitPrice is the lower bound for the ask price of a producer with no
// purchase costs (raw resource nodes) -- extraction is treated as nearly
// free, but a zero price would make price adjustment multiplicative
// against zero and stick there forever.
const MinUnitPrice = 0.01

// RateEpsilon is the smallest production rate treated as non-zero when
// publishing, matching, and inspecting orders.
const RateEpsilon = 1e-9

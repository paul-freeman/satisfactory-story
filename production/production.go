package production

import (
	"fmt"
)

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

func (p Production) String() string {
	return fmt.Sprintf("%s (%.2f)", p.Name, p.Rate)
}

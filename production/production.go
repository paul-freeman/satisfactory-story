package production

import (
	"fmt"
	"log/slog"
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

// Contract is a contract for a producer to produce a product.
//
// A pointer to the contract should be held by both the producer and the
// consumer. Either side may cancel the contract at any time, so the contract
// should be checked for cancellation regularly.
type Contract struct {
	// Seller is the seller of the product.
	Seller Producer
	// Buyer is the buyer of the product.
	Buyer Producer
	// Order is the rate of production of the product.
	Order Production
	// Price is the price of the product.
	Price float64
	// Cancelled is true if the contract has been cancelled.
	Cancelled bool
}

func NewContract(seller Producer, buyer Producer, order Production, price float64) *Contract {
	return &Contract{
		Seller: seller,
		Buyer:  buyer,
		Order:  order,
		Price:  price,
	}
}

// Cancel cancels the contract.
func (c *Contract) Cancel() {
	c.Cancelled = true
}

func SignContract(l *slog.Logger, seller Producer, buyer Producer, order Production, price float64) error {
	if err := seller.HasCapacityFor(order); err != nil {
		return fmt.Errorf("cannot sign contract: %w", err)
	}

	contract := NewContract(seller, buyer, order, price)
	if err := seller.AcceptSale(contract); err != nil {
		contract.Cancel()
		return fmt.Errorf("seller rejected contract: %w", err)
	}
	if err := buyer.AcceptPurchase(contract); err != nil {
		contract.Cancel()
		return fmt.Errorf("buyer rejected contract: %w", err)
	}

	return nil
}

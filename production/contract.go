package production

import (
	"fmt"
)

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

// Cancel cancels the contract.
func (c *Contract) Cancel() {
	c.Cancelled = true
}

func WriteContract(seller Producer, buyer Producer, order Production, price float64) error {
	if err := seller.HasCapacityFor(order); err != nil {
		return fmt.Errorf("cannot sign contract: %w", err)
	}

	contract := &Contract{
		Seller: seller,
		Buyer:  buyer,
		Order:  order,
		Price:  price,
	}
	if err := seller.SignAsSeller(contract); err != nil {
		contract.Cancel()
		return fmt.Errorf("seller rejected contract: %w", err)
	}
	if err := buyer.SignAsBuyer(contract); err != nil {
		contract.Cancel()
		return fmt.Errorf("buyer rejected contract: %w", err)
	}

	return nil
}

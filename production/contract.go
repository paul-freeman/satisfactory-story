package production

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
	// ProductCost is the price of the product.
	ProductCost float64
	// TransportCost is the price of transporting the product. Both the seller
	// and the buyer pay the transport cost.
	TransportCost float64
	// Cancelled is true if the contract has been cancelled.
	Cancelled bool
}

// Cancel cancels the contract.
func (c *Contract) Cancel() {
	c.Cancelled = true
}

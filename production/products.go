package production

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/paul-freeman/satisfactory-story/point"
)

// Producer is a type that can be used to produce a resource
type Producer interface {
	// Location returns the location of the producer.
	Location() point.Point
	// IsMovable returns true if the producer can be moved.
	IsMovable() bool
	// IsRemovable returns true if the producer can be removed.
	IsRemovable() bool
	// Products returns the products that the producer produces.
	Products() Products
	// Profit returns the profit of the producer.
	Profit() float64
	// HasCapacityFor returns true if the producer produces the given product at the
	// given rate.
	HasCapacityFor(Production) error
	// AcceptSale acknowledges that the producer will sell a product.
	AcceptSale(*Contract) error
	// AcceptPurchase acknowledges that the producer will purchase a product.
	AcceptPurchase(*Contract) error
}

type Products []Production

func (ps Products) String() string {
	strs := make([]string, len(ps))
	for i, p := range ps {
		strs[i] = p.String()
	}
	return fmt.Sprintf("[%s]", strings.Join(strs, ", "))
}

func (ps Products) Contains(name string) bool {
	for _, p := range ps {
		if p.Name == name {
			return true
		}
	}
	return false
}

func (ps *Products) UnmarshalJSON(b []byte) error {
	if ps == nil {
		return fmt.Errorf("cannot unmarshal into nil pointer")
	}
	rawString := string(b)
	rawString, err := trimQuoteAndParenthesis(rawString)
	if err != nil {
		return fmt.Errorf("failed to trim first quote and parenthesis: %w", err)
	}
	products := splitParenthesisGroups(rawString)
	*ps = make([]Production, 0, len(products))
	for _, p := range products {

		// Check for empty product
		if p == "" {
			return fmt.Errorf("empty product in %s", rawString)
		}

		// Trim parenthesis
		p, err = trimParenthesis(p)
		if err != nil {
			return fmt.Errorf("failed to trim second parenthesis: %w", err)
		}

		// Split nameStr from amount
		nameStr, amountStr, err := splitNameFromAmount(p)
		if err != nil {
			return fmt.Errorf("failed to split product name from amount: %w", err)
		}

		// Trim single quote, slash, and quote
		nameStr, err = trimSingleQuoteSlashAndQuote(nameStr)
		if err != nil {
			return fmt.Errorf("failed to trim single quote slash and quote: %w", err)
		}

		// Clean up product name
		nameStr, err = cleanUpName(nameStr)
		if err != nil {
			return fmt.Errorf("failed to clean up product name: %w", err)
		}

		// Parse product amount
		amount, err := strconv.ParseFloat(amountStr, 32)
		if err != nil {
			return fmt.Errorf("failed to parse product count: %w", err)
		}
		*ps = append(*ps, New(nameStr, amount, 0))
	}
	return nil
}

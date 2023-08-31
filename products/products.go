package products

import (
	"fmt"
	"strconv"
	"strings"
)

type Products []Product

func (ps Products) String() string {
	strs := make([]string, len(ps))
	for i, p := range ps {
		strs[i] = p.String()
	}
	return fmt.Sprintf("[%s]", strings.Join(strs, ", "))
}

func (ps Products) Contains(name string) bool {
	for _, p := range ps {
		if p.name == name {
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
	*ps = make([]Product, 0, len(products))
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
		*ps = append(*ps, Product{
			name:   nameStr,
			amount: amount,
		})
	}
	return nil
}

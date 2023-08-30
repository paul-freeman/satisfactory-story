package recipes

import (
	"fmt"
	"strconv"
	"strings"
)

type products []product

func (ps products) String() string {
	strs := make([]string, len(ps))
	for i, p := range ps {
		strs[i] = p.String()
	}
	return fmt.Sprintf("[%s]", strings.Join(strs, ", "))
}

type product struct {
	Name   string
	Amount int
}

func (p product) String() string {
	return fmt.Sprintf("%s (%d)", p.Name, p.Amount)
}

func (ps *products) UnmarshalJSON(b []byte) error {
	if ps == nil {
		return fmt.Errorf("cannot unmarshal into nil pointer")
	}
	rawString := string(b)
	rawString, err := trimQuoteAndParenthesis(rawString)
	if err != nil {
		return fmt.Errorf("failed to trim first quote and parenthesis: %w", err)
	}
	products := splitParenthesisGroups(rawString)
	*ps = make([]product, 0, len(products))
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

		// Split name from amount
		name, amountStr, err := splitNameFromAmount(p)
		if err != nil {
			return fmt.Errorf("failed to split product name from amount: %w", err)
		}

		// Trim single quote, slash, and quote
		name, err = trimSingleQuoteSlashAndQuote(name)
		if err != nil {
			return fmt.Errorf("failed to trim single quote slash and quote: %w", err)
		}

		// Clean up product name
		name, err = cleanUpName(name)
		if err != nil {
			return fmt.Errorf("failed to clean up product name: %w", err)
		}

		// Parse product amount
		amount, err := strconv.Atoi(amountStr)
		if err != nil {
			return fmt.Errorf("failed to parse product count: %w", err)
		}
		*ps = append(*ps, product{
			Name:   name,
			Amount: amount,
		})
	}
	return nil
}

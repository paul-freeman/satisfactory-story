package recipes

import (
	"fmt"
	"strconv"
	"strings"
)

type Ingredients []Ingredient

type Ingredient struct {
	Name   string
	Amount int
}

func (i *Ingredients) UnmarshalJSON(b []byte) error {
	if i == nil {
		return fmt.Errorf("cannot unmarshal into nil pointer")
	}
	rawString := string(b)
	rawString, err := trimQuoteAndParenthesis(rawString)
	if err != nil {
		return fmt.Errorf("failed to trim first quote and parenthesis: %w", err)
	}
	ingredients := splitParenthesisGroups(rawString)
	*i = make([]Ingredient, 0, len(ingredients))
	for _, ingredient := range ingredients {

		// Check for empty ingredient
		if ingredient == "" {
			return fmt.Errorf("empty ingredient in %s", rawString)
		}

		// Trim parenthesis
		ingredient, err = trimParenthesis(ingredient)
		if err != nil {
			return fmt.Errorf("failed to trim second parenthesis: %w", err)
		}

		// Split name from amount
		name, amountStr, err := splitNameFromAmount(ingredient)
		if err != nil {
			return fmt.Errorf("failed to split ingredient name from amount: %w", err)
		}

		// Trim single quote, slash, and quote
		name, err = trimSingleQuoteSlashAndQuote(name)
		if err != nil {
			return fmt.Errorf("failed to trim single quote slash and quote: %w", err)
		}

		// Clean up ingredient name
		name, err = cleanUpName(name)
		if err != nil {
			return fmt.Errorf("failed to clean up ingredient name: %w", err)
		}

		// Parse ingredient amount
		amount, err := strconv.Atoi(amountStr)
		if err != nil {
			return fmt.Errorf("failed to parse ingredient count: %w", err)
		}
		*i = append(*i, Ingredient{
			Name:   name,
			Amount: amount,
		})
	}
	return nil
}

func trimQuoteAndParenthesis(s string) (string, error) {
	if !strings.HasPrefix(s, "\"(") || !strings.HasSuffix(s, ")\"") {
		return "", fmt.Errorf("no outer \"()\" on value %q", s)
	}
	return s[2 : len(s)-2], nil
}

func trimParenthesis(s string) (string, error) {
	if !strings.HasPrefix(s, "(") || !strings.HasSuffix(s, ")") {
		return "", fmt.Errorf("missing match parenthesis in %s", s)
	}
	return s[1 : len(s)-1], nil
}

func trimSingleQuoteSlashAndQuote(s string) (string, error) {
	if !strings.HasPrefix(s, "'\\\"") || !strings.HasSuffix(s, "\\\"'") {
		return "", fmt.Errorf("missing single quote slash and quote in %s", s)
	}
	return s[3 : len(s)-3], nil
}

func splitNameFromAmount(s string) (string, string, error) {
	ss := strings.Split(s, ",")
	if len(ss) != 2 {
		return "", "", fmt.Errorf("too many commas in %s", s)
	}
	ingredient := strings.TrimPrefix(ss[0], "ItemClass=/Script/Engine.BlueprintGeneratedClass")
	amount := strings.TrimPrefix(ss[1], "Amount=")
	return ingredient, amount, nil
}

func splitParenthesisGroups(s string) []string {
	return re.FindAllString(s, -1)
}

func cleanUpName(s string) (string, error) {
	s = strings.TrimPrefix(s, "/Game/FactoryGame/Resource/Parts/")
	s = strings.TrimPrefix(s, "/Game/FactoryGame/Events/Christmas/")
	s = strings.TrimPrefix(s, "/Game/FactoryGame/Resource/RawResources/")
	s = strings.TrimPrefix(s, "/Game/FactoryGame/Resource/Equipment/")
	s = strings.TrimPrefix(s, "/Game/FactoryGame/Resource/Environment/")
	s = strings.TrimPrefix(s, "/Game/FactoryGame/Buildable/Factory/")
	s = strings.TrimPrefix(s, "/Game/FactoryGame/Buildable/Building/")
	s = strings.TrimPrefix(s, "/Game/FactoryGame/Buildable/Vehicle/")
	s = strings.TrimPrefix(s, "/Game/FactoryGame/Equipment/")
	s = strings.TrimPrefix(s, "/Game/FactoryGame/Prototype/Buildable/")
	if strings.HasPrefix(s, "/Game/FactoryGame") {
		return "", fmt.Errorf("unknown name: %s", s)
	}
	ss := strings.Split(s, ".")
	if len(ss) != 2 {
		return "", fmt.Errorf("too many dots: %s", s)
	}
	s = ss[1]
	s = strings.TrimPrefix(s, "Desc_")
	s = strings.TrimPrefix(s, "BP_")
	s = strings.TrimSuffix(s, "_C")
	if s == ss[1] {
		return "", fmt.Errorf("unknown name without known prefixes or suffixes: %s", s)
	}
	return s, nil
}

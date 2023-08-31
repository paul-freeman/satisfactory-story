package products

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Products []Product

func (ps Products) String(d float32) string {
	strs := make([]string, len(ps))
	for i, p := range ps {
		strs[i] = p.String(d)
	}
	return fmt.Sprintf("[%s]", strings.Join(strs, ", "))
}

func (ps Products) Contains(name Name) bool {
	for _, p := range ps {
		if p.Name == name {
			return true
		}
	}
	return false
}

type Product struct {
	Name   Name
	Amount float32
}

type Name string

const (
	Limestone Name = "Limestone"
	Iron      Name = "Iron"
	Copper    Name = "Copper"
	Caterium  Name = "Caterium"
	Coal      Name = "Coal"
	Oil       Name = "Oil"
	Sulfur    Name = "Sulfur"
	Bauxite   Name = "Bauxite"
	Quartz    Name = "Quartz"
	Uranium   Name = "Uranium"
	SAM       Name = "SAM"
	Geyser    Name = "Geyser"
)

func NameFromString(s string) (Name, error) {
	switch strings.ToLower(s) {
	case "limestone":
		return Limestone, nil
	case "iron":
		return Iron, nil
	case "copper":
		return Copper, nil
	case "caterium":
		return Caterium, nil
	case "coal":
		return Coal, nil
	case "oil":
		return Oil, nil
	case "sulfur":
		return Sulfur, nil
	case "bauxite":
		return Bauxite, nil
	case "quartz":
		return Quartz, nil
	case "uranium":
		return Uranium, nil
	case "sam":
		return SAM, nil
	case "geyser":
		return Geyser, nil
	default:
		return "", fmt.Errorf("invalid product name: %s", s)
	}
}

func (p Product) String(d float32) string {
	return fmt.Sprintf("%s (%.2f)", p.Name, p.Amount/d)
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
		name, err := NameFromString(nameStr)
		if err != nil {
			return fmt.Errorf("failed to parse product name: %w", err)
		}
		amount, err := strconv.ParseFloat(amountStr, 32)
		if err != nil {
			return fmt.Errorf("failed to parse product count: %w", err)
		}
		*ps = append(*ps, Product{
			Name:   name,
			Amount: float32(amount),
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

var re *regexp.Regexp

func init() {
	re = regexp.MustCompile(`\(([^)]+)\)`) // Match text between parentheses
}

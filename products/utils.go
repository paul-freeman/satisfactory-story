package products

import (
	"fmt"
	"regexp"
	"strings"
)

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

package products

import (
	"fmt"
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

type Product string

const (
	Limestone Product = "Limestone"
	Iron      Product = "Iron"
	Copper    Product = "Copper"
	Caterium  Product = "Caterium"
	Coal      Product = "Coal"
	Oil       Product = "Oil"
	Sulfur    Product = "Sulfur"
	Bauxite   Product = "Bauxite"
	Quartz    Product = "Quartz"
	Uranium   Product = "Uranium"
	SAM       Product = "SAM"
	Geyser    Product = "Geyser"
)

func FromString(s string) (Product, error) {
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

func (p Product) String() string {
	return string(p)
}

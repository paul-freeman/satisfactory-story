package resources

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

//go:embed Resource.json
var resourceJson []byte

type purity string

const (
	impure purity = "Impure"
	normal purity = "Normal"
	pure   purity = "Pure"
)

type resourceData struct {
	ID        string  `json:"id"`
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lng"`
}

type resource struct {
	Production production.Production
	Purity     purity
	Loc        point.Point
	sales      []*production.Contract
}

func New() ([]*resource, error) {
	var resourceData []resourceData
	if err := json.Unmarshal(resourceJson, &resourceData); err != nil {
		return nil, fmt.Errorf("failed to decode: %w", err)
	}

	resources := make([]*resource, len(resourceData))
	for i, data := range resourceData {
		var purity purity
		var rate float64
		var name string
		if strings.HasSuffix(data.ID, string(impure)) {
			name = strings.TrimSuffix(data.ID, string(impure))
			rate = 30.0
			purity = impure
		} else if strings.HasSuffix(data.ID, string(normal)) {
			name = strings.TrimSuffix(data.ID, string(normal))
			rate = 60.0
			purity = normal
		} else if strings.HasSuffix(data.ID, string(pure)) {
			name = strings.TrimSuffix(data.ID, string(pure))
			rate = 120.0
			purity = pure
		} else {
			return nil, fmt.Errorf("invalid resource ID: %s", data.ID)
		}
		name = toCanonicalName(name)
		resources[i] = &resource{
			Production: production.New(name, 1, rate/60.0),
			Purity:     purity,
			Loc:        point.Point{X: int(data.Longitude * 100), Y: int(data.Latitude * 100)},
		}
	}

	return resources, nil
}

func (r *resource) String() string {
	return fmt.Sprintf("Resource %s (%s) @ %s", r.Production.Name, r.Purity, r.Loc.String())
}

func (r *resource) Location() point.Point {
	return r.Loc
}

func (r *resource) IsMovable() bool {
	return false
}

func (r *resource) IsRemovable() bool {
	return false
}

func (r *resource) Products() production.Products {
	return production.Products{r.Production}
}

func (r resource) Profit() float64 {
	return 0.0
}

// AcceptsPurchase implements production.Producer.
func (r *resource) AcceptPurchase(_ *production.Contract) error {
	return fmt.Errorf("resource %s cannot make purchases", r.String())
}

// AcceptsSale implements production.Producer.
func (r *resource) AcceptSale(contract *production.Contract) error {
	r.sales = append(r.sales, contract)
	return nil
}

// HasCapacityFor implements production.Producer.
func (r *resource) HasCapacityFor(order production.Production) error {
	// Check current sales
	rate := r.Production.Rate
	for _, sale := range r.sales {
		if sale.Cancelled {
			continue
		}
		if sale.Order.Name != r.Production.Name || order.Name != r.Production.Name {
			continue
		}
		rate -= sale.Order.Rate
	}

	// Check new order
	if rate < order.Rate {
		return fmt.Errorf("resource %s cannot produce %s at rate %f", r.String(), order.String(), order.Rate)
	}
	return nil
}

var _ production.Producer = (*resource)(nil)

func toCanonicalName(name string) string {
	switch name {
	case "limestone":
		return "Stone"
	case "iron":
		return "OreIron"
	case "copper":
		return "OreCopper"
	case "caterium":
		return "OreGold"
	case "coal":
		return "Coal"
	case "oil":
		return "LiquidOil"
	case "sulfur":
		return "Sulfur"
	case "bauxite":
		return "OreBauxite"
	case "quartz":
		return "RawQuartz"
	case "uranium":
		return "OreUranium"
	default:
		return name
	}
}

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

type Resource struct {
	Production production.Production
	Purity     purity
	Loc        point.Point
	Sales      []*production.Contract
}

func New() ([]*Resource, error) {
	var resourceData []resourceData
	if err := json.Unmarshal(resourceJson, &resourceData); err != nil {
		return nil, fmt.Errorf("failed to decode: %w", err)
	}

	resources := make([]*Resource, len(resourceData))
	for i, data := range resourceData {
		var purity purity
		var amount float64
		var name string
		if strings.HasSuffix(data.ID, string(impure)) {
			name = strings.TrimSuffix(data.ID, string(impure))
			amount = 30.0
			purity = impure
		} else if strings.HasSuffix(data.ID, string(normal)) {
			name = strings.TrimSuffix(data.ID, string(normal))
			amount = 60.0
			purity = normal
		} else if strings.HasSuffix(data.ID, string(pure)) {
			name = strings.TrimSuffix(data.ID, string(pure))
			amount = 120.0
			purity = pure
		} else {
			return nil, fmt.Errorf("invalid resource ID: %s", data.ID)
		}
		name = toCanonicalName(name)
		const duration = 60.0 // 60 seconds
		x := int(data.Longitude * 1000)
		y := int(data.Latitude * 1000)
		resources[i] = &Resource{
			Production: production.New(name, 1, amount/duration),
			Purity:     purity,
			Loc:        point.Point{X: x, Y: y},
		}
	}

	return resources, nil
}

func (r *Resource) String() string {
	return fmt.Sprintf("Resource %s (%s) @ %s", r.Production.Name, r.Purity, r.Loc.String())
}

func (r *Resource) Location() point.Point {
	return r.Loc
}

func (r *Resource) IsMovable() bool {
	return false
}

func (r *Resource) IsRemovable() bool {
	return false
}

func (r *Resource) Remove() error {
	return fmt.Errorf("resource %s cannot be removed", r.String())
}

func (r *Resource) Products() production.Products {
	return production.Products{r.Production}
}

func (r Resource) Profit() float64 {
	profit := 0.0

	// Review sales
	newSales := make([]*production.Contract, 0)
	for _, sale := range r.Sales {
		if !sale.Cancelled {
			profit += sale.ProductCost
			profit -= sale.TransportCost
			newSales = append(newSales, sale)
		}
	}
	r.Sales = newSales

	return profit
}

func (r Resource) Profitability() float64 {
	income := 0.0
	expenses := 0.0
	for _, sale := range r.Sales {
		if !sale.Cancelled {
			income += sale.ProductCost
			expenses += sale.TransportCost
		}
	}
	return income / expenses
}

// SignAsBuyer implements production.Producer.
func (r *Resource) SignAsBuyer(_ *production.Contract) error {
	return fmt.Errorf("resource %s cannot make purchases", r.String())
}

// SignAsSeller implements production.Producer.
func (r *Resource) SignAsSeller(contract *production.Contract) error {
	r.Sales = append(r.Sales, contract)
	return nil
}

// SalesPriceFor computes the price of a sale.
//
// For resources, this is the transport cost plus 50%. This is very simple due
// to the fact that resources do not rely on purchasing other products.
func (r *Resource) SalesPriceFor(order production.Production, transportCost float64) float64 {
	return transportCost * 1.50 // 50% profit
}

// HasCapacityFor implements production.Producer.
func (r *Resource) HasCapacityFor(order production.Production) error {
	if order.Rate <= 0 {
		return fmt.Errorf("production rate must be positive")
	}

	// Check current sales
	rate := r.Production.Rate
	for _, sale := range r.Sales {
		if sale.Cancelled {
			continue
		}
		if sale.Order.Name != r.Production.Name || order.Name != r.Production.Name {
			continue
		}
		rate -= sale.Order.Rate
	}

	// Check new order
	if order.Name != r.Production.Name {
		return fmt.Errorf("resource %s cannot produce %s", r.String(), order.String())
	}
	if rate < order.Rate {
		return fmt.Errorf("resource %s cannot produce %s at rate %f", r.String(), order.String(), order.Rate)
	}
	return nil
}

// ContractsIn implements production.Producer.
func (r *Resource) ContractsIn() []*production.Contract {
	return []*production.Contract{}
}

// TryMove implements production.Producer.
func (r *Resource) TryMove() bool {
	// Resources cannot be moved
	return false
}

var _ production.Producer = (*Resource)(nil)

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

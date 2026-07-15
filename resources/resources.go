package resources

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
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
	// AskPrice is the persistent per-unit sale price for this node's
	// product, adjusted by the market loop. Zero means "not yet quoted";
	// it defaults on first use.
	AskPrice float64
	// Stock is the units of extracted product on hand, bounded by the
	// production step's cap. Asks are backed by this.
	Stock float64
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

// PrettyPrint returns a human-readable string representation of the resource.
func (r *Resource) PrettyPrint() string {
	return fmt.Sprintf("Resource %s (%s) @ %s", r.Production.Name, r.Purity, r.Loc.String())
}

// Location returns the location of the resource.
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
	return fmt.Errorf("resource %s cannot be removed", r.PrettyPrint())
}

func (r *Resource) Products() production.Products {
	return production.Products{r.Production}
}

// AskPriceFor returns the standing per-unit sale price for the named
// product (0 for a product this node does not produce), defaulting on
// first quote.
func (r *Resource) AskPriceFor(name string) float64 {
	if name != r.Production.Name {
		return 0
	}
	if r.AskPrice == 0 {
		r.AskPrice = production.DefaultUnitPrice
	}
	return r.AskPrice
}

// SetAskPrice records a new standing per-unit sale price.
func (r *Resource) SetAskPrice(name string, price float64) {
	if name != r.Production.Name {
		return
	}
	r.AskPrice = price
}

// ProduceTick extracts one tick's worth of product into stock, clamped
// at outputCapTicks worth of production.
func (r *Resource) ProduceTick(outputCapTicks float64) {
	cap := r.Production.Rate * outputCapTicks
	r.Stock = math.Min(cap, r.Stock+r.Production.Rate)
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

package resources

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/products"
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
	Product products.Product
	Purity  purity
	Loc     point.Point
	profit  float64
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
		resources[i] = &resource{
			Product: products.New(name, 1, rate/60.0),
			Purity:  purity,
			Loc:     point.Point{X: int(data.Longitude * 100), Y: int(data.Latitude * 100)},
		}
	}

	return resources, nil
}

func (r *resource) Name() string {
	return string(r.Product.Name())
}

func (r *resource) String() string {
	return fmt.Sprintf("%s (%s) @ %s", r.Name(), r.Purity, r.Loc.String())
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

func (r *resource) Products() products.Products {
	return products.Products{r.Product}
}

func (r resource) Profit() float64 {
	return 0.0
}

func (r *resource) HasProduct(p products.Product) bool {
	return r.Product == p
}

func (r *resource) AddProfits(p float64) {
	r.profit += p
}

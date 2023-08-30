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

type name string

const (
	limestone name = "Limestone"
	iron      name = "Iron"
	copper    name = "Copper"
	caterium  name = "Caterium"
	coal      name = "Coal"
	oil       name = "Oil"
	sulfur    name = "Sulfur"
	bauxite   name = "Bauxite"
	quatrz    name = "Quartz"
	uranium   name = "Uranium"
	sam       name = "SAM"
	geyser    name = "Geyser"
)

func new(s string) (name, error) {
	switch strings.ToLower(s) {
	case "limestone":
		return limestone, nil
	case "iron":
		return iron, nil
	case "copper":
		return copper, nil
	case "caterium":
		return caterium, nil
	case "coal":
		return coal, nil
	case "oil":
		return oil, nil
	case "sulfur":
		return sulfur, nil
	case "bauxite":
		return bauxite, nil
	case "quartz":
		return quatrz, nil
	case "uranium":
		return uranium, nil
	case "sam":
		return sam, nil
	case "geyser":
		return geyser, nil
	default:
		return "", fmt.Errorf("invalid resource name: %s", s)
	}
}

func (n name) String() string {
	return string(n)
}

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
	Type   products.Product
	Purity purity
	Loc    point.Point
}

func New() ([]resource, error) {
	var resourceData []resourceData
	if err := json.Unmarshal(resourceJson, &resourceData); err != nil {
		return nil, fmt.Errorf("failed to decode: %w", err)
	}

	resources := make([]resource, len(resourceData))
	for i, data := range resourceData {
		var purity purity
		var s string
		if strings.HasSuffix(data.ID, string(impure)) {
			s = strings.TrimSuffix(data.ID, string(impure))
			purity = impure
		} else if strings.HasSuffix(data.ID, string(normal)) {
			s = strings.TrimSuffix(data.ID, string(normal))
			purity = normal
		} else if strings.HasSuffix(data.ID, string(pure)) {
			s = strings.TrimSuffix(data.ID, string(pure))
			purity = pure
		} else {
			return nil, fmt.Errorf("invalid resource ID: %s", data.ID)
		}
		t, err := products.FromString(s)
		if err != nil {
			return nil, fmt.Errorf("failed to create resource: %w", err)
		}
		resources[i] = resource{
			Type:   t,
			Purity: purity,
			Loc:    point.Point{X: int(data.Longitude * 100), Y: int(data.Latitude * 100)},
		}
	}

	return resources, nil
}

func (r resource) Name() string {
	return string(r.Type)
}

func (r resource) String() string {
	return fmt.Sprintf("%s (%s) @ %s", r.Name(), r.Purity, r.Loc.String())
}

func (r resource) Location() (loc point.Point, err error) {
	return r.Loc, nil
}

func (r resource) IsMovable() bool {
	return false
}

func (r resource) IsRemovable() bool {
	return false
}

func (r resource) Products() products.Products {
	return products.Products{r.Type}
}

func (r resource) Profit() float64 {
	return 0
}

package recipes

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

//go:embed Docs.json
var docsJson []byte

const (
	fgRecipe = "/Script/CoreUObject.Class'/Script/FactoryGame.FGRecipe'"
)

func New() (Recipes, error) {
	utf16Transformer := unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder()

	var rs Recipes
	r := bytes.NewReader(docsJson)
	if err := json.NewDecoder(transform.NewReader(r, utf16Transformer)).Decode(&rs); err != nil {
		return nil, fmt.Errorf("failed to decode: %w", err)
	}

	return rs, nil
}

type Recipes []Recipe

type Recipe struct {
	DisplayName    string              `json:"mDisplayName"`
	ProducedIn     Producer            `json:"mProducedIn"`
	InputProducts  production.Products `json:"mIngredients"`
	OutputProducts production.Products `json:"mProduct"`
	DurationStr    floatString         `json:"mManufactoringDuration"`
}

type producerCost struct {
	p    production.Producer
	cost float64
}

func (r Recipe) SourceProducts(sellers []production.Producer, location point.Point) (map[string]producerCost, error) {
	sourcedProducts := make(map[string]producerCost)
	for _, product := range r.Inputs() {
		// Find producers that produce the input product
		var bestProducer production.Producer
		var bestCost float64
		for _, p := range sellers {
			err := p.HasCapacityFor(product)
			if err != nil {
				continue
			}
			if bestProducer == nil {
				bestProducer = p
			} else {
				cost := costFunction(p, location, product)
				if cost < bestCost {
					bestProducer = p
					bestCost = cost
				}
			}
		}
		if bestProducer == nil {
			return nil, fmt.Errorf("failed to find producer for input %s", product.Name)
		}
		sourcedProducts[product.Name] = producerCost{
			p:    bestProducer,
			cost: bestCost,
		}
	}

	// Check that all products are available
	if len(sourcedProducts) != len(r.Inputs()) {
		return nil, fmt.Errorf("failed to find all inputs for %s", r.String())
	}

	return sourcedProducts, nil
}

func (r Recipe) Name() string {
	return r.DisplayName
}

func (r Recipe) String() string {
	return fmt.Sprintf(
		"%s (%s) %s => %s",
		r.Name(),
		r.ProducedIn.String(),
		r.InputProducts.String(),
		r.OutputProducts.String(),
	)
}

func (r Recipe) Inputs() production.Products {
	return r.InputProducts
}

func (r Recipe) Outputs() production.Products {
	return r.OutputProducts
}

func (r Recipe) Duration() float64 {
	return float64(r.DurationStr)
}

func (rs *Recipes) UnmarshalJSON(b []byte) error {
	if rs == nil {
		return fmt.Errorf("cannot unmarshal into nil pointer")
	}

	docs := make([]struct {
		NativeClass string          `json:"NativeClass"`
		Classes     json.RawMessage `json:"Classes"`
	}, 0, 1000)
	if err := json.Unmarshal(b, &docs); err != nil {
		return fmt.Errorf("failed to unmarshal docs: %w", err)
	}

	for _, doc := range docs {
		if doc.NativeClass == fgRecipe {
			tmpRecipes := make([]json.RawMessage, 0)
			if err := json.Unmarshal(doc.Classes, &tmpRecipes); err != nil {
				return fmt.Errorf("failed to unmarshal recipes: %w", err)
			}
			for _, tmpRecipe := range tmpRecipes {
				var r Recipe
				if err := json.Unmarshal(tmpRecipe, &r); err != nil {
					return fmt.Errorf("failed to unmarshal recipe: %w", err)
				}
				if r.ProducedIn == NullProducer {
					continue
				} else if r.ProducedIn == BuildGun {
					continue
				} else if r.ProducedIn == Workshop {
					continue
				}
				*rs = append(*rs, r)
			}
		}
	}
	return nil
}

type floatString float64

func (f *floatString) UnmarshalJSON(b []byte) error {
	if f == nil {
		return fmt.Errorf("cannot unmarshal into nil pointer")
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("failed to unmarshal float string: %w", err)
	}
	fl, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("failed to parse float string: %w", err)
	}
	*f = floatString(fl)
	return nil
}

// costFunction returns the cost of transporting the given product from the
// given producer to the given location.
func costFunction(p production.Producer, loc point.Point, product production.Production) float64 {
	return loc.Distance(p.Location())
}

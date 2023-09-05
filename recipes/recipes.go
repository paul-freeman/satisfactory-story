package recipes

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
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
	DisplayName    string
	ProducedIn     Producer
	InputProducts  production.Products
	OutputProducts production.Products
}

type recipeJSON struct {
	DisplayName    string              `json:"mDisplayName"`
	ProducedIn     Producer            `json:"mProducedIn"`
	InputProducts  production.Products `json:"mIngredients"`
	OutputProducts production.Products `json:"mProduct"`
	DurationStr    floatString         `json:"mManufactoringDuration"`
}

func (j recipeJSON) toRecipe() Recipe {
	r := Recipe{
		DisplayName:    j.DisplayName,
		ProducedIn:     j.ProducedIn,
		InputProducts:  j.InputProducts,
		OutputProducts: j.OutputProducts,
	}
	// TODO: We update the rate here. We originally set the rate the amount
	// produced, which is not correct. But now that we know the duration, we can
	// set the rate correctly.
	for i := range r.InputProducts {
		r.InputProducts[i].Rate /= float64(j.DurationStr)
	}
	for i := range r.OutputProducts {
		r.OutputProducts[i].Rate /= float64(j.DurationStr)
	}
	return r
}

type Source struct {
	Order         production.Production
	Seller        production.Producer
	TransportCost float64
}

func (r Recipe) SourceProducts(l *slog.Logger, sellers []production.Producer, destination point.Point) (map[string]Source, error) {
	sourcedProducts := make(map[string]Source)
	for _, order := range r.Inputs() {
		// Find producers that produce the input product
		var bestProducer production.Producer
		var bestCost float64
		for _, seller := range sellers {
			err := seller.HasCapacityFor(order)
			if err != nil {
				continue
			}
			origin := seller.Location()
			cost := TransportCost(origin, destination)
			if bestProducer == nil {
				bestProducer = seller
				bestCost = TransportCost(seller.Location(), destination)
			} else if cost < bestCost {
				bestProducer = seller
				bestCost = cost
			}
		}
		if bestProducer == nil {
			l.Info("failed to find producer for input", slog.String("input", order.Name))
			return nil, fmt.Errorf("failed to find producer for input %s", order.Name)
		}
		sourcedProducts[order.Name] = Source{
			Order:         order,
			Seller:        bestProducer,
			TransportCost: bestCost,
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
		r.InputProducts.Key(),
		r.OutputProducts.Key(),
	)
}

func (r Recipe) Inputs() production.Products {
	return r.InputProducts
}

func (r Recipe) Outputs() production.Products {
	return r.OutputProducts
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
				var jsonRecipe recipeJSON
				if err := json.Unmarshal(tmpRecipe, &jsonRecipe); err != nil {
					return fmt.Errorf("failed to unmarshal recipe: %w", err)
				}
				if jsonRecipe.ProducedIn == NullProducer {
					continue
				} else if jsonRecipe.ProducedIn == BuildGun {
					continue
				} else if jsonRecipe.ProducedIn == Workshop {
					continue
				}
				*rs = append(*rs, jsonRecipe.toRecipe())
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

// TransportCost returns the cost of transporting the given product from the
// given producer to the given location.
func TransportCost(origin point.Point, destination point.Point) float64 {
	return 1 + origin.Distance(destination)/10000.0
}

package recipes

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"github.com/paul-freeman/satisfactory-story/products"
)

//go:embed Docs.json
var docsJson []byte

const (
	fgRecipe = "/Script/CoreUObject.Class'/Script/FactoryGame.FGRecipe'"
)

func New() (recipes, error) {
	utf16Transformer := unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder()

	var rs recipes
	r := bytes.NewReader(docsJson)
	if err := json.NewDecoder(transform.NewReader(r, utf16Transformer)).Decode(&rs); err != nil {
		return nil, fmt.Errorf("failed to decode: %w", err)
	}

	return rs, nil
}

type recipes []recipe

type recipe struct {
	DisplayName    string            `json:"mDisplayName"`
	ProducedIn     Producer          `json:"mProducedIn"`
	InputProducts  products.Products `json:"mIngredients"`
	OutputProducts products.Products `json:"mProduct"`
	DurationStr    floatString       `json:"mManufactoringDuration"`
}

func (r recipe) Name() string {
	return r.DisplayName
}

func (r recipe) String() string {
	return fmt.Sprintf(
		"%s (%s) %s => %s",
		r.Name(),
		r.ProducedIn.String(),
		r.InputProducts.String(),
		r.OutputProducts.String(),
	)
}

func (r recipe) Inputs() products.Products {
	return r.InputProducts
}

func (r recipe) Outputs() products.Products {
	return r.OutputProducts
}

func (r recipe) Duration() float64 {
	return float64(r.DurationStr)
}

func (rs *recipes) UnmarshalJSON(b []byte) error {
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
				var r recipe
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

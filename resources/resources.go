package resources

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed Resource.json
var resourceJson []byte

type name string

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
	Name      name    `json:"name"`
	Purity    purity  `json:"purity"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func New() ([]resource, error) {
	var resourceData []resourceData
	if err := json.Unmarshal(resourceJson, &resourceData); err != nil {
		return nil, fmt.Errorf("failed to decode: %w", err)
	}

	resources := make([]resource, len(resourceData))
	for i, data := range resourceData {
		var purity purity
		var n name
		if strings.HasSuffix(data.ID, string(impure)) {
			n = name(strings.TrimSuffix(data.ID, string(impure)))
			purity = impure
		} else if strings.HasSuffix(data.ID, string(normal)) {
			n = name(strings.TrimSuffix(data.ID, string(normal)))
			purity = normal
		} else if strings.HasSuffix(data.ID, string(pure)) {
			n = name(strings.TrimSuffix(data.ID, string(pure)))
			purity = pure
		} else {
			return nil, fmt.Errorf("invalid resource ID: %s", data.ID)
		}
		resources[i] = resource{
			Name:      n,
			Purity:    purity,
			Latitude:  data.Latitude,
			Longitude: data.Longitude,
		}
	}

	return resources, nil
}

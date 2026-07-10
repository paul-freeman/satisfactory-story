package recipes

import (
	"fmt"
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

// Source describes where and at what transport cost a product can be
// bought.
type Source struct {
	Order         production.Production
	Seller        production.Producer
	TransportCost float64
}

// FindBestSeller returns the seller from sellers with spare capacity for
// order that is cheapest to transport to destination. Returns false if no
// seller currently has the capacity.
func FindBestSeller(sellers []production.Producer, order production.Production, destination point.Point) (Source, bool) {
	var bestProducer production.Producer
	var bestCost float64
	for _, seller := range sellers {
		if err := seller.HasCapacityFor(order); err != nil {
			continue
		}
		cost := TransportCost(seller.Location(), destination)
		if bestProducer == nil || cost < bestCost {
			bestProducer = seller
			bestCost = cost
		}
	}
	if bestProducer == nil {
		return Source{}, false
	}
	return Source{Order: order, Seller: bestProducer, TransportCost: bestCost}, true
}

// SourceProducts finds the cheapest available seller (with spare capacity)
// for each of the recipe's inputs. It always evaluates every input --
// unmet lists any inputs that could not be sourced, so callers can record
// them as shortages even when the recipe as a whole can't be built yet.
// err is non-nil exactly when unmet is non-empty.
func (r Recipe) SourceProducts(l *slog.Logger, sellers []production.Producer, destination point.Point) (map[string]Source, []production.Production, error) {
	sourced := make(map[string]Source)
	unmet := make([]production.Production, 0)
	for _, order := range r.Inputs() {
		source, found := FindBestSeller(sellers, order, destination)
		if !found {
			l.Debug("failed to find producer for input", slog.String("input", order.Name))
			unmet = append(unmet, order)
			continue
		}
		sourced[order.Name] = source
	}
	if len(unmet) > 0 {
		return sourced, unmet, fmt.Errorf("failed to find %d input(s) for %s", len(unmet), r.String())
	}
	return sourced, unmet, nil
}

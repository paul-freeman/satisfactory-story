package state

import (
	"log/slog"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/resources"
)

// outputStockCapTicks bounds every output buffer at this many ticks of
// production. When a buffer is full, production halts and input buying
// stops -- the back-pressure signal. A milestone tuning knob.
const outputStockCapTicks = 60.0

// inputStockTargetTicks is how many ticks of consumption a factory
// tries to keep on hand; the gap to it is the bid quantity (hunger).
// A milestone tuning knob.
const inputStockTargetTicks = 60.0

// produceGoods runs one tick of physical production for every producer:
// resources extract into stock, factories run their recipes against
// stock. Runs before the market so fresh goods are sellable this tick.
func (s *State) produceGoods(_ *slog.Logger) {
	for _, p := range s.producers {
		switch producer := p.(type) {
		case *resources.Resource:
			producer.ProduceTick(outputStockCapTicks)
		case *factory.Factory:
			producer.ProduceTick(outputStockCapTicks)
		}
	}
}

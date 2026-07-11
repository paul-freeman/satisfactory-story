package market

import (
	"errors"
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
)

// flatTransport makes delivered cost independent of geometry so price
// assertions are exact.
func flatTransport(_, _ point.Point) float64 { return 1.0 }

func collectMatches(matches *[]Match) func(Match) error {
	return func(m Match) error {
		*matches = append(*matches, m)
		return nil
	}
}

func Test_MatchAll_crosses_bid_and_ask_at_ask_price(t *testing.T) {
	b := NewBook()
	seller := testProducer(0, 0)
	buyer := testProducer(10, 10)
	b.PostAsk(seller, "Ingot", 5, 2.0)
	b.PostBid(buyer, "Ingot", 5, 3.0) // 3.0 >= 2.0 + 1.0/5

	var matches []Match
	b.MatchAll(flatTransport, collectMatches(&matches))

	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	m := matches[0]
	if m.Seller != seller || m.Buyer != buyer {
		t.Error("match connected the wrong parties")
	}
	if m.Order.Name != "Ingot" || m.Order.Rate != 5 {
		t.Errorf("expected order Ingot@5, got %s@%f", m.Order.Name, m.Order.Rate)
	}
	if m.UnitPrice != 2.0 {
		t.Errorf("trade should execute at the ask price 2.0, got %f", m.UnitPrice)
	}
	if m.TransportCost != 1.0 {
		t.Errorf("expected transport 1.0, got %f", m.TransportCost)
	}
}

func Test_MatchAll_does_not_cross_unaffordable(t *testing.T) {
	b := NewBook()
	b.PostAsk(testProducer(0, 0), "Ingot", 5, 2.0)
	b.PostBid(testProducer(1, 1), "Ingot", 5, 1.0) // 1.0 < 2.0 + transport

	var matches []Match
	b.MatchAll(flatTransport, collectMatches(&matches))
	if len(matches) != 0 {
		t.Fatalf("expected no matches, got %d", len(matches))
	}
}

func Test_MatchAll_partial_fill_spans_two_asks(t *testing.T) {
	b := NewBook()
	s1 := testProducer(0, 0)
	s2 := testProducer(1, 1)
	buyer := testProducer(2, 2)
	b.PostAsk(s1, "Ingot", 3, 1.0) // cheaper, taken first
	b.PostAsk(s2, "Ingot", 10, 2.0)
	b.PostBid(buyer, "Ingot", 5, 10.0)

	var matches []Match
	b.MatchAll(flatTransport, collectMatches(&matches))

	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0].Seller != s1 || matches[0].Order.Rate != 3 {
		t.Errorf("first match should take all of s1 (rate 3), got %+v", matches[0])
	}
	if matches[1].Seller != s2 || matches[1].Order.Rate != 2 {
		t.Errorf("second match should take rate 2 from s2, got %+v", matches[1])
	}
	if bid := b.Bids("Ingot")[0]; bid.Remaining > 1e-9 {
		t.Errorf("bid should be fully consumed, has %f remaining", bid.Remaining)
	}
}

func Test_MatchAll_higher_bid_served_first(t *testing.T) {
	b := NewBook()
	seller := testProducer(0, 0)
	rich := testProducer(1, 1)
	poor := testProducer(2, 2)
	b.PostAsk(seller, "Ingot", 5, 1.0)
	b.PostBid(poor, "Ingot", 5, 2.0)
	b.PostBid(rich, "Ingot", 5, 9.0)

	var matches []Match
	b.MatchAll(flatTransport, collectMatches(&matches))

	if len(matches) != 1 {
		t.Fatalf("expected exactly 1 match (capacity 5), got %d", len(matches))
	}
	if matches[0].Buyer != rich {
		t.Error("the higher bid should be served first")
	}
}

func Test_MatchAll_prefers_lower_delivered_cost(t *testing.T) {
	b := NewBook()
	near := testProducer(0, 0)
	far := testProducer(100, 100)
	buyer := testProducer(0, 1)
	// Same ask price; transport must decide.
	distanceTransport := func(o, d point.Point) float64 { return o.Distance(d) }
	b.PostAsk(far, "Ingot", 5, 1.0)
	b.PostAsk(near, "Ingot", 5, 1.0)
	b.PostBid(buyer, "Ingot", 5, 100.0)

	var matches []Match
	b.MatchAll(distanceTransport, collectMatches(&matches))

	if len(matches) != 1 || matches[0].Seller != near {
		t.Fatalf("expected the near seller to win, got %+v", matches)
	}
}

func Test_MatchAll_skips_ask_when_sign_fails(t *testing.T) {
	b := NewBook()
	bad := testProducer(0, 0)
	good := testProducer(1, 1)
	buyer := testProducer(2, 2)
	b.PostAsk(bad, "Ingot", 5, 1.0) // cheaper but sign will reject it
	b.PostAsk(good, "Ingot", 5, 2.0)
	b.PostBid(buyer, "Ingot", 5, 10.0)

	var matches []Match
	b.MatchAll(flatTransport, func(m Match) error {
		if m.Seller == bad {
			return errors.New("rejected")
		}
		matches = append(matches, m)
		return nil
	})

	if len(matches) != 1 || matches[0].Seller != good {
		t.Fatalf("expected fallback to the good seller, got %+v", matches)
	}
	if b.Asks("Ingot")[0].Remaining != 5 {
		t.Error("a failed sign must not consume the ask")
	}
}

func Test_MatchAll_never_self_trades(t *testing.T) {
	b := NewBook()
	p := testProducer(0, 0)
	b.PostAsk(p, "Ingot", 5, 1.0)
	b.PostBid(p, "Ingot", 5, 10.0)

	var matches []Match
	b.MatchAll(flatTransport, collectMatches(&matches))
	if len(matches) != 0 {
		t.Fatalf("expected no self-trade, got %d matches", len(matches))
	}
}

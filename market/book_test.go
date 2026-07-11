package market

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/factory"
	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

func testProducer(x, y int) production.Producer {
	return factory.New("Test", "Recipe_Test_C", point.Point{X: x, Y: y},
		0, production.Products{}, production.Products{}, 0)
}

func Test_Book_BestAsk_returns_lowest_unfilled(t *testing.T) {
	b := NewBook()
	s1 := testProducer(0, 0)
	s2 := testProducer(1, 1)
	b.PostAsk(s1, "Ingot", 5, 2.0)
	b.PostAsk(s2, "Ingot", 5, 1.5)

	ask, ok := b.BestAsk("Ingot")
	if !ok {
		t.Fatal("expected a best ask")
	}
	if ask.UnitPrice != 1.5 || ask.Seller != s2 {
		t.Errorf("expected the 1.5 ask from s2, got %f from %v", ask.UnitPrice, ask.Seller)
	}

	// Fully consumed asks are ignored.
	ask.Remaining = 0
	ask, ok = b.BestAsk("Ingot")
	if !ok || ask.UnitPrice != 2.0 {
		t.Errorf("expected the 2.0 ask once the cheaper one is consumed, got %+v ok=%v", ask, ok)
	}
}

func Test_Book_BestBid_returns_highest_unfilled(t *testing.T) {
	b := NewBook()
	b1 := testProducer(0, 0)
	b2 := testProducer(1, 1)
	b.PostBid(b1, "Ingot", 5, 2.0)
	b.PostBid(b2, "Ingot", 5, 3.0)

	bid, ok := b.BestBid("Ingot")
	if !ok || bid.UnitPrice != 3.0 || bid.Buyer != b2 {
		t.Errorf("expected the 3.0 bid from b2, got %+v ok=%v", bid, ok)
	}

	if _, ok := b.BestBid("NoSuchProduct"); ok {
		t.Error("expected no bid for an unknown product")
	}
}

func Test_Book_ignores_zero_rate_orders(t *testing.T) {
	b := NewBook()
	p := testProducer(0, 0)
	b.PostAsk(p, "Ingot", 0, 1.0)
	b.PostBid(p, "Ingot", 0, 1.0)
	if len(b.Asks("Ingot")) != 0 || len(b.Bids("Ingot")) != 0 {
		t.Error("zero-rate orders should not be recorded")
	}
}

func Test_Book_Products_sorted_and_Clear(t *testing.T) {
	b := NewBook()
	p := testProducer(0, 0)
	b.PostAsk(p, "Zinc", 1, 1.0)
	b.PostBid(p, "Alumina", 1, 1.0)
	b.PostBid(p, "Zinc", 1, 1.0) // product on both sides appears once

	got := b.Products()
	if len(got) != 2 || got[0] != "Alumina" || got[1] != "Zinc" {
		t.Errorf("expected sorted unique products [Alumina Zinc], got %v", got)
	}

	b.Clear()
	if len(b.Products()) != 0 {
		t.Error("expected an empty book after Clear")
	}
}

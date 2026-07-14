package resources

import (
	_ "embed"
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

func Test_resource_RemainingCapacityFor(t *testing.T) {
	r := &Resource{
		Production: production.Production{Name: "Stuff", Rate: 10},
		Purity:     "Normal",
		Loc:        point.Point{X: 0, Y: 0},
		Sales:      []*production.Contract{},
	}
	if got := r.RemainingCapacityFor("Stuff"); got != 10 {
		t.Errorf("got %f, want 10", got)
	}
	if got := r.RemainingCapacityFor("SomethingElse"); got != 0 {
		t.Errorf("got %f, want 0", got)
	}
	r.Sales = append(r.Sales, &production.Contract{
		Order: production.Production{Name: "Stuff", Rate: 4},
	})
	if got := r.RemainingCapacityFor("Stuff"); got != 6 {
		t.Errorf("got %f, want 6", got)
	}
}

func Test_resource_HasCapacityFor(t *testing.T) {
	type fields struct {
		Production production.Production
		Purity     purity
		Loc        point.Point
		sales      []*production.Contract
	}
	type args struct {
		order production.Production
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "Allows order if has capacity",
			fields: fields{
				Production: production.Production{Name: "Stuff", Rate: 10},
				Purity:     "Normal",
				Loc:        point.Point{X: 0, Y: 0},
				sales:      []*production.Contract{},
			},
			args:    args{order: production.Production{Name: "Stuff", Rate: 5}},
			wantErr: false,
		},
		{
			name: "Rejects order if insufficient capacity",
			fields: fields{
				Production: production.Production{Name: "Stuff", Rate: 10},
				Purity:     "Normal",
				Loc:        point.Point{X: 0, Y: 0},
				sales:      []*production.Contract{},
			},
			args:    args{order: production.Production{Name: "Stuff", Rate: 15}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Resource{
				Production: tt.fields.Production,
				Purity:     tt.fields.Purity,
				Loc:        tt.fields.Loc,
				Sales:      tt.fields.sales,
			}
			if err := r.HasCapacityFor(tt.args.order); (err != nil) != tt.wantErr {
				t.Errorf("resource.HasCapacityFor() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_Resource_ask_price_defaults_and_sets(t *testing.T) {
	r := &Resource{Production: production.Production{Name: "Ore", Rate: 100}}

	if got := r.AskPriceFor("Ore"); got != production.DefaultUnitPrice {
		t.Errorf("unquoted ask should default to %f, got %f", production.DefaultUnitPrice, got)
	}
	r.SetAskPrice("Ore", 0.5)
	if got := r.AskPriceFor("Ore"); got != 0.5 {
		t.Errorf("got %f, want 0.5", got)
	}
	if got := r.AskPriceFor("NotMyProduct"); got != 0 {
		t.Errorf("asking about a foreign product should return 0, got %f", got)
	}
}

func Test_Resource_ProduceTick(t *testing.T) {
	r := &Resource{
		Production: production.Production{Name: "OreIron", Rate: 2},
		Loc:        point.Point{X: 0, Y: 0},
	}
	r.ProduceTick(3) // cap = 6 units
	if r.Stock != 2 {
		t.Fatalf("stock after 1 tick = %v, want 2", r.Stock)
	}
	r.ProduceTick(3)
	r.ProduceTick(3)
	r.ProduceTick(3) // would be 8, clamps at cap 6
	if r.Stock != 6 {
		t.Fatalf("stock at cap = %v, want 6", r.Stock)
	}
}

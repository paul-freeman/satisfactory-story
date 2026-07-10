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

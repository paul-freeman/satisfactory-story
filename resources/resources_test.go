package resources

import (
	_ "embed"
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

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
			r := &resource{
				Production: tt.fields.Production,
				Purity:     tt.fields.Purity,
				Loc:        tt.fields.Loc,
				sales:      tt.fields.sales,
			}
			if err := r.HasCapacityFor(tt.args.order); (err != nil) != tt.wantErr {
				t.Errorf("resource.HasCapacityFor() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

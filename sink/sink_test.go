package sink

import (
	"testing"

	"github.com/paul-freeman/satisfactory-story/point"
	"github.com/paul-freeman/satisfactory-story/production"
)

func Test_Sink_RecordDelivery(t *testing.T) {
	sk := New("SpaceElevatorPart_1", point.Point{X: 0, Y: 0}, production.Products{
		production.Production{Name: "SpaceElevatorPart_1", Rate: 1},
	}, 1000)
	if sk.TotalDelivered() != 0 {
		t.Fatalf("fresh sink TotalDelivered = %v, want 0", sk.TotalDelivered())
	}
	sk.RecordDelivery("SpaceElevatorPart_1", 2.5)
	sk.RecordDelivery("SpaceElevatorPart_1", 0.5)
	if sk.TotalDelivered() != 3 {
		t.Fatalf("TotalDelivered = %v, want 3", sk.TotalDelivered())
	}
}

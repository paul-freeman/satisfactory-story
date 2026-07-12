package production

import "testing"

func Test_Inventory(t *testing.T) {
	inv := make(Inventory)
	if got := inv.Get("IronOre"); got != 0 {
		t.Fatalf("empty Get = %v, want 0", got)
	}
	inv.Add("IronOre", 5)
	inv.Add("IronOre", 2.5)
	if got := inv.Get("IronOre"); got != 7.5 {
		t.Fatalf("Get after adds = %v, want 7.5", got)
	}
	took := inv.Take("IronOre", 3)
	if took != 3 || inv.Get("IronOre") != 4.5 {
		t.Fatalf("Take(3) = %v (stock %v), want 3 (stock 4.5)", took, inv.Get("IronOre"))
	}
	// Take clamps at what is available and never goes negative.
	took = inv.Take("IronOre", 100)
	if took != 4.5 || inv.Get("IronOre") != 0 {
		t.Fatalf("clamped Take = %v (stock %v), want 4.5 (stock 0)", took, inv.Get("IronOre"))
	}
	if took := inv.Take("Coal", 1); took != 0 {
		t.Fatalf("Take of absent product = %v, want 0", took)
	}
}

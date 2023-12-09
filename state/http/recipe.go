package http

type Recipe struct {
	Name    string    `json:"name"`
	Inputs  []Product `json:"inputs"`
	Outputs []Product `json:"outputs"`
	Active  bool      `json:"active"`
}

type Product struct {
	Name string  `json:"name"`
	Rate float64 `json:"rate"`
}

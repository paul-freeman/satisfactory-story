package http

type State struct {
	Factories  []Factory   `json:"factories"`
	Transports []Transport `json:"transports"`
	Tick       int         `json:"tick"`
	Xmin       int         `json:"xmin"`
	Xmax       int         `json:"xmax"`
	Ymin       int         `json:"ymin"`
	Ymax       int         `json:"ymax"`
}

type Factory struct {
	Location      Location `json:"location"`
	Recipe        string   `json:"recipe"`
	Products      []string `json:"products"`
	Profitability float64  `json:"profitability"`
	Active        bool     `json:"active"`
}

type Transport struct {
	Origin      Location `json:"origin"`
	Destination Location `json:"destination"`
	Rate        float64  `json:"rate"`
}

type Location struct {
	X int `json:"x"`
	Y int `json:"y"`
}

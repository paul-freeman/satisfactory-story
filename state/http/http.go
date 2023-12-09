package http

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
)

type State struct {
	Resources  []Resource  `json:"resources"`
	Factories  []Factory   `json:"factories"`
	Sinks      []Sink      `json:"sinks"`
	Transports []Transport `json:"transports"`
	Tick       int         `json:"tick"`
	Running    bool        `json:"running"`
	Bounds     Bounds      `json:"bounds"`
}

type Bounds struct {
	Xmin int `json:"xmin"`
	Xmax int `json:"xmax"`
	Ymin int `json:"ymin"`
	Ymax int `json:"ymax"`
}

type Resource struct {
	Location      Location `json:"location"`
	Recipe        string   `json:"recipe"`
	Product       string   `json:"product"`
	Profitability float64  `json:"profitability"`
	Active        bool     `json:"active"`
}

type Factory struct {
	Location      Location `json:"location"`
	Recipe        string   `json:"recipe"`
	Products      []string `json:"products"`
	Profitability float64  `json:"profitability"`
}

type Sink struct {
	Location Location `json:"location"`
	Label    string   `json:"label"`
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

type Server interface {
	json.Marshaler
	Tick(*slog.Logger) error
	Run(*slog.Logger)
	Stop(*slog.Logger)
	Reset(*slog.Logger, *slog.Level)
	Recipes(*slog.Logger) []Recipe
	SetRecipe(*slog.Logger, string, bool) []Recipe
}

func Serve(s Server, port string, l *slog.Logger, logLevel *slog.Level) {
	// Setup HTTP server
	http.HandleFunc("/state", handleState(s, l))
	http.HandleFunc("/tick", handleTick(s, l))
	http.HandleFunc("/run", handleRun(s, l))
	http.HandleFunc("/stop", handleStop(s, l))
	http.HandleFunc("/reset", handleReset(s, l, logLevel))
	http.HandleFunc("/recipes", handleRecipes(s, l))
	http.HandleFunc("/recipe/", handleRecipe(s, l))
	http.Handle("/", http.FileServer(http.Dir(".")))
	fmt.Printf("Server running on %s\n", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		panic(fmt.Sprintf("failed to start HTTP server: %v", err))
	}
}

// handleState is a closure over a json.Marshaler that serves the JSON
// representation.
func handleState(s Server, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w)
		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(s); err != nil {
			l.Error("failed to encode state: " + err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// handleTick is a closure over a Server that calls Tick() and returns the
// new state.
func handleTick(s Server, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.Tick(l); err != nil {
			l.Error("failed to tick: " + err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		setCORSHeaders(w)
		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(s); err != nil {
			l.Error("failed to encode state: " + err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// handleRun is a closure over a Server that calls Run(). It doesn't return
// the new state. The application should poll the state using /json.
func handleRun(s Server, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w)
		s.Run(l)
	}
}

// handleStop is a closure over a Server that calls Stop(). It returns the
// final state.
func handleStop(s Server, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w)
		w.Header().Set("Content-Type", "application/json")

		s.Stop(l)
		if err := json.NewEncoder(w).Encode(s); err != nil {
			l.Error("failed to encode state: " + err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// handleReset is a closure over a Server that calls Reset(). It returns the
// initial state.
func handleReset(s Server, l *slog.Logger, logLevel *slog.Level) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w)
		w.Header().Set("Content-Type", "application/json")

		s.Reset(l, logLevel)
		if err := json.NewEncoder(w).Encode(s); err != nil {
			l.Error("failed to encode state: " + err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// handleRecipes is a closure over a Server that calls Recipes(). It returns
// the recipes.
func handleRecipes(s Server, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w)
		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(s.Recipes(l)); err != nil {
			l.Error("failed to encode state: " + err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func handleRecipe(s Server, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setCORSHeaders(w)
		w.Header().Set("Content-Type", "application/json")

		// Split the URL path into segments
		pathSegments := strings.Split(r.URL.Path, "/")

		// Basic validation to check if the correct number of segments are present
		if len(pathSegments) < 4 {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		// Extract the 'name' and 'set' flag from the URL
		name := pathSegments[2]
		setFlag := pathSegments[3]

		rs := s.SetRecipe(l, name, setFlag == "1")

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(rs)
	}
}

func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "http://localhost:8000")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
}

package server

import (
	"net/http"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/service"
)

// figure grows with the progress of the day on the level card. Its drawings come
// from tools/genfigures.
type figure struct {
	Key  string
	Name string
}

const (
	figureStages  = 8
	defaultFigure = "pflanze"
	noFigure      = "keine"
)

var figureGroups = []struct {
	Name    string
	Figures []figure
}{
	{"Natur & Alltag", []figure{{"pflanze", "Pflanze"}, {"ei", "Ei"}, {"kaffee", "Kaffee"}, {"lagerfeuer", "Lagerfeuer"},
		{"aquarium", "Aquarium"}, {"himmel", "Himmel"}, {"gluehbirne", "Glühbirne"}, {"rakete", "Rakete"}}},
	{"Roboter & Technik", []figure{{"roboter", "Roboter"}, {"roboter-bau", "Roboter-Bau"}, {"akku", "Akku"}, {"terminal", "Terminal"},
		{"laptop", "Laptop"}, {"wlan", "WLAN"}, {"git", "Git"}, {"platine", "Platine"}}},
}

func findFigure(key string) (figure, bool) {
	for _, group := range figureGroups {
		for _, fig := range group.Figures {
			if fig.Key == key {
				return fig, true
			}
		}
	}
	return figure{}, false
}

// userFigure is the chosen figure; ok is false when the user turned it off.
func userFigure(q *db.Q) (figure, bool, error) {
	key, err := q.Setting(service.KeyFigure, defaultFigure)
	if err != nil {
		return figure{}, false, err
	}
	if key == noFigure {
		return figure{}, false, nil
	}
	fig, ok := findFigure(key)
	if !ok {
		fig, _ = findFigure(defaultFigure)
	}
	return fig, true, nil
}

// figureStage turns the progress of the day into a stage: 1 means nothing is done,
// the last stage needs everything.
func figureStage(percent int) int {
	return 1 + min(max(percent, 0), 100)*(figureStages-1)/100
}

// dayProgress is the mean fill of the goals of today, so every click counts.
func dayProgress(cards ...[]habitCard) int {
	sum, n := 0, 0
	for _, list := range cards {
		for _, c := range list {
			sum += min(c.Ring, 100)
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / n
}

func (s *Server) figures(w http.ResponseWriter, r *http.Request) error {
	q := s.svc.Store.Q(r.Context())
	fig, on, err := userFigure(q)
	if err != nil {
		return err
	}
	stages := make([]int, figureStages)
	for i := range stages {
		stages[i] = i + 1
	}
	return s.view.Render(w, http.StatusOK, "figures", s.page(r, "Figur", "profile", map[string]any{
		"Groups": figureGroups, "Figure": fig, "On": on, "None": noFigure, "Stages": stages, "Last": figureStages,
	}))
}

func (s *Server) figureSave(w http.ResponseWriter, r *http.Request) error {
	key := r.FormValue("figure")
	if _, ok := findFigure(key); !ok && key != noFigure {
		return badRequest("Unbekannte Figur")
	}
	if err := s.svc.Store.Q(r.Context()).SetSetting(service.KeyFigure, key); err != nil {
		return err
	}
	return redirect(w, r, "/settings/figure")
}

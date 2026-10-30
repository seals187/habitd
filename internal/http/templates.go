package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/service"
	"github.com/seals187/habitd/internal/templates"
	"github.com/seals187/habitd/internal/view"
)

const (
	maxSuggestions  = 5
	startersPerArea = 3
)

// templateRow is a template as shown in a list.
type templateRow struct {
	templates.Template
	Area   model.Area
	Meta   string
	KindDE string
	Active bool
}

var difficultyDE = map[string]string{model.Easy: "leicht", model.Medium: "mittel", model.Hard: "schwer"}

func kindLabel(kind string) string {
	if kind == model.KindAvoid {
		return "Vermeiden"
	}
	return "Aufbauen"
}

func rhythmLabel(t templates.Template) string {
	return view.FrequencyLabel(t.Habit(0, time.Time{}))
}

func capitalized(s string) string {
	if s == "täglich" {
		return "Täglich"
	}
	return s
}

func newRow(t templates.Template, byKey map[string]model.Area, active map[string]bool) templateRow {
	row := templateRow{Template: t, Area: byKey[t.Area], KindDE: kindLabel(t.Kind), Active: active[t.ID]}
	first := capitalized(rhythmLabel(t))
	if t.Kind == model.KindAvoid {
		first = "Vermeiden"
	}
	row.Meta = first + " · " + difficultyDE[t.Difficulty]
	if t.Popular {
		row.Meta += " · beliebt"
	}
	return row
}

func areasByKey(areas []model.Area) map[string]model.Area {
	out := make(map[string]model.Area, len(areas))
	for _, a := range areas {
		if a.Key != "" {
			out[a.Key] = a
		}
	}
	return out
}

func activeTemplates(habits []model.Habit) map[string]bool {
	out := map[string]bool{}
	for _, h := range habits {
		if h.TemplateID != "" && !h.Archived {
			out[h.TemplateID] = true
		}
	}
	return out
}

type templateGroup struct {
	Title string
	Rows  []templateRow
}

type templateAreaChip struct {
	model.Area
	On bool
}

type templatesData struct {
	Query  string
	Kind   string
	Area   string
	Total  int
	Builds int
	Avoids int
	Areas  []templateAreaChip
	Groups []templateGroup
}

func (s *Server) templatesData(r *http.Request) (*templatesData, error) {
	q := s.svc.Store.Q(r.Context())
	areas, err := q.Areas()
	if err != nil {
		return nil, err
	}
	habits, err := q.Habits()
	if err != nil {
		return nil, err
	}
	d := &templatesData{Query: r.FormValue("q"), Kind: r.FormValue("kind"), Area: r.FormValue("area"), Total: len(templates.All()),
		Builds: templates.CountByKind(model.KindBuild), Avoids: templates.CountByKind(model.KindAvoid)}
	if d.Kind != model.KindBuild && d.Kind != model.KindAvoid {
		d.Kind = ""
	}
	byKey, active := areasByKey(areas), activeTemplates(habits)
	for _, a := range areas {
		if a.Key != "" {
			d.Areas = append(d.Areas, templateAreaChip{Area: a, On: a.Key == d.Area})
		}
	}
	found := templates.Filter(d.Query, d.Kind, d.Area)
	for _, a := range templates.Areas() {
		for _, kind := range []string{model.KindBuild, model.KindAvoid} {
			group := templateGroup{Title: byKey[a.ID].Name + " · " + kindLabel(kind)}
			for _, t := range found {
				if t.Area == a.ID && t.Kind == kind {
					group.Rows = append(group.Rows, newRow(t, byKey, active))
				}
			}
			if len(group.Rows) > 0 {
				d.Groups = append(d.Groups, group)
			}
		}
	}
	return d, nil
}

func (s *Server) templateList(w http.ResponseWriter, r *http.Request) error {
	d, err := s.templatesData(r)
	if err != nil {
		return err
	}
	p := s.page(r, "Vorlagen", "profile", d)
	if isHTMX(r) {
		return s.view.Fragment(w, "templates", "templates-list", p)
	}
	return s.view.Render(w, http.StatusOK, "templates", p)
}

// templateSuggest answers the name field of the habit form.
func (s *Server) templateSuggest(w http.ResponseWriter, r *http.Request) error {
	query := r.FormValue("q")
	if query == "" {
		query = r.FormValue("name")
	}
	if query == "" {
		w.WriteHeader(http.StatusOK)
		return nil
	}
	areas, err := s.svc.Store.Q(r.Context()).Areas()
	if err != nil {
		return err
	}
	byKey := areasByKey(areas)
	rows := []templateRow{}
	for _, t := range templates.Suggest(query, maxSuggestions) {
		rows = append(rows, newRow(t, byKey, nil))
	}
	return s.view.Fragment(w, "habit_form", "suggestions", s.page(r, "", "", map[string]any{"Query": query, "Rows": rows}))
}

func (s *Server) addTemplates(ctx context.Context, ids []string) error {
	err := s.svc.Store.Tx(ctx, func(q *db.Q) error {
		areas, err := q.Areas()
		if err != nil {
			return err
		}
		habits, err := q.Habits()
		if err != nil {
			return err
		}
		byKey, active := areasByKey(areas), activeTemplates(habits)
		for _, id := range ids {
			t, ok := templates.Find(id)
			if !ok {
				return badRequest("Unbekannte Vorlage")
			}
			if active[id] {
				continue
			}
			active[id] = true
			area, ok := byKey[t.Area]
			if !ok && len(areas) > 0 { // the built-in area was deleted
				area = areas[0]
			}
			if _, err := q.SaveHabit(t.Habit(area.ID, s.svc.Now())); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return s.svc.HabitSaved(ctx)
}

func (s *Server) templateAdd(w http.ResponseWriter, r *http.Request) error {
	if err := s.addTemplates(r.Context(), []string{r.PathValue("id")}); err != nil {
		return err
	}
	waiting, err := s.svc.Store.Q(r.Context()).HasCelebration()
	if err != nil {
		return err
	}
	if waiting {
		return redirect(w, r, "/celebrate")
	}
	return s.templateList(w, r)
}

type onboardTile struct {
	model.Area
	Count int
}

type onboardGroup struct {
	Area model.Area
	Rows []templateRow
}

type onboardData struct {
	Step   int
	Tiles  []onboardTile
	Groups []onboardGroup
}

func (s *Server) onboarding(w http.ResponseWriter, r *http.Request) error {
	areas, err := s.svc.Store.Q(r.Context()).Areas()
	if err != nil {
		return err
	}
	d := &onboardData{Step: 1}
	switch r.FormValue("step") {
	case "2":
		d.Step = 2
		s.onboardingGroups(d, areas, r.Form["area"])
		if len(d.Groups) == 0 {
			return redirect(w, r, "/onboarding")
		}
	case "3":
		d.Step = 3
	default:
		counts := templates.CountByArea()
		for _, a := range areas {
			if counts[a.Key] > 0 {
				d.Tiles = append(d.Tiles, onboardTile{Area: a, Count: counts[a.Key]})
			}
		}
	}
	return s.view.Render(w, http.StatusOK, "onboarding", s.page(r, "Willkommen", "", d))
}

func (s *Server) onboardingGroups(d *onboardData, areas []model.Area, picked []string) {
	byKey := areasByKey(areas)
	for _, a := range areas {
		if !contains(picked, a.Key) {
			continue
		}
		group := onboardGroup{Area: a}
		for _, t := range templates.Starters(a.Key, startersPerArea) {
			row := newRow(t, byKey, nil)
			row.Meta = kindLabel(t.Kind)
			if t.Kind == model.KindBuild {
				row.Meta += " · " + rhythmLabel(t)
			}
			group.Rows = append(group.Rows, row)
		}
		d.Groups = append(d.Groups, group)
	}
}

func contains(list []string, want string) bool {
	for _, x := range list {
		if x == want {
			return true
		}
	}
	return false
}

func (s *Server) onboardingFinish(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return badRequest("Ungültiges Formular")
	}
	if err := s.addTemplates(r.Context(), r.Form["template"]); err != nil {
		return err
	}
	if err := s.svc.Store.Q(r.Context()).SetSetting(service.KeyOnboarded, "1"); err != nil {
		return err
	}
	return redirect(w, r, "/onboarding?step=3")
}

func (s *Server) onboardingSkip(w http.ResponseWriter, r *http.Request) error {
	if err := s.svc.Store.Q(r.Context()).SetSetting(service.KeyOnboarded, "1"); err != nil {
		return err
	}
	return redirect(w, r, "/")
}

// needsOnboarding is true on a fresh installation.
func (s *Server) needsOnboarding(ctx context.Context) (bool, error) {
	q := s.svc.Store.Q(ctx)
	done, err := q.Setting(service.KeyOnboarded, "")
	if err != nil || done == "1" {
		return false, err
	}
	habits, err := q.Habits()
	return len(habits) == 0, err
}

// popularChip is a shortcut in the empty habit form.
type popularChip struct {
	Label string
	templateRow
}

var popularChips = []struct{ label, id string }{
	{"Wasser", "8-glaser-wasser-trinken"},
	{"Lesen", "20-min-lesen"},
	{"Meditieren", "10-min-meditieren"},
	{"Eltern anrufen", "eltern-anrufen"},
	{"Doomscrolling", "doomscrolling"},
}

func popularRows(areas []model.Area) []popularChip {
	byKey := areasByKey(areas)
	out := make([]popularChip, 0, len(popularChips))
	for _, c := range popularChips {
		if t, ok := templates.Find(c.id); ok {
			out = append(out, popularChip{Label: c.label, templateRow: newRow(t, byKey, nil)})
		}
	}
	return out
}

// freqOption is a rhythm pill in the habit form.
type freqOption struct {
	Value   string
	Label   string
	Checked bool
}

// freqOptions lists the standard rhythms plus the habit's own, if it differs.
func freqOptions(h model.Habit) []freqOption {
	current := "custom"
	switch h.Frequency {
	case model.FreqDaily:
		current = "daily"
	case model.FreqWeekly:
		current = fmt.Sprintf("w%d", h.WeeklyTarget)
	case model.FreqMonthly:
		current = fmt.Sprintf("m%d", h.WeeklyTarget)
	}
	options := []freqOption{{Value: "daily", Label: "Täglich"}, {Value: "w3", Label: "3× / Woche"}, {Value: "w5", Label: "5× / Woche"}}
	known := current == "custom"
	for _, o := range options {
		known = known || o.Value == current
	}
	if !known {
		unit := "Woche"
		if h.Frequency == model.FreqMonthly {
			unit = "Monat"
		}
		options = append(options, freqOption{Value: current, Label: fmt.Sprintf("%d× / %s", h.WeeklyTarget, unit)})
	}
	options = append(options, freqOption{Value: "custom", Label: "Eigene"})
	for i := range options {
		options[i].Checked = options[i].Value == current
	}
	return options
}

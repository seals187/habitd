// Package templates holds the embedded habit suggestions. They are read-only:
// adopting one creates a normal habit that remembers the template id.
package templates

import (
	_ "embed" // templates.json
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/seals187/habitd/internal/model"
)

//go:embed templates.json
var raw []byte

// Frequency is the rhythm of a template.
type Frequency struct {
	Type  string `json:"type"`
	Times int    `json:"times"`
}

// Measure turns a template into a counter or quantity habit.
type Measure struct {
	Type   string `json:"type"`
	Target int    `json:"target"`
	Step   int    `json:"step"`
	Unit   string `json:"unit"`
}

// Template is a habit suggestion.
type Template struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`
	Area       string    `json:"area"`
	Icon       string    `json:"icon"`
	Tag        string    `json:"tag"`
	Frequency  Frequency `json:"frequency"`
	Difficulty string    `json:"difficulty"`
	Popular    bool      `json:"popular"`
	Measure    *Measure  `json:"measure,omitempty"`

	fold string
}

// Area is a default life area.
type Area struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Icon  string `json:"icon"`
}

type file struct {
	Areas     []Area     `json:"areas"`
	Templates []Template `json:"templates"`
}

var data = load()

func load() file {
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		panic(fmt.Sprintf("templates.json: %v", err)) // embedded at build time, covered by tests
	}
	for i := range f.Templates {
		f.Templates[i].fold = Fold(f.Templates[i].Name)
	}
	return f
}

// All returns every template in file order.
func All() []Template { return data.Templates }

// Areas returns the default areas.
func Areas() []Area { return data.Areas }

// Find returns a template by id.
func Find(id string) (Template, bool) {
	for _, t := range data.Templates {
		if t.ID == id {
			return t, true
		}
	}
	return Template{}, false
}

// CountByArea counts the templates per area.
func CountByArea() map[string]int {
	out := map[string]int{}
	for _, t := range data.Templates {
		out[t.Area]++
	}
	return out
}

// CountByKind counts the templates of a kind.
func CountByKind(kind string) int {
	n := 0
	for _, t := range data.Templates {
		if t.Kind == kind {
			n++
		}
	}
	return n
}

var folder = strings.NewReplacer("ä", "a", "ö", "o", "ü", "u", "ß", "ss", "ae", "a", "oe", "o", "ue", "u")

// Fold normalises text for search: case and umlauts do not matter, so "mude",
// "muede" and "Müde" all match.
func Fold(s string) string { return folder.Replace(strings.ToLower(s)) }

// Filter selects templates. Empty fields match everything.
func Filter(query, kind, area string) []Template {
	q := Fold(strings.TrimSpace(query))
	out := []Template{}
	for _, t := range data.Templates {
		if (kind == "" || t.Kind == kind) && (area == "" || t.Area == area) && strings.Contains(t.fold, q) {
			out = append(out, t)
		}
	}
	return out
}

// Suggest returns the best matches for a typed name: popular ones first, then
// names starting with the query.
func Suggest(query string, limit int) []Template {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	out := Filter(query, "", "")
	q := Fold(strings.TrimSpace(query))
	rank := func(t Template) int {
		switch {
		case t.Popular:
			return 0
		case strings.HasPrefix(t.fold, q):
			return 1
		}
		return 2
	}
	slices.SortStableFunc(out, func(a, b Template) int { return rank(a) - rank(b) })
	return out[:min(len(out), limit)]
}

// Starters returns the suggestions for an area in the onboarding: up to max
// popular ones, filled up with the next ones to at least two.
func Starters(area string, limit int) []Template {
	var popular, rest []Template
	for _, t := range data.Templates {
		switch {
		case t.Area != area:
		case t.Popular:
			popular = append(popular, t)
		default:
			rest = append(rest, t)
		}
	}
	for _, t := range rest {
		sameTag := slices.ContainsFunc(popular, func(p Template) bool { return p.Tag != "" && p.Tag == t.Tag })
		if len(popular) < 2 && !sameTag {
			popular = append(popular, t)
		}
	}
	return popular[:min(len(popular), limit)]
}

// Habit turns a template into a new habit.
func (t Template) Habit(areaID int64, now time.Time) model.Habit {
	h := model.Habit{TemplateID: t.ID, Name: t.Name, Kind: t.Kind, Icon: t.Icon, Tag: t.Tag, AreaID: areaID,
		Difficulty: t.Difficulty, Frequency: model.FreqDaily, CreatedAt: now}
	if t.Kind == model.KindBuild && t.Frequency.Type != "daily" {
		h.Frequency, h.WeeklyTarget = t.Frequency.Type, t.Frequency.Times
	}
	if m := t.Measure; m != nil {
		h.Measure, h.Target, h.Step, h.Unit = m.Type, m.Target, m.Step, m.Unit
	}
	return h
}

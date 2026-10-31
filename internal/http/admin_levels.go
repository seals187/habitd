package server

import (
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
)

// levelMilestones stay visible while the table is collapsed.
var levelMilestones = []int{1, 2, 3, 4, 5, 10, 15, 25, 50}

// avgHabitXP is the XP of an average check-in, a habit of medium difficulty
// before any streak bonus.
const avgHabitXP = 15

const (
	chartWidth  = 300
	chartHeight = 130
)

type levelsData struct {
	Rows     []game.Level
	All      bool
	Hidden   int
	Settings game.LevelSettings
	Base     string
	Exponent string
	Error    string
	Current  string // polyline points
	Compare  string
	CompareY int
	CurrentY int
	Label    string // name of the compare curve
	Estimate string
}

func decimal(f float64) string {
	return strings.ReplaceAll(strconv.FormatFloat(f, 'f', -1, 64), ".", ",")
}

func parseDecimal(s string) (float64, error) {
	return strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", "."), 64)
}

// estimateLabel turns days into a rough span.
func estimateLabel(days int) string {
	switch {
	case days < 14:
		return fmt.Sprintf("≈ %d Tage", days)
	case days < 70:
		return fmt.Sprintf("≈ %d Wochen", (days+3)/7)
	}
	return fmt.Sprintf("≈ %d Monate", (days+15)/30)
}

func polyline(xp []int, top int) string {
	var b strings.Builder
	for i, v := range xp {
		x := float64(i) * chartWidth / float64(len(xp)-1)
		y := chartHeight - float64(v)/float64(top)*(chartHeight-6)
		fmt.Fprintf(&b, "%.1f,%.1f ", x, y)
	}
	return strings.TrimSpace(b.String())
}

// fillChart draws the curve in the table against the same formula at another tempo.
func fillChart(d *levelsData, levels []game.Level, st game.LevelSettings) {
	compare := st
	compare.Tempo = game.TempoRelaxed
	d.Label = "gemütlich"
	if st.Tempo == game.TempoRelaxed {
		compare.Tempo = game.TempoNormal
		d.Label = "normal"
	}
	current := make([]int, len(levels))
	other := make([]int, len(levels))
	for i, l := range levels {
		current[i] = l.XP
		other[i] = compare.XP(l.Level)
	}
	top := max(slices.Max(current), slices.Max(other), 1)
	d.Current, d.Compare = polyline(current, top), polyline(other, top)
	d.CurrentY = chartHeight - current[len(current)-1]*(chartHeight-6)/top
	d.CompareY = chartHeight - other[len(other)-1]*(chartHeight-6)/top
}

func (s *Server) levelsPage(w http.ResponseWriter, r *http.Request, status int, levels []game.Level, st game.LevelSettings, message string) error {
	// A rejected threshold can collide with a level the short table hides.
	d := levelsData{All: r.FormValue("all") == "1" || message != "", Settings: st, Base: decimal(st.Base), Exponent: decimal(st.Exponent), Error: message}
	for _, l := range levels {
		if d.All || slices.Contains(levelMilestones, l.Level) || l.Overridden {
			d.Rows = append(d.Rows, l)
		}
	}
	d.Hidden = len(levels) - len(d.Rows)
	fillChart(&d, levels, st)
	if len(levels) >= 10 {
		d.Estimate = estimateLabel(max(int(math.Ceil(float64(levels[9].XP)/(5*avgHabitXP))), 1))
	}
	return s.view.Render(w, status, "admin_levels", s.adminPage(r, "Levels", "levels", d))
}

func (s *Server) adminLevels(w http.ResponseWriter, r *http.Request) error {
	rules := s.svc.Rules()
	return s.levelsPage(w, r, http.StatusOK, rules.Levels, rules.Settings, "")
}

// parseLevelSettings reads the formula and the switches.
func parseLevelSettings(r *http.Request, st *game.LevelSettings) string {
	base, err := parseDecimal(r.FormValue("base"))
	if err != nil || base <= 0 || base > 100000 {
		return "Die Basis muss eine Zahl über 0 sein"
	}
	exponent, err := parseDecimal(r.FormValue("exponent"))
	if err != nil || exponent <= 0 || exponent > 5 {
		return "Der Exponent muss zwischen 0 und 5 liegen"
	}
	st.Base, st.Exponent = base, exponent
	switch tempo := r.FormValue("tempo"); tempo {
	case game.TempoRelaxed, game.TempoNormal, game.TempoAmbitious:
		st.Tempo = tempo
	default:
		return "Unbekanntes Tempo"
	}
	st.Celebrate = r.FormValue("celebrate") == "on"
	st.NoDrop = r.FormValue("no_drop") == "on"
	st.ShowTitle = r.FormValue("show_title") == "on"
	st.Limit = r.FormValue("limit") == "on"
	return ""
}

// parseLevelRows applies the visible rows. An XP value that differs from the
// stored one counts as set by hand.
func parseLevelRows(r *http.Request, levels []game.Level) string {
	for _, raw := range r.Form["level"] {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > len(levels) {
			return "Unbekanntes Level"
		}
		row := &levels[n-1]
		row.Title = strings.TrimSpace(r.FormValue("title_" + raw))
		xp, err := strconv.Atoi(r.FormValue("xp_" + raw))
		if err != nil {
			return fmt.Sprintf("Level %d: ungültige XP-Schwelle", n)
		}
		if row.Coins, err = strconv.Atoi(r.FormValue("coins_" + raw)); err != nil || row.Coins < 0 {
			return fmt.Sprintf("Level %d: ungültige Taler", n)
		}
		if row.Jokers, err = strconv.Atoi(r.FormValue("jokers_" + raw)); err != nil || row.Jokers < 0 || row.Jokers > game.MaxJokers {
			return fmt.Sprintf("Level %d: ungültige Joker", n)
		}
		if xp != row.XP {
			row.XP, row.Overridden = xp, true
		}
	}
	return ""
}

// applyFormula recomputes everything that was not set by hand.
func applyFormula(levels []game.Level, st game.LevelSettings) {
	for i := range levels {
		if !levels[i].Overridden {
			levels[i].XP = st.XP(levels[i].Level)
		}
	}
}

func validateLevels(levels []game.Level) string {
	for i, l := range levels {
		if l.Title == "" {
			return fmt.Sprintf("Level %d braucht einen Titel", l.Level)
		}
		if i == 0 {
			if l.XP != 0 {
				return "Level 1 muss bei 0 XP beginnen"
			}
			continue
		}
		if l.XP <= levels[i-1].XP {
			return fmt.Sprintf("Die XP-Schwellen müssen streng steigen: Level %d (%d XP) liegt nicht über Level %d (%d XP)", l.Level, l.XP, levels[i-1].Level, levels[i-1].XP)
		}
	}
	return ""
}

// storeLevels saves the table and moves every user to the matching level.
func (s *Server) storeLevels(r *http.Request, levels []game.Level, st game.LevelSettings) error {
	err := s.svc.Store.Tx(r.Context(), func(q *db.Q) error {
		if err := q.SaveLevels(levels); err != nil {
			return err
		}
		return q.SaveLevelSettings(st)
	})
	if err != nil {
		return err
	}
	if err := s.svc.LoadRules(r.Context()); err != nil {
		return err
	}
	return s.svc.RecalculateLevels(r.Context())
}

func levelsURL(r *http.Request) string {
	if r.FormValue("all") == "1" {
		return "/admin/levels?all=1"
	}
	return "/admin/levels"
}

func (s *Server) adminLevelsSave(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return badRequest("Ungültiges Formular")
	}
	rules := s.svc.Rules()
	st := rules.Settings
	levels := slices.Clone(rules.Levels)
	message := parseLevelSettings(r, &st)
	if message == "" {
		message = parseLevelRows(r, levels)
	}
	if message == "" {
		applyFormula(levels, st)
		message = validateLevels(levels)
	}
	if message != "" {
		return s.levelsPage(w, r, http.StatusUnprocessableEntity, levels, st, message)
	}
	if err := s.storeLevels(r, levels, st); err != nil {
		return err
	}
	return redirect(w, r, levelsURL(r))
}

// adminLevelFormula gives a level back to the formula.
func (s *Server) adminLevelFormula(w http.ResponseWriter, r *http.Request) error {
	n, err := strconv.Atoi(r.PathValue("level"))
	rules := s.svc.Rules()
	if err != nil || n < 1 || n > len(rules.Levels) {
		return notFound()
	}
	levels := slices.Clone(rules.Levels)
	levels[n-1].Overridden = false
	applyFormula(levels, rules.Settings)
	if message := validateLevels(levels); message != "" {
		return s.levelsPage(w, r, http.StatusUnprocessableEntity, rules.Levels, rules.Settings, message)
	}
	if err := s.storeLevels(r, levels, rules.Settings); err != nil {
		return err
	}
	return redirect(w, r, levelsURL(r))
}

func (s *Server) adminLevelsReset(w http.ResponseWriter, r *http.Request) error {
	if err := s.storeLevels(r, game.DefaultLevels(), game.DefaultLevelSettings()); err != nil {
		return err
	}
	return redirect(w, r, "/admin/levels")
}

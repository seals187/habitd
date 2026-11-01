package server

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

// The groups of the streak sheet.
const (
	streakRisk = "risk"
	streakSafe = "safe"
	streakNew  = "new"
)

// streakRow is one habit in the streak sheet.
type streakRow struct {
	ID       int64
	Name     string
	Value    int
	Detail   []string // joined by a dot
	Record   bool
	Ring     int
	Weeks    bool // counted in weeks or months, which shows the calendar
	Unit     string
	rank     int
	goalLeft int
	goalName string
}

type streakGroup struct {
	Key   string
	Title string
	Rows  []streakRow
}

// bestRun is the longest running streak of one rhythm.
type bestRun struct {
	Value  int
	Unit   string // T, Wo or Mo
	Long   string // 12 Tage
	Habit  string
	Record bool
}

// streakGoal is what the best daily streak heads for.
type streakGoal struct {
	Left    int
	Name    string
	Percent int
}

type streakSheet struct {
	Best   []bestRun
	Day    *bestRun
	Other  *bestRun // weeks, or months without any weeks
	Goal   *streakGoal
	Groups []streakGroup
	Total  int
	Risk   int
	Safe   int
	New    int
}

// Line is the running streak of a rhythm for the short tile.
func (b bestRun) Line() string { return fmt.Sprintf("%d %s", b.Value, b.Unit) }

// Week and month streaks have no badges, so their goals are round spans.
var (
	weekGoals  = []int{4, 8, 12, 26, 52, 104}
	monthGoals = []int{3, 6, 12, 24}
)

// dayGoals lists the milestones of a daily streak: the streak badges in use, or
// their default lengths when badges are off.
func dayGoals(rules *game.Rules) []game.Badge {
	var out []game.Badge
	for _, b := range rules.Badges {
		if b.Active && b.Metric.Rule == game.RuleStreak && b.Metric.Scope == game.ScopeKindBuild {
			out = append(out, b)
		}
	}
	if rules.BadgesOff || len(out) == 0 {
		out = out[:0]
		for _, days := range []int{3, 7, 14, 30, 60, 100, 180, 365} {
			out = append(out, game.Badge{Target: days})
		}
	}
	slices.SortFunc(out, func(a, b game.Badge) int { return a.Target - b.Target })
	return out
}

// nextGoal finds the next span after value and how far the streak got.
func nextGoal(value int, goals []int) (target, ring int) {
	for _, g := range goals {
		if g > value {
			return g, min(value*100/g, 100)
		}
	}
	return 0, 100
}

func plainGoals(badges []game.Badge) []int {
	out := make([]int, len(badges))
	for i, b := range badges {
		out[i] = b.Target
	}
	return out
}

func streakUnitOf(s game.Streak) (short string, goals []int, rank int) {
	switch {
	case s.Monthly:
		return "Mo", monthGoals, 2
	case s.Weekly:
		return "Wo", weekGoals, 1
	}
	return "T", nil, 0
}

// recordPart names the record of a streak, or marks a running record with a star.
func recordPart(row *streakRow, s game.Streak) string {
	if s.Current > 1 && s.Current >= s.Longest {
		row.Record = true
		return ""
	}
	return fmt.Sprintf("Rekord %d %s", s.Longest, row.Unit)
}

// periodLine says how far the running week or month is.
func periodLine(h *game.HabitData, today model.Day) string {
	span := "diese Woche"
	if h.Frequency == model.FreqMonthly {
		span = "diesen Monat"
	}
	return fmt.Sprintf("%d von %d %s", min(h.PeriodDone(today), h.WeekTarget()), h.WeekTarget(), span)
}

// restartLine invites to start a broken streak again.
func restartLine(row *streakRow, h *game.HabitData, s game.Streak, today model.Day) []string {
	var head string
	if s.Longest > 0 {
		head = fmt.Sprintf("Rekord %d %s", s.Longest, row.Unit)
	}
	switch {
	case h.Kind == model.KindAvoid && slices.Contains(h.Relapses, today):
		return joinDash(head, "morgen wieder anfangen")
	case h.Kind == model.KindAvoid:
		return joinDash(head, "zählt, sobald der Tag vorbei ist")
	case s.Weekly || s.Monthly:
		return joinDash(head, periodLine(h, today))
	}
	if head == "" {
		return []string{"heute anfangen"}
	}
	return joinDash(head, "heute wieder anfangen")
}

func joinDash(head, tail string) []string {
	if head == "" {
		return []string{tail}
	}
	return []string{head + " – " + tail}
}

// hoursLine says how much of the day is left.
func hoursLine(hours int) string {
	if hours <= 1 {
		return "letzte Stunde"
	}
	return fmt.Sprintf("%d Std. übrig", hours)
}

// openLine says what is missing for a streak of today.
func openLine(h *game.HabitData, today model.Day) string {
	if !h.Measured() {
		return "heute noch offen"
	}
	u := amountUnitOf(h.Unit)
	return "noch " + u.label(model.RoundAmount(h.Target-h.GoalTotal(today))) + " bis zum Ziel"
}

// dayRow fills the row of a streak counted in days.
func dayRow(row *streakRow, h *game.HabitData, s game.Streak, today model.Day, hours int, badges []game.Badge) string {
	target, ring := nextGoal(s.Current, plainGoals(badges))
	row.Ring = ring
	goal := ""
	if target > 0 {
		goal = fmt.Sprintf("nächstes Ziel %d T", target)
		row.goalLeft, row.goalName = target-s.Current, spanWord(target)
		if h.Kind == model.KindBuild {
			if i := slices.IndexFunc(badges, func(b game.Badge) bool { return b.Target == target }); i >= 0 && badges[i].Name != "" {
				row.goalName = badges[i].Name
				goal = fmt.Sprintf("noch %s bis „%s“", spanWord(row.goalLeft), row.goalName)
			}
		}
	}
	_, reached := h.GoalCheckin(today)
	if h.Kind == model.KindBuild && !reached && !h.Joker[today] {
		row.Detail = []string{openLine(h, today), hoursLine(hours)}
		return streakRisk
	}
	record := recordPart(row, s)
	row.Detail = slices.DeleteFunc([]string{goal, record}, func(p string) bool { return p == "" })
	return streakSafe
}

func spanWord(n int) string {
	if n == 1 {
		return "1 Tag"
	}
	return strconv.Itoa(n) + " Tage"
}

// periodRow fills the row of a streak counted in weeks or months.
func periodRow(row *streakRow, h *game.HabitData, s game.Streak, today model.Day, goals []int) string {
	_, row.Ring = nextGoal(s.Current, goals)
	missing := h.WeekTarget() - h.PeriodDone(today)
	record := recordPart(row, s)
	switch {
	case missing <= 0:
		span := "diese Woche"
		if s.Monthly {
			span = "diesen Monat"
		}
		row.Detail = slices.DeleteFunc([]string{record, span + " schon erreicht"}, func(p string) bool { return p == "" })
		return streakSafe
	case missing >= h.DaysLeft(today):
		row.Detail = []string{periodLine(h, today), fmt.Sprintf("noch %s", spanWord(h.DaysLeft(today)))}
		return streakRisk
	}
	row.Detail = slices.DeleteFunc([]string{record, periodLine(h, today)}, func(p string) bool { return p == "" })
	return streakSafe
}

// newStreakRow sorts a habit into its group and words its state.
func newStreakRow(h *game.HabitData, today model.Day, hours int, badges []game.Badge) (streakRow, string) {
	s := h.StreakAt(today)
	row := streakRow{ID: h.ID, Name: h.Name, Value: s.Current, Weeks: s.Weekly || s.Monthly}
	var goals []int
	row.Unit, goals, row.rank = streakUnitOf(s)
	if s.Current == 0 {
		row.Detail = restartLine(&row, h, s, today)
		return row, streakNew
	}
	if s.Weekly || s.Monthly {
		return row, periodRow(&row, h, s, today, goals)
	}
	return row, dayRow(&row, h, s, today, hours, badges)
}

// longUnit writes a streak out: 12 Tage, 1 Woche.
func longUnit(value int, unit string) string {
	names := map[string][2]string{"T": {"Tag", "Tage"}, "Wo": {"Woche", "Wochen"}, "Mo": {"Monat", "Monate"}}[unit]
	if value == 1 {
		return "1 " + names[0]
	}
	return fmt.Sprintf("%d %s", value, names[1])
}

func (s *Server) streakSheet(w *game.World) *streakSheet {
	badges := dayGoals(s.svc.Rules())
	hours := 24 - s.svc.Now().In(s.svc.Loc).Hour()
	groups := map[string]*streakGroup{
		streakRisk: {Key: streakRisk, Title: "In Gefahr · heute noch offen"},
		streakSafe: {Key: streakSafe, Title: "Heute gesichert"},
		streakNew:  {Key: streakNew, Title: "Neu starten"},
	}
	sheet := &streakSheet{}
	best := map[string]*bestRun{}
	var bestDay streakRow
	for _, h := range w.Habits {
		if h.Archived || w.Today < h.Start {
			continue
		}
		row, group := newStreakRow(h, w.Today, hours, badges)
		groups[group].Rows = append(groups[group].Rows, row)
		sheet.Total++
		if b := best[row.Unit]; row.Value > 0 && (b == nil || row.Value > b.Value) {
			st := h.StreakAt(w.Today)
			best[row.Unit] = &bestRun{Value: row.Value, Unit: row.Unit, Long: longUnit(row.Value, row.Unit), Habit: h.Name, Record: st.Current > 1 && st.Current >= st.Longest}
			if row.Unit == "T" {
				bestDay = row
			}
		}
	}
	for _, unit := range []string{"T", "Wo", "Mo"} {
		if b := best[unit]; b != nil {
			sheet.Best = append(sheet.Best, *b)
		}
	}
	sheet.Day = best["T"]
	if sheet.Other = best["Wo"]; sheet.Other == nil {
		sheet.Other = best["Mo"]
	}
	if bestDay.goalName != "" {
		sheet.Goal = &streakGoal{Left: bestDay.goalLeft, Name: bestDay.goalName, Percent: bestDay.Ring}
	}
	sheet.Risk, sheet.Safe, sheet.New = len(groups[streakRisk].Rows), len(groups[streakSafe].Rows), len(groups[streakNew].Rows)
	for _, key := range []string{streakRisk, streakSafe, streakNew} {
		g := groups[key]
		if len(g.Rows) == 0 {
			continue
		}
		slices.SortStableFunc(g.Rows, func(a, b streakRow) int {
			if a.rank != b.rank {
				return a.rank - b.rank
			}
			return b.Value - a.Value
		})
		sheet.Groups = append(sheet.Groups, *g)
	}
	return sheet
}

// DetailLine joins the parts of a row.
func (r streakRow) DetailLine() string { return strings.Join(r.Detail, " · ") }

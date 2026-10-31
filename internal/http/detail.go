package server

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/service"
	"github.com/seals187/habitd/internal/view"
)

const heatmapWeeks = 53

// dayCell is one day of the week strip or the month grid.
type dayCell struct {
	Label    int
	Letter   string
	Date     string
	State    string // done, part, joker, missed, open or none
	Fill     int    // filled height of a partly reached day, in percent
	Value    string // amount of the day, for habits with a unit
	Today    bool
	Editable bool
}

// barDay is one bar of the week chart.
type barDay struct {
	Name    string
	Label   string
	Height  int
	Reached bool
	Today   bool
	Future  bool
	Empty   bool // before the habit started
}

type weekChart struct {
	Title     string
	Summary   string
	Bars      []barDay
	GoalAt    int // height of the goal line in percent, 0 hides it
	GoalLabel string
}

type heatCell struct {
	Class string
	Title string
}

type heatColumn struct {
	Month string
	Cells []heatCell
}

type monthBar struct {
	Letter  string
	Label   string
	Height  int
	Empty   bool
	Current bool
}

// statBox is one of the small figures at the end of a view.
type statBox struct {
	Value string
	Label string
	Class string
}

type linkedReward struct {
	Title    string
	Progress int
	Target   int
	Percent  int
	Ready    bool
}

type detailData struct {
	Habit      model.Habit
	Area       model.Area
	Frequency  string
	Since      string
	View       string
	Streak     game.Streak
	Avoid      bool
	Ring       int
	Quote      int
	XPTotal    int
	CoinsTotal int
	Strip      []dayCell

	MonthParam string
	MonthTitle string
	PrevMonth  string
	NextMonth  string
	Weeks      [][]dayCell

	Chart weekChart

	YearDone  int
	Heat      []heatColumn
	StartHint string
	MonthBars []monthBar

	Amount *amountData // set for habits with a unit, replaces the streak

	Stats   []statBox
	Rewards []linkedReward
}

var (
	dayLetters = []string{"M", "D", "M", "D", "F", "S", "S"}
	dayNames   = []string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"}
)

// streakMilestones are the marks the ring around the streak runs towards.
var streakMilestones = []int{7, 30, 60, 100, 180, 365, 500, 1000}

func ringPercent(days int) int {
	for _, m := range streakMilestones {
		if days < m {
			return days * 100 / m
		}
	}
	return 100
}

func dayMonth(d model.Day) string {
	return fmt.Sprintf("%d. %s", d.Dom(), view.ShortMonths[d.Month()-1])
}

// sinceLabel adds the year only when it is not the current one.
func sinceLabel(d, today model.Day) string {
	if d.Year() == today.Year() {
		return dayMonth(d)
	}
	return fmt.Sprintf("%s %d", dayMonth(d), d.Year())
}

// rangeLabel formats a span like 26.–31. Okt.
func rangeLabel(from, to model.Day) string {
	switch {
	case from == to:
		return dayMonth(from)
	case from.Month() == to.Month():
		return fmt.Sprintf("%d.–%s", from.Dom(), dayMonth(to))
	}
	return dayMonth(from) + "–" + dayMonth(to)
}

// cellFor derives the display state of one day.
func cellFor(h *game.HabitData, d, today model.Day, totals map[model.Day]float64) dayCell {
	c := dayCell{Label: d.Dom(), Letter: dayLetters[d.WeekdayIndex()], Date: d.String(), Today: d == today}
	c.Editable = d <= today && d >= today-game.BackfillDays && !h.Archived
	switch {
	case h.Kind == model.KindAvoid:
		switch {
		case d < h.Start:
			c.State, c.Editable = "none", false
		case containsDay(h.Relapses, d):
			c.State = "missed"
		case d >= today:
			c.State = "open"
		default:
			c.State = "done"
		}
	case hasDone(h, d) && !h.GoalWeek:
		c.State = "done"
	case h.Joker[d]:
		c.State = "joker"
	case h.Measured() && totals[d] > 0:
		c.State = "part"
		c.Fill = min(int(totals[d]*100/h.Target), 99)
	case d >= today:
		c.State = "open"
	case h.Scheduled(d):
		c.State = "missed"
	default:
		c.State = "none"
	}
	return c
}

func containsDay(days []model.Day, d model.Day) bool {
	for _, x := range days {
		if x == d {
			return true
		}
	}
	return false
}

func hasDone(h *game.HabitData, d model.Day) bool {
	_, ok := h.Done[d]
	return ok
}

// planned counts the expected and the achieved days in a range. Weekly rhythms
// expect their share of the days.
func planned(h *game.HabitData, from, to model.Day) (done, plan int) {
	from = max(from, h.Start)
	if h.Kind == model.KindAvoid {
		for d := from; d <= to; d++ {
			plan++
			if !containsDay(h.Relapses, d) {
				done++
			}
		}
		return done, plan
	}
	days := 0
	for d := from; d <= to; d++ {
		days++
		if h.Scheduled(d) {
			plan++
		}
		if hasDone(h, d) {
			done++
		}
	}
	switch h.Frequency {
	case model.FreqWeekly:
		plan = (days*h.WeekTarget() + 6) / 7
	case model.FreqMonthly:
		plan = (days*h.WeekTarget() + 29) / 30
	}
	return done, plan
}

// lastSettledDay is the end of a statistic: today only counts once it is decided,
// an open day is neither done nor missed yet.
func lastSettledDay(h *game.HabitData, today model.Day) model.Day {
	if hasDone(h, today) || containsDay(h.Relapses, today) {
		return today
	}
	return today - 1
}

// rememberedView stores an explicitly chosen view of a habit and falls back to
// the stored one, so every habit opens the way it was left.
func rememberedView(q *db.Q, id int64, chosen string) (string, error) {
	stored, err := q.HabitView(id)
	if err != nil {
		return "", err
	}
	switch chosen {
	case "week", "month", "year":
		if chosen == stored {
			return chosen, nil
		}
		return chosen, q.SetHabitView(id, chosen)
	}
	return stored, nil
}

func percent(part, whole int) int {
	if whole <= 0 {
		return 0
	}
	return min(int(float64(part)*100/float64(whole)+0.5), 100)
}

// goalOf is the value a day needs: the goal of a measured habit, else one tick.
func goalOf(h *game.HabitData) float64 {
	if h.Measured() {
		return h.Target
	}
	return 1
}

// valueOn is what a day is worth in the charts.
func valueOn(h *game.HabitData, d, today model.Day, totals map[model.Day]float64) float64 {
	if h.Kind == model.KindAvoid {
		if d < today && d >= h.Start && !containsDay(h.Relapses, d) {
			return 1
		}
		return 0
	}
	v := 0.0
	if h.Measured() {
		v = totals[d]
	}
	if hasDone(h, d) && !h.GoalWeek {
		v = max(v, goalOf(h))
	}
	return v
}

func (s *Server) detailData(r *http.Request, id int64) (*detailData, error) {
	q := s.svc.Store.Q(r.Context())
	w, err := s.svc.World(q)
	if err != nil {
		return nil, err
	}
	h := service.Find(w, id)
	if h == nil {
		return nil, db.ErrNotFound
	}
	areas, err := q.Areas()
	if err != nil {
		return nil, err
	}
	xp, coins, err := q.HabitEarnings(id)
	if err != nil {
		return nil, err
	}
	totals, err := q.HabitDays(id)
	if err != nil {
		return nil, err
	}
	streak := h.StreakAt(w.Today)
	done, plan := planned(h, w.Today-29, lastSettledDay(h, w.Today))
	d := &detailData{
		Habit: h.Habit, Area: areaMap(areas)[h.AreaID], Frequency: view.FrequencyLabel(h.Habit), Since: sinceLabel(h.Start, w.Today),
		Streak: streak, Avoid: h.Kind == model.KindAvoid, Ring: ringPercent(streak.Days), Quote: percent(done, plan), XPTotal: xp, CoinsTotal: coins,
	}
	unit := amountUnitOf(h.Unit)
	if d.Avoid {
		d.Frequency = "vermeiden"
	} else if h.GoalWeek {
		d.Frequency = unit.label(h.Target) + " pro Woche"
	} else if h.Measured() {
		d.Frequency += " " + unit.label(h.Target)
	}
	if d.View, err = rememberedView(q, id, r.FormValue("view")); err != nil {
		return nil, err
	}
	switch d.View {
	case "week":
		if !h.Measured() {
			buildStrip(d, h, w.Today, totals)
		}
		buildWeek(d, h, w.Today, totals)
	case "year":
		if h.Measured() {
			buildYearAmount(d, h, unit, w.Today, totals)
		} else {
			buildYear(d, h, w.Today, totals)
		}
	default:
		d.View = "month"
		if !h.Measured() {
			buildStrip(d, h, w.Today, totals)
		}
		if err := buildMonth(d, h, w.Today, r.FormValue("m"), totals); err != nil {
			return nil, err
		}
	}
	states, err := s.svc.RewardStates(q, w)
	if err != nil {
		return nil, err
	}
	for _, st := range states {
		if st.Mode == model.RewardStreak && st.HabitID == id && !st.Archived && !(st.Redeemed && !st.Repeatable) {
			d.Rewards = append(d.Rewards, linkedReward{Title: st.Title, Progress: min(st.Progress, st.StreakTarget),
				Target: st.StreakTarget, Percent: percent(st.Progress, st.StreakTarget), Ready: st.Available})
		}
	}
	return d, nil
}

func buildStrip(d *detailData, h *game.HabitData, today model.Day, totals map[model.Day]float64) {
	for x := today.WeekStart(); x < today.WeekStart()+7; x++ {
		d.Strip = append(d.Strip, cellFor(h, x, today, totals))
	}
}

func buildWeek(d *detailData, h *game.HabitData, today model.Day, totals map[model.Day]float64) {
	start := today.WeekStart()
	goal := goalOf(h)
	unit := amountUnitOf(h.Unit)
	top := goal
	if h.GoalWeek { // a single day says nothing about the goal of the week
		top = h.Step
	}
	for x := start; x <= today; x++ {
		top = max(top, valueOn(h, x, today, totals))
	}
	sum, days, best, bestValue := 0.0, 0, "–", 0.0
	for i, name := range dayNames {
		x := start + model.Day(i)
		bar := barDay{Name: name, Label: "–", Today: x == today}
		switch {
		case x > today:
			bar.Future = true
		case x < h.Start:
			bar.Empty = true
		default:
			v := valueOn(h, x, today, totals)
			sum += v
			days++
			bar.Height, bar.Reached = max(int(v*100/top), 3), v >= goal || (h.GoalWeek && v > 0)
			switch {
			case h.Measured() && v > 0:
				bar.Label = unit.bare(v)
			case !h.Measured() && v > 0:
				bar.Label = "✓"
			}
			if v > bestValue {
				best, bestValue = name, v
			}
		}
		d.Chart.Bars = append(d.Chart.Bars, bar)
	}
	_, week := today.Time().ISOWeek()
	d.Chart.Title = fmt.Sprintf("KW %d · %s", week, rangeLabel(start, min(start+6, today)))
	if h.Measured() {
		if !h.GoalWeek {
			d.Chart.GoalAt = int(goal * 100 / top)
			d.Chart.GoalLabel = "Ziel " + unit.label(goal)
		}
		buildWeekAmount(d, h, unit, today, totals)
		return
	}
	d.Chart.Summary = fmt.Sprintf("%d von %d Tagen", int(sum), days)
	best = bestWeekday(h, today-364, today)
	done, plan := planned(h, start, lastSettledDay(h, today))
	d.Stats = []statBox{
		{Value: fmt.Sprintf("%d/%d", done, plan), Label: "Ziel erreicht", Class: "accent"},
		versusLastWeek(h, today, totals),
		{Value: best, Label: "stärkster Tag"},
	}
}

// versusLastWeek compares with the same days of the week before.
func versusLastWeek(h *game.HabitData, today model.Day, totals map[model.Day]float64) statBox {
	box := statBox{Value: "–", Label: "vs. Vorwoche"}
	now, before := 0.0, 0.0
	for x := today.WeekStart(); x <= today; x++ {
		now += valueOn(h, x, today, totals)
		before += valueOn(h, x-7, today, totals)
	}
	if before == 0 {
		return box
	}
	change := int(math.Round((now - before) * 100 / before))
	box.Value, box.Class = fmt.Sprintf("%+d%%", change), "green"
	if change < 0 {
		box.Class = "orange"
	}
	return box
}

func bestWeekday(h *game.HabitData, from, today model.Day) string {
	counts := make([]int, 7)
	best := "–"
	top := 0
	for x := max(from, h.Start); x <= today; x++ {
		done := hasDone(h, x)
		if h.Kind == model.KindAvoid {
			done = x != today && !containsDay(h.Relapses, x)
		}
		if !done {
			continue
		}
		i := x.WeekdayIndex()
		counts[i]++
		if counts[i] > top {
			top, best = counts[i], dayNames[i]
		}
	}
	return best
}

func buildMonth(d *detailData, h *game.HabitData, today model.Day, param string, totals map[model.Day]float64) error {
	unit := amountUnitOf(h.Unit)
	year, month := today.Year(), today.Month()
	if param != "" {
		t, err := time.Parse("2006-01", param)
		if err != nil {
			return badRequest("Ungültiger Monat")
		}
		year, month = t.Year(), t.Month()
	}
	first := model.NewDay(year, month, 1)
	last := model.NewDay(year, month+1, 1) - 1
	d.MonthParam = first.String()[:7]
	d.MonthTitle = fmt.Sprintf("%s %d", view.Months[month-1], year)
	d.PrevMonth = (first - 1).String()[:7]
	d.NextMonth = (last + 1).String()[:7]

	week := make([]dayCell, first.WeekdayIndex())
	for x := first; x <= last; x++ {
		cell := cellFor(h, x, today, totals)
		if v := valueOn(h, x, today, totals); h.Measured() && v > 0 {
			cell.Value = unit.bare(v)
		}
		week = append(week, cell)
		if len(week) == 7 {
			d.Weeks = append(d.Weeks, week)
			week = nil
		}
	}
	if len(week) > 0 {
		d.Weeks = append(d.Weeks, append(week, make([]dayCell, 7-len(week))...))
	}
	if h.Measured() {
		buildMonthAmount(d, h, unit, first, last, today, totals)
	}
	return nil
}

func heatFor(h *game.HabitData, x, today model.Day, totals map[model.Day]float64) heatCell {
	c := heatCell{Class: "miss", Title: x.String()}
	switch {
	case x > today:
		c.Class = "future"
	case h.Kind == model.KindAvoid:
		switch {
		case x < h.Start:
			c.Class = "pre"
		case containsDay(h.Relapses, x):
		case x == today:
			c.Class = "rest"
		default:
			c.Class = "done"
		}
	case hasDone(h, x):
		c.Class = "done"
	case x < h.Start:
		c.Class = "pre"
	case h.Joker[x]:
		c.Class = "joker"
	case h.Measured() && totals[x] > 0:
		c.Class = "part"
	case x == today || !h.Scheduled(x):
		c.Class = "rest"
	}
	return c
}

func buildYear(d *detailData, h *game.HabitData, today model.Day, totals map[model.Day]float64) {
	from := today - 364
	d.YearDone, _ = planned(h, from, lastSettledDay(h, today))
	if h.Start > from {
		d.StartHint = fmt.Sprintf("Grau = vor dem Start (%s) · zählt nicht zur Quote", dayMonth(h.Start))
	}
	start := today.WeekStart() - (heatmapWeeks-1)*7
	month := time.Month(0)
	labels := 0
	for week := range heatmapWeeks {
		col := heatColumn{}
		monday := start + model.Day(week*7)
		for offset := model.Day(0); offset < 7; offset++ {
			col.Cells = append(col.Cells, heatFor(h, monday+offset, today, totals))
		}
		if m := monday.Month(); m != month && week < heatmapWeeks-3 {
			if labels%2 == 0 {
				col.Month = string([]rune(view.Months[m-1])[:3])
			}
			month = m
			labels++
		}
		d.Heat = append(d.Heat, col)
	}
	buildMonthBars(d, h, today)
	d.Stats = []statBox{
		{Value: view.Number(d.Streak.Longest), Label: "Rekord-Serie", Class: "orange"},
		{Value: view.Number(len(d.Streak.Runs)), Label: "Serien gesamt"},
		totalStat(h, today),
	}
}

func buildMonthBars(d *detailData, h *game.HabitData, today model.Day) {
	settled := lastSettledDay(h, today)
	letters := []rune("JFMAMJJASOND")
	for i := 11; i >= 0; i-- {
		first := model.NewDay(today.Year(), today.Month()-time.Month(i), 1)
		last := model.NewDay(first.Year(), first.Month()+1, 1) - 1
		done, plan := planned(h, first, min(last, settled))
		bar := monthBar{Letter: string(letters[first.Month()-1]), Current: i == 0, Empty: plan == 0}
		if !bar.Empty {
			bar.Height = max(percent(done, plan), 3)
			bar.Label = fmt.Sprintf("%d%%", percent(done, plan))
		}
		d.MonthBars = append(d.MonthBars, bar)
	}
}

// totalStat sums up what the habit has brought so far.
func totalStat(h *game.HabitData, today model.Day) statBox {
	box := statBox{Label: "insgesamt", Class: "accent"}
	switch {
	case h.Measured() && view.IsMinutes(h.Unit):
		if h.Tag == "lesen" {
			box.Label = "gelesen"
		}
		box.Value = view.Number(int(h.Amount)/60) + " Std."
		if h.Amount < 60 {
			box.Value = view.Decimal(h.Amount) + " Min."
		}
	case h.Measured():
		box.Value, box.Label = view.Decimal(h.Amount), h.Unit+" gesamt"
	default:
		done, _ := planned(h, h.Start, lastSettledDay(h, today))
		box.Value, box.Label = view.Number(done), "Tage erledigt"
	}
	return box
}

func (s *Server) habitDetail(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	d, err := s.detailData(r, id)
	if err != nil {
		return err
	}
	return s.view.Render(w, http.StatusOK, "habit", s.page(r, d.Habit.Name, "", d))
}

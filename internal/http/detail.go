package server

import (
	"cmp"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/service"
	"github.com/seals187/habitd/internal/tips"
	"github.com/seals187/habitd/internal/view"
)

// Lengths of the year view in months.
const (
	spanShort = 6
	spanLong  = 12
)

// minHintSpan is how many free days the hint of a month needs beside the last day.
const minHintSpan = 3

// dayCell is one day of the week strip or the month grid.
type dayCell struct {
	Label    int
	Letter   string
	Date     string
	State    string // done, part, joker, missed, open or none
	Fill     int    // filled height of a partly reached day, in percent
	Value    string // amount of the day, for habits with a unit
	Time     string // when it was reached, as far as it is known
	Today    bool
	Editable bool
	Sheet    bool // a tap opens the sheet of the day
}

// barDay is one bar of the week chart.
type barDay struct {
	Name        string
	Label       string
	Height      int
	Reached     bool
	Today       bool
	Future      bool
	Empty       bool   // before the habit started
	Status      string // how the day went, such as ✓18:40
	StatusClass string
}

type weekChart struct {
	Title   string
	Summary string
	Bars    []barDay
	GoalAt  int  // height of the goal line in percent, 0 hides it
	Timed   bool // the bars say how their day went
}

// patternData is the box with the hints of a habit.
type patternData struct {
	Label string
	Hours []int // level per hour from 6 to 24, nil hides the strip
	Tips  []tips.Tip
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

	HintSpan   int    // free days after the last one that hold the hint, 0 keeps it below
	Prev       string // link to the period before
	Next       string // link to the period after, empty for the running one
	WeekParam  string
	MonthParam string
	MonthTitle string
	Weeks      [][]dayCell

	Chart weekChart

	YearDone  int
	YearParam string
	YearTitle string
	HeatTitle string
	Span      int
	Heat      []heatColumn
	StartHint string
	MonthBars []monthBar

	Amount *amountData // set for habits with a unit, replaces the streak

	Patterns     *patternData
	PatternsOpen bool
	Stats        []statBox
	Rewards      []linkedReward
	Zoom         int // size of the page in percent, per device
}

// Scale is the size of the page as a CSS factor.
func (d *detailData) Scale() float64 { return float64(d.Zoom) / 100 }

// patternsOpen reads whether this device keeps the hints of a habit open.
func patternsOpen(r *http.Request) bool {
	c, err := r.Cookie("patterns_open")
	return err == nil && c.Value == "1"
}

var dayNames = []string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"}

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

// monthsLabel formats whole months like Mai–Okt 2026 or, short for the head of the
// card, Dez 25–Nov 26.
func monthsLabel(first, last model.Day) string {
	from, to := monthName(first.Month(), spanShort), monthName(last.Month(), spanShort)
	if first.Year() != last.Year() {
		return fmt.Sprintf("%s %02d–%s %02d", from, first.Year()%100, to, last.Year()%100)
	}
	return fmt.Sprintf("%s–%s %d", from, to, last.Year())
}

// periodLink opens a view of a habit at another period.
func periodLink(id int64, view, key, value string) string {
	return fmt.Sprintf("/habits/%d?view=%s&%s=%s", id, view, key, value)
}

// yearSpan is the length of the year view: asked for by a link or kept on this device.
func yearSpan(r *http.Request) int {
	span := r.FormValue("span")
	if c, err := r.Cookie("year_span"); span == "" && err == nil {
		span = c.Value
	}
	if span == strconv.Itoa(spanLong) {
		return spanLong
	}
	return spanShort
}

// weekOf finds the Monday of the asked week. The view ends at the running week.
func weekOf(param string, today model.Day) (model.Day, error) {
	if param == "" {
		return today.WeekStart(), nil
	}
	d, err := model.ParseDay(param)
	if err != nil {
		return 0, badRequest("Ungültige Woche")
	}
	return min(d, today).WeekStart(), nil
}

// yearEnd finds the last month of the year view, at most the running one.
func yearEnd(param string, today model.Day) (model.Day, error) {
	running := model.NewDay(today.Year(), today.Month(), 1)
	if param == "" {
		return running, nil
	}
	t, err := time.Parse("2006-01", param)
	if err != nil {
		return 0, badRequest("Ungültiger Zeitraum")
	}
	return min(model.NewDay(t.Year(), t.Month(), 1), running), nil
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
	c := dayCell{Label: d.Dom(), Letter: dayNames[d.WeekdayIndex()], Date: d.String(), Today: d == today}
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
	h.Start = h.Since() // a day filled in before the habit was created belongs to it
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
		Zoom: zoomOf(r, "detail_zoom"), PatternsOpen: patternsOpen(r),
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
		start, err := weekOf(r.FormValue("w"), w.Today)
		if err != nil {
			return nil, err
		}
		if !h.Measured() {
			buildStrip(d, h, start, w.Today, totals)
		}
		var clicks []model.Entry
		if h.GoalWeek {
			if clicks, err = q.HabitEntries(h.ID, start); err != nil {
				return nil, err
			}
		}
		buildWeek(d, h, start, w.Today, totals, lastClicks(clicks))
	case "year":
		end, err := yearEnd(r.FormValue("y"), w.Today)
		if err != nil {
			return nil, err
		}
		first := setYearWindow(d, h.ID, end, yearSpan(r), w.Today)
		last := model.NewDay(end.Year(), end.Month()+1, 1) - 1
		if h.Measured() {
			buildYearAmount(d, h, unit, first, last, w.Today, totals)
		} else {
			buildYear(d, h, first, last, w.Today, totals)
		}
	default:
		d.View = "month"
		if !h.Measured() {
			buildStrip(d, h, w.Today.WeekStart(), w.Today, totals)
		}
		var clicks []model.Entry
		if h.Measured() {
			if clicks, err = q.HabitEntries(h.ID, h.Start); err != nil {
				return nil, err
			}
		}
		if err := buildMonth(d, h, w.Today, r.FormValue("m"), totals, lastClicks(clicks)); err != nil {
			return nil, err
		}
	}
	if d.Patterns, err = s.patterns(q, h, streak, w.Today); err != nil {
		return nil, err
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

// realMoment tells a time of day from a day filled in afterwards, which is stored
// at noon of that day.
func realMoment(at time.Time, d model.Day) bool {
	const noon = "12:00:00"
	return model.DayOf(at, at.Location()) == d && at.Format("15:04:05") != noon
}

// patterns collects what really happened lately and asks the rules about it.
func (s *Server) patterns(q *db.Q, h *game.HabitData, streak game.Streak, today model.Day) (*patternData, error) {
	unit := amountUnitOf(h.Unit)
	from := today - tips.Window
	in := tips.Input{Habit: h, Streak: streak, Today: today, Now: s.svc.Now(), Format: unit.label, Drink: unit.kind == unitVolume}
	for day, at := range h.Done {
		if day >= from && realMoment(at, day) {
			in.Done = append(in.Done, tips.Stamp{Day: day, At: at, Amount: 1})
		}
	}
	slices.SortFunc(in.Done, func(a, b tips.Stamp) int { return cmp.Compare(a.Day, b.Day) })
	in.Stamps = in.Done
	if h.Measured() {
		entries, err := q.HabitEntries(h.ID, from)
		if err != nil {
			return nil, err
		}
		in.Stamps = nil
		for _, e := range entries {
			if realMoment(e.CreatedAt, e.Date) {
				in.Stamps = append(in.Stamps, tips.Stamp{Day: e.Date, At: e.CreatedAt, Amount: e.Amount})
			}
		}
	}
	p := &patternData{Label: "wann du es erledigst", Hours: tips.Hours(in), Tips: tips.For(in)}
	if in.Drink {
		p.Label = "wann du trinkst"
	}
	if len(p.Tips) == 0 && p.Hours == nil {
		return nil, nil
	}
	return p, nil
}

// barStatus says how a day of the week went: when the goal was reached, or why not.
func barStatus(h *game.HabitData, x, today model.Day) (string, string) {
	at, done := h.Done[x]
	switch {
	case done && realMoment(at, x):
		return "✓" + at.Format("15:04"), "done"
	case done:
		return "✓", "done"
	case h.Joker[x]:
		return "Joker", "joker"
	case x == today:
		return "läuft", ""
	case h.Scheduled(x):
		return "verpasst", "missed"
	}
	return "", ""
}

// lastClicks finds the last click of each day.
func lastClicks(entries []model.Entry) map[model.Day]time.Time {
	out := map[model.Day]time.Time{}
	for _, e := range entries {
		out[e.Date] = e.CreatedAt
	}
	return out
}

// weekGoalStatus is barStatus for a goal that adds up over the week: every day
// with a click counts towards it, and a day without one is not missed.
func weekGoalStatus(h *game.HabitData, x, today model.Day, clicks map[model.Day]time.Time) (string, string) {
	at, clicked := clicks[x]
	_, reached := h.GoalCheckin(today)
	switch {
	case clicked && realMoment(at, x):
		return "✓" + at.Format("15:04"), "done"
	case clicked:
		return "✓", "done"
	case x == today && !reached:
		return "läuft", ""
	}
	return "", ""
}

func buildStrip(d *detailData, h *game.HabitData, start, today model.Day, totals map[model.Day]float64) {
	for x := start; x < start+7; x++ {
		cell := cellFor(h, x, today, totals)
		cell.Sheet = service.DayEditable(h, x, today)
		d.Strip = append(d.Strip, cell)
	}
}

func buildWeek(d *detailData, h *game.HabitData, start, today model.Day, totals map[model.Day]float64, clicks map[model.Day]time.Time) {
	d.WeekParam = start.String()
	d.Prev = periodLink(h.ID, "week", "w", (start - 7).String())
	if start < today.WeekStart() {
		d.Next = periodLink(h.ID, "week", "w", (start + 7).String())
	}
	end := min(start+6, today)
	goal := goalOf(h)
	unit := amountUnitOf(h.Unit)
	top := goal
	if h.GoalWeek { // a single day says nothing about the goal of the week
		top = h.Step
	}
	for x := start; x <= end; x++ {
		top = max(top, valueOn(h, x, today, totals))
	}
	sum, days, best, bestValue := 0.0, 0, "–", 0.0
	d.Chart.Timed = h.Kind != model.KindAvoid
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
				bar.Label = unit.tag(v)
			case !h.Measured() && v > 0:
				bar.Label = "✓"
			}
			if v > bestValue {
				best, bestValue = name, v
			}
			if d.Chart.Timed {
				if h.GoalWeek {
					bar.Status, bar.StatusClass = weekGoalStatus(h, x, today, clicks)
				} else {
					bar.Status, bar.StatusClass = barStatus(h, x, today)
				}
			}
		}
		if d.Chart.Timed && !h.Measured() { // a tick says it all with its time
			bar.Label = ""
		}
		d.Chart.Bars = append(d.Chart.Bars, bar)
	}
	year, week := start.Time().ISOWeek()
	d.Chart.Title = fmt.Sprintf("KW %d · %s", week, rangeLabel(start, end))
	if year != today.Year() {
		d.Chart.Title = fmt.Sprintf("KW %d/%d · %s", week, year, rangeLabel(start, end))
	}
	if h.Measured() {
		if !h.GoalWeek {
			d.Chart.GoalAt = int(goal * 100 / top)
		}
		buildWeekAmount(d, h, unit, start, today, totals)
		return
	}
	d.Chart.Summary = fmt.Sprintf("%d von %d Tagen", int(sum), days)
	best = bestWeekday(h, today-364, today)
	done, plan := planned(h, start, min(end, lastSettledDay(h, today)))
	d.Stats = []statBox{
		{Value: fmt.Sprintf("%d/%d", done, plan), Label: "Ziel erreicht", Class: "accent"},
		versusLastWeek(h, start, today, totals),
		{Value: best, Label: "stärkster Tag"},
	}
}

// versusLastWeek compares with the same days of the week before.
func versusLastWeek(h *game.HabitData, start, today model.Day, totals map[model.Day]float64) statBox {
	box := statBox{Value: "–", Label: "vs. Vorwoche"}
	now, before := 0.0, 0.0
	for x := start; x <= min(start+6, today); x++ {
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

// reachedAt is the time of a day for the calendar, as far as it is known: when its
// goal was reached, else its last click. That covers days below the goal, and the
// days of a goal of the week, which have no reached day of their own.
func reachedAt(h *game.HabitData, x model.Day, clicks map[model.Day]time.Time) string {
	if at, done := h.Done[x]; done && !h.GoalWeek && realMoment(at, x) {
		return at.Format("15:04")
	}
	if at, clicked := clicks[x]; clicked && realMoment(at, x) {
		return at.Format("15:04")
	}
	return ""
}

func buildMonth(d *detailData, h *game.HabitData, today model.Day, param string, totals map[model.Day]float64, clicks map[model.Day]time.Time) error {
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
	d.Prev = periodLink(h.ID, "month", "m", (first - 1).String()[:7])
	if last < today {
		d.Next = periodLink(h.ID, "month", "m", (last + 1).String()[:7])
	}

	week := make([]dayCell, first.WeekdayIndex())
	for x := first; x <= last; x++ {
		cell := cellFor(h, x, today, totals)
		cell.Sheet = service.DayEditable(h, x, today)
		cell.Time = reachedAt(h, x, clicks)
		if v := valueOn(h, x, today, totals); h.Measured() && v > 0 {
			cell.Value = unit.tag(v)
		}
		week = append(week, cell)
		if len(week) == 7 {
			d.Weeks = append(d.Weeks, week)
			week = nil
		}
	}
	if left := 7 - len(week); left >= minHintSpan && left < 7 && h.Measured() {
		d.HintSpan = left // the hint moves into the free days, which saves its line
		d.Weeks = append(d.Weeks, week)
	} else if left < 7 {
		d.Weeks = append(d.Weeks, append(week, make([]dayCell, left)...))
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

// setYearWindow fills the head of the year view and returns its first day.
func setYearWindow(d *detailData, id int64, end model.Day, span int, today model.Day) model.Day {
	first := model.NewDay(end.Year(), end.Month()-time.Month(span-1), 1)
	last := model.NewDay(end.Year(), end.Month()+1, 1) - 1
	running := model.NewDay(today.Year(), today.Month(), 1)
	d.Span, d.YearParam = span, end.String()[:7]
	d.YearTitle = monthsLabel(first, last)
	d.HeatTitle = d.YearTitle
	if end == running {
		d.HeatTitle = fmt.Sprintf("Letzte %d Monate", span)
	}
	d.Prev = periodLink(id, "year", "y", model.NewDay(end.Year(), end.Month()-time.Month(span), 1).String()[:7])
	if end < running {
		next := min(model.NewDay(end.Year(), end.Month()+time.Month(span), 1), running)
		d.Next = periodLink(id, "year", "y", next.String()[:7])
	}
	return first
}

func buildYear(d *detailData, h *game.HabitData, first, last, today model.Day, totals map[model.Day]float64) {
	d.YearDone, _ = planned(h, first, min(last, lastSettledDay(h, today)))
	if h.Start > first {
		d.StartHint = fmt.Sprintf("Grau = vor dem Start (%s) · zählt nicht zur Quote", dayMonth(h.Start))
	}
	start := first.WeekStart()
	weeks := int(min(last, today).WeekStart()-start)/7 + 1
	every := d.Span / spanShort // a long window only has room for every other name
	month := time.Month(0)
	labels := 0
	for week := range weeks {
		col := heatColumn{}
		monday := start + model.Day(week*7)
		for offset := model.Day(0); offset < 7; offset++ {
			col.Cells = append(col.Cells, heatFor(h, monday+offset, today, totals))
		}
		if m := (monday + 6).Month(); m != month && week < weeks-3 {
			if labels%every == 0 {
				col.Month = monthName(m, spanShort)
			}
			month = m
			labels++
		}
		d.Heat = append(d.Heat, col)
	}
	buildMonthBars(d, h, first, today)
	d.Stats = []statBox{
		{Value: view.Number(d.Streak.Longest), Label: "Rekord-Serie", Class: "orange"},
		{Value: view.Number(len(d.Streak.Runs)), Label: "Serien gesamt"},
		totalStat(h, today),
	}
}

// monthName is the name of a month on its bar: one letter when there are many bars.
func monthName(m time.Month, bars int) string {
	name := []rune(view.Months[m-1])
	if bars > maxNamedBars {
		return string(name[:1])
	}
	return string(name[:3])
}

func buildMonthBars(d *detailData, h *game.HabitData, from, today model.Day) {
	settled := lastSettledDay(h, today)
	for i := range d.Span {
		first := model.NewDay(from.Year(), from.Month()+time.Month(i), 1)
		last := model.NewDay(first.Year(), first.Month()+1, 1) - 1
		done, plan := planned(h, first, min(last, settled))
		bar := monthBar{Letter: monthName(first.Month(), d.Span), Current: first <= today && today <= last, Empty: plan == 0}
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

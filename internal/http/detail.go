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

// cellStates are the CSS classes of the days in the strip and the month.
var cellStates = map[game.DayStatus]string{
	game.DayFree: "none", game.DayBefore: "none", game.DayFuture: "open", game.DayOpen: "open",
	game.DayDone: "done", game.DayPart: "part", game.DayJoker: "joker", game.DayMissed: "missed",
}

// cellFor shows one day of the strip or the month.
func cellFor(h *game.HabitData, d, today model.Day) dayCell {
	m := h.Mark(d, today)
	c := dayCell{Label: d.Dom(), Letter: view.Weekdays[d.WeekdayIndex()], Date: d.String(), Today: d == today, State: cellStates[m.Status]}
	if m.Status == game.DayPart {
		c.Fill = m.Fill
	}
	// An avoid habit cannot have relapsed before it began.
	c.Editable = d <= today && d >= today-game.BackfillDays && !h.Archived && !(h.Kind == model.KindAvoid && m.Status == game.DayBefore)
	return c
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

func (s *Server) detailData(r *http.Request, id int64) (*detailData, error) {
	q := s.svc.Store.Q(r.Context())
	w, err := s.svc.World(q)
	if err != nil {
		return nil, err
	}
	found := service.Find(w, id)
	if found == nil {
		return nil, db.ErrNotFound
	}
	h := found.ForStatistics()
	areas, err := q.Areas()
	if err != nil {
		return nil, err
	}
	xp, coins, err := q.HabitEarnings(id)
	if err != nil {
		return nil, err
	}
	streak := h.StreakAt(w.Today)
	done, plan := h.Quota(w.Today-29, w.Today, w.Today)
	d := &detailData{
		Habit: h.Habit, Area: areaMap(areas)[h.AreaID], Frequency: view.RhythmLabel(h.Habit), Since: sinceLabel(h.Start, w.Today),
		Streak: streak, Avoid: h.Kind == model.KindAvoid, Ring: ringPercent(streak.Days), Quote: percent(done, plan), XPTotal: xp, CoinsTotal: coins,
		Zoom: zoomOf(r, "detail_zoom"), PatternsOpen: patternsOpen(r),
	}
	unit := view.UnitOf(h.Unit)
	if d.Avoid {
		d.Frequency = "vermeiden"
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
			buildStrip(d, h, start, w.Today)
		}
		var clicks []model.Entry
		if h.GoalWeek {
			if clicks, err = q.HabitEntries(h.ID, start); err != nil {
				return nil, err
			}
		}
		buildWeek(d, h, start, w.Today, lastClicks(clicks))
	case "year":
		end, err := yearEnd(r.FormValue("y"), w.Today)
		if err != nil {
			return nil, err
		}
		first := setYearWindow(d, h.ID, end, yearSpan(r), w.Today)
		last := model.NewDay(end.Year(), end.Month()+1, 1) - 1
		if h.Measured() {
			buildYearAmount(d, h, unit, first, last, w.Today)
		} else {
			buildYear(d, h, first, last, w.Today)
		}
	default:
		d.View = "month"
		if !h.Measured() {
			buildStrip(d, h, w.Today.WeekStart(), w.Today)
		}
		var clicks []model.Entry
		if h.Measured() {
			if clicks, err = q.HabitEntries(h.ID, h.Start); err != nil {
				return nil, err
			}
		}
		if err := buildMonth(d, h, w.Today, r.FormValue("m"), lastClicks(clicks)); err != nil {
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
	unit := view.UnitOf(h.Unit)
	from := today - tips.Window
	in := tips.Input{Habit: h, Streak: streak, Today: today, Now: s.svc.Now()}
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
	if unit.Kind == view.UnitVolume {
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

func buildStrip(d *detailData, h *game.HabitData, start, today model.Day) {
	for x := start; x < start+7; x++ {
		cell := cellFor(h, x, today)
		cell.Sheet = service.DayEditable(h, x, today)
		d.Strip = append(d.Strip, cell)
	}
}

func buildWeek(d *detailData, h *game.HabitData, start, today model.Day, clicks map[model.Day]time.Time) {
	d.WeekParam = start.String()
	d.Prev = periodLink(h.ID, "week", "w", (start - 7).String())
	if start < today.WeekStart() {
		d.Next = periodLink(h.ID, "week", "w", (start + 7).String())
	}
	end := min(start+6, today)
	goal := h.GoalValue()
	unit := view.UnitOf(h.Unit)
	top := goal
	if h.GoalWeek { // a single day says nothing about the goal of the week
		top = h.Step
	}
	for x := start; x <= end; x++ {
		top = max(top, h.Value(x, today))
	}
	sum, days, best, bestValue := 0.0, 0, "–", 0.0
	d.Chart.Timed = h.Kind != model.KindAvoid
	for i, name := range view.Weekdays {
		x := start + model.Day(i)
		bar := barDay{Name: name, Label: "–", Today: x == today}
		switch {
		case x > today:
			bar.Future = true
		case x < h.Start:
			bar.Empty = true
		default:
			v := h.Value(x, today)
			sum += v
			days++
			bar.Height, bar.Reached = max(int(v*100/top), 3), v >= goal || (h.GoalWeek && v > 0)
			switch {
			case h.Measured() && v > 0:
				bar.Label = unit.Tag(v)
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
		buildWeekAmount(d, h, unit, start, today)
		return
	}
	d.Chart.Summary = fmt.Sprintf("%d von %d Tagen", int(sum), days)
	best = bestWeekday(h, today-364, today)
	d.Stats = []statBox{
		{Value: goalsLabel(h, start, today), Label: "Ziel erreicht", Class: "accent"},
		versusLastWeek(h, start, today),
		{Value: best, Label: "stärkster Tag"},
	}
}

// versusLastWeek compares with the same days of the week before.
func versusLastWeek(h *game.HabitData, start, today model.Day) statBox {
	box := statBox{Value: "–", Label: "vs. Vorwoche"}
	now, before := 0.0, 0.0
	for x := start; x <= min(start+6, today); x++ {
		now += h.Value(x, today)
		before += h.Value(x-7, today)
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
	if i := h.BestWeekday(from, today); i >= 0 {
		return view.Weekdays[i]
	}
	return "–"
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

func buildMonth(d *detailData, h *game.HabitData, today model.Day, param string, clicks map[model.Day]time.Time) error {
	unit := view.UnitOf(h.Unit)
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
		cell := cellFor(h, x, today)
		cell.Sheet = service.DayEditable(h, x, today)
		cell.Time = reachedAt(h, x, clicks)
		if v := h.Value(x, today); h.Measured() && v > 0 {
			cell.Value = unit.Tag(v)
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
		buildMonthAmount(d, h, unit, first, last, today)
	}
	return nil
}

// heatClasses are the CSS classes of the days in the year view, which shows
// nothing that is undecided yet.
var heatClasses = map[game.DayStatus]string{
	game.DayFree: "rest", game.DayBefore: "pre", game.DayFuture: "future", game.DayOpen: "rest",
	game.DayDone: "done", game.DayPart: "part", game.DayJoker: "joker", game.DayMissed: "miss",
}

func heatFor(h *game.HabitData, x, today model.Day) heatCell {
	m := h.Mark(x, today)
	c := heatCell{Class: heatClasses[m.Status], Title: x.String()}
	if m.Checkin && m.Status != game.DayFuture { // the day a weekly goal was reached
		c.Class = "done"
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

func buildYear(d *detailData, h *game.HabitData, first, last, today model.Day) {
	d.YearDone = h.Achieved(first, min(last, h.Settled(today)))
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
			col.Cells = append(col.Cells, heatFor(h, monday+offset, today))
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
	for i := range d.Span {
		first := model.NewDay(from.Year(), from.Month()+time.Month(i), 1)
		last := model.NewDay(first.Year(), first.Month()+1, 1) - 1
		done, plan := h.MonthQuota(first, today)
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
		box.Value, box.Label = view.Number(h.Achieved(h.Start, h.Settled(today))), "Tage erledigt"
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

// goalsLabel is the progress of a week: the goals met so far of all its goals.
func goalsLabel(h *game.HabitData, start, today model.Day) string {
	met, planned := h.Goals(start, start+6, today)
	if planned == 0 {
		return "–"
	}
	return fmt.Sprintf("%d/%d", met, planned)
}

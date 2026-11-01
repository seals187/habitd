package server

import (
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/view"
)

// The tabs of the level page and the ranges of its statistics.
const (
	tabStats   = "stats"
	tabPath    = "path"
	tabHistory = "history"

	rangeToday = "today"
	rangeWeek  = "week"
	rangeMonth = "month"
	rangeAll   = "all"
)

// levelTab is one tab of the level page.
type levelTab struct {
	Key   string
	Label string
}

// levelTabs are the three views, also linked from the level card.
var levelTabs = []levelTab{{tabStats, "Statistik"}, {tabPath, "Levelweg"}, {tabHistory, "Verlauf"}}

var levelRanges = []levelTab{{rangeToday, "Heute"}, {rangeWeek, "Woche"}, {rangeMonth, "Monat"}, {rangeAll, "Gesamt"}}

type levelData struct {
	Tab     string
	Tabs    []levelTab
	Stats   *levelStats
	Path    *levelPath
	History *levelHistory
}

// levelNext is the next level on the level card.
type levelNext struct {
	Level int
	Title string
	Line  string
}

// xpRate is the mean XP per day of the last days, counted from the first day on.
func xpRate(q *db.Q, w *game.World) (int, error) {
	from := max(w.Today-estimateDays+1, w.Installed)
	byDay, err := q.XPByDay(from, w.Today)
	if err != nil {
		return 0, err
	}
	sum := 0
	for _, xp := range byDay {
		sum += xp
	}
	return max(sum, 0) / max(int(w.Today-from)+1, 1), nil
}

// etaDays says how many days the missing XP take at the given pace, 0 if unknown.
func etaDays(missing, rate int) int {
	if rate <= 0 {
		return 0
	}
	return int(math.Ceil(float64(missing) / float64(rate)))
}

// dayCount writes a number of days after "ca.".
func dayCount(days int, dative bool) string {
	switch {
	case days == 1:
		return "1 Tag"
	case dative:
		return strconv.Itoa(days) + " Tagen"
	}
	return strconv.Itoa(days) + " Tage"
}

// levelReward words what a level gives. A badge only counts while badges are on.
func levelReward(r *game.Rules, row game.Level) string {
	var parts []string
	if row.Coins != 0 {
		parts = append(parts, fmt.Sprintf("+%s Taler", view.Number(row.Coins)))
	}
	if row.Jokers != 0 {
		parts = append(parts, fmt.Sprintf("+%d Joker", row.Jokers))
	}
	if b, ok := r.BadgeByKey(row.BadgeKey); ok && !r.BadgesOff {
		parts = append(parts, "Abzeichen "+b.Name)
	}
	return strings.Join(parts, " · ")
}

// newLevelNext describes the next level, nil once the last level is reached.
func newLevelNext(r *game.Rules, p game.Progress, rate int) *levelNext {
	if p.Cost == 0 {
		return nil
	}
	row := r.Level(p.Level + 1)
	parts := []string{}
	if reward := levelReward(r, row); reward != "" {
		parts = append(parts, reward)
	}
	if days := etaDays(p.Missing, rate); days > 0 {
		parts = append(parts, "bei deinem Tempo in ca. "+dayCount(days, true))
	}
	return &levelNext{Level: p.Level + 1, Title: row.Title, Line: strings.Join(parts, " · ")}
}

func (s *Server) level(w http.ResponseWriter, r *http.Request) error {
	if err := s.svc.EnsureQuests(r.Context()); err != nil {
		return err
	}
	q := s.svc.Store.Q(r.Context())
	world, err := s.svc.World(q)
	if err != nil {
		return err
	}
	d := levelData{Tab: r.URL.Query().Get("tab"), Tabs: levelTabs}
	switch d.Tab {
	case tabPath:
		d.Path, err = s.levelPath(r, q, world)
	case tabHistory:
		d.History, err = s.levelHistory(q, world, r.URL.Query().Get("day"))
	default:
		d.Tab = tabStats
		d.Stats, err = s.levelStats(q, world, r.URL.Query().Get("range"))
	}
	if err != nil {
		return err
	}
	return s.view.Render(w, http.StatusOK, "level", s.page(r, "Mein Level", "today", d))
}

// statRange is the span of the statistics and the span it is compared with.
type statRange struct {
	Key      string
	From, To model.Day
	Before   [2]model.Day // empty for the whole time
	Label    string
	Versus   string
}

func firstOfMonth(d model.Day) model.Day { return d - model.Day(d.Dom()-1) }

func newStatRange(key string, today, first model.Day) statRange {
	switch key {
	case rangeToday:
		return statRange{Key: key, From: today, To: today, Before: [2]model.Day{today - 1, today - 1}, Label: "heute", Versus: "vs. gestern"}
	case rangeMonth:
		from := firstOfMonth(today)
		last := firstOfMonth(from - 1)
		return statRange{Key: key, From: from, To: today, Before: [2]model.Day{last, min(last+today-from, from-1)}, Label: "diesen Monat", Versus: "vs. Vormonat"}
	case rangeAll:
		return statRange{Key: key, From: first, To: today, Label: "gesamt"}
	}
	from := today.WeekStart()
	return statRange{Key: rangeWeek, From: from, To: today, Before: [2]model.Day{from - 7, today - 7}, Label: "diese Woche", Versus: "vs. Vorwoche"}
}

// statDelta is a comparison below a figure.
type statDelta struct {
	Dir  string // up, down or flat
	Text string
}

// newDelta compares in percent. Without a difference there is nothing to show.
func newDelta(diff int, text string) *statDelta {
	dir := "up"
	switch {
	case diff == 0:
		return nil
	case diff < 0:
		dir, diff = "down", -diff
	}
	return &statDelta{Dir: dir, Text: strings.TrimSpace(fmt.Sprintf("%d %% %s", diff, text))}
}

// xpBar is one bar of the XP chart.
type xpBar struct {
	Label   string
	Name    string
	Height  int
	Current bool
	Future  bool
}

// areaXP is one row of the XP per area.
type areaXP struct {
	model.Area
	XP    int
	Width int
}

type levelStats struct {
	Range   statRange
	Ranges  []levelTab
	XP      int
	XPDelta *statDelta
	Quote   string
	QuoteUp *statDelta
	Coins   int
	Sheet   *streakSheet
	Heading string
	Bars    []xpBar
	Dense   bool // many bars without their values
	Areas   []areaXP
}

// quotaOf sums the goals of all habits. Today alone also counts what is still
// open, a longer range leaves it out until it is missed.
func quotaOf(w *game.World, from, to model.Day) (done, planned int) {
	for _, h := range w.Habits {
		var d, p int
		if from == to {
			d, p = h.DayGoal(from)
		} else {
			d, p = h.Quota(from, to, w.Today)
		}
		done, planned = done+d, planned+p
	}
	return done, planned
}

func sumRange(byDay map[string]int, from, to model.Day) int {
	sum := 0
	for d := from; d <= to; d++ {
		sum += byDay[d.String()]
	}
	return sum
}

func scaleBars(bars []xpBar, values []int) {
	top := 0
	for _, v := range values {
		top = max(top, v)
	}
	for i, v := range values {
		if v > 0 {
			bars[i].Height = max(v*100/top, 5)
		}
	}
}

// weekBars shows the seven days of the week of today.
func weekBars(byDay map[string]int, today model.Day) []xpBar {
	bars, values := make([]xpBar, 7), make([]int, 7)
	for i := range bars {
		d := today.WeekStart() + model.Day(i)
		values[i] = max(byDay[d.String()], 0)
		bars[i] = xpBar{Name: weekNames[i], Current: d == today, Future: d > today}
		if !bars[i].Future {
			bars[i].Label = view.Number(values[i])
		}
	}
	scaleBars(bars, values)
	return bars
}

// monthBars shows the days of the month of today. Only every seventh is named.
func monthBars(byDay map[string]int, today model.Day) []xpBar {
	from := firstOfMonth(today)
	end := firstOfMonth(from+31) - 1
	var bars []xpBar
	var values []int
	for d := from; d <= end; d++ {
		bar := xpBar{Current: d == today, Future: d > today}
		if d.Dom()%7 == 1 {
			bar.Name = strconv.Itoa(d.Dom())
		}
		bars, values = append(bars, bar), append(values, max(byDay[d.String()], 0))
	}
	scaleBars(bars, values)
	return bars
}

// yearBars shows the last twelve months.
func yearBars(byDay map[string]int, today model.Day) []xpBar {
	months := make([]model.Day, 12)
	m := firstOfMonth(today)
	for i := 11; i >= 0; i-- {
		months[i] = m
		m = firstOfMonth(m - 1)
	}
	bars, values := make([]xpBar, 12), make([]int, 12)
	for i, start := range months {
		end := firstOfMonth(start+31) - 1
		values[i] = max(sumRange(byDay, start, end), 0)
		bars[i] = xpBar{Name: view.Months[start.Month()-1][:1], Label: view.Number(values[i]), Current: start == firstOfMonth(today)}
	}
	scaleBars(bars, values)
	return bars
}

func areaRows(areas []model.Area, habits []*game.HabitData, byHabit map[int64]int) []areaXP {
	sums := map[int64]int{}
	for _, h := range habits {
		sums[h.AreaID] += byHabit[h.ID]
	}
	var rows []areaXP
	top := 0
	for _, a := range areas {
		if sums[a.ID] > 0 {
			rows = append(rows, areaXP{Area: a, XP: sums[a.ID]})
			top = max(top, sums[a.ID])
		}
	}
	for i := range rows {
		rows[i].Width = max(rows[i].XP*100/top, 3)
	}
	slices.SortStableFunc(rows, func(a, b areaXP) int { return b.XP - a.XP })
	return rows
}

// firstDay is the earliest day any habit counts from.
func firstDay(w *game.World) model.Day {
	first := w.Installed
	for _, h := range w.Habits {
		first = min(first, h.Start)
	}
	return first
}

func quoteLabel(done, planned int) string {
	if planned == 0 {
		return "–"
	}
	return fmt.Sprintf("%d %%", percent(done, planned))
}

func (s *Server) levelStats(q *db.Q, w *game.World, key string) (*levelStats, error) {
	rng := newStatRange(key, w.Today, firstDay(w))
	d := &levelStats{Range: rng, Ranges: levelRanges, Sheet: s.streakSheet(w), Heading: "XP pro Tag"}
	byDay, err := q.XPByDay(firstOfMonth(w.Today-366), w.Today)
	if err != nil {
		return nil, err
	}
	done, planned := quotaOf(w, rng.From, rng.To)
	d.Quote = quoteLabel(done, planned)
	if rng.Key == rangeAll {
		d.XP, d.Coins = w.Balances.XP, w.Balances.CoinsEarned
	} else {
		d.XP = sumRange(byDay, rng.From, rng.To)
		if d.Coins, err = q.EarnedCoins(rng.From, rng.To); err != nil {
			return nil, err
		}
		if before := sumRange(byDay, rng.Before[0], rng.Before[1]); before > 0 {
			d.XPDelta = newDelta((d.XP-before)*100/before, rng.Versus)
		}
		if oldDone, oldPlanned := quotaOf(w, rng.Before[0], rng.Before[1]); oldPlanned > 0 && planned > 0 {
			d.QuoteUp = newDelta(percent(done, planned)-percent(oldDone, oldPlanned), "")
		}
	}
	switch rng.Key {
	case rangeMonth:
		d.Bars, d.Dense = monthBars(byDay, w.Today), true
	case rangeAll:
		d.Bars, d.Heading = yearBars(byDay, w.Today), "XP pro Monat"
	default:
		d.Bars = weekBars(byDay, w.Today)
	}
	byHabit, err := q.XPByHabit(rng.From, rng.To)
	if err != nil {
		return nil, err
	}
	areas, err := q.Areas()
	if err != nil {
		return nil, err
	}
	d.Areas = areaRows(areas, w.Habits, byHabit)
	return d, nil
}

// pathStep is a level on the way.
type pathStep struct {
	Level   int
	Title   string
	Sub     string
	XP      int
	State   string // done, now or next
	Percent int
	Into    int
	Cost    int
	ETA     string
}

type levelPath struct {
	Level  int
	Title  string
	XP     int
	Rate   int
	Figure figure
	Stage  int
	Steps  []pathStep
}

// isMilestone reports whether a level is worth a stop on the way: a new title or
// a badge. The levels around the current one always are.
func isMilestone(r *game.Rules, level, current int) bool {
	row := r.Level(level)
	return level == 1 || level == current || level == current+1 || row.BadgeKey != "" && !r.BadgesOff || row.Title != r.Title(level-1)
}

// pathSteps lists the stops of the level way.
func pathSteps(r *game.Rules, p game.Progress, rate int) []pathStep {
	last := len(r.Levels)
	if r.Settings.Limit {
		last = min(last, r.Settings.MaxLevel)
	}
	var steps []pathStep
	for level := 1; level <= max(last, p.Level); level++ {
		if !isMilestone(r, level, p.Level) {
			continue
		}
		row := r.Level(level)
		step := pathStep{Level: level, Title: row.Title, XP: r.LevelXP(level), Sub: levelReward(r, row), State: "next"}
		if level == 1 {
			step.Sub = "Start"
		}
		switch {
		case level < p.Level:
			step.State = "done"
		case level == p.Level:
			step.State, step.Percent, step.Into, step.Cost = "now", p.Percent, p.Into, p.Cost
			if days := etaDays(p.Missing, rate); days > 0 && p.Cost > 0 {
				step.ETA = "noch ca. " + dayCount(days, false)
			}
		}
		steps = append(steps, step)
	}
	return steps
}

func (s *Server) levelPath(r *http.Request, q *db.Q, w *game.World) (*levelPath, error) {
	today, err := s.todayData(r)
	if err != nil {
		return nil, err
	}
	rate, err := xpRate(q, w)
	if err != nil {
		return nil, err
	}
	rules := s.svc.Rules()
	return &levelPath{Level: today.Level.Level, Title: today.Title, XP: w.Balances.XP, Rate: rate, Figure: today.Figure, Stage: today.Stage,
		Steps: pathSteps(rules, today.Level, rate)}, nil
}

// historyDay is one day of the strip of the history.
type historyDay struct {
	Date    string
	Name    string
	Dom     int
	State   string // full, part or empty
	Current bool
}

// historyEvent is one line of the history of a day.
type historyEvent struct {
	Time   string
	Title  string
	Tag    string
	Color  string
	Detail string
	Open   bool // a step on the way, which draws a hollow mark
	XP     int
	Coins  int
	Jokers int
	at     time.Time
}

type levelHistory struct {
	Days    []historyDay
	Date    string
	IsToday bool
	XP      int
	Coins   int
	Done    int
	Planned int
	Events  []historyEvent
}

func stripState(done, planned int) string {
	switch {
	case planned > 0 && done >= planned:
		return "full"
	case done > 0:
		return "part"
	}
	return "empty"
}

// clockOn formats the time of an event. Something entered later has no time on
// the day it counts for.
func clockOn(at time.Time, d model.Day, loc *time.Location) (string, time.Time) {
	if model.DayOf(at, loc) != d {
		return "", time.Time{}
	}
	return at.In(loc).Format("15:04"), at
}

// goalLine says what a check-in reached.
func goalLine(h *game.HabitData, d model.Day) string {
	if h.Measured() {
		return "Ziel erreicht · " + amountUnitOf(h.Unit).label(h.GoalTotal(d))
	}
	if h.Frequency == model.FreqWeekly || h.Frequency == model.FreqMonthly {
		n := 0
		start, span := d.WeekStart(), "diese Woche"
		if h.Frequency == model.FreqMonthly {
			start, span = firstOfMonth(d), "diesen Monat"
		}
		for x := start; x <= d; x++ {
			if _, ok := h.Done[x]; ok {
				n++
			}
		}
		return fmt.Sprintf("%d / %d %s", n, h.WeekTarget(), span)
	}
	return "Ziel erreicht"
}

// entryEvents turns the clicks of counters into lines. The click that reaches the
// goal is left out, the check-in stands for it.
func entryEvents(entries []model.Entry, byID map[int64]*game.HabitData, areas map[int64]model.Area, d model.Day, loc *time.Location) []historyEvent {
	sums := map[int64]float64{}
	var out []historyEvent
	for _, e := range entries {
		h := byID[e.HabitID]
		if h == nil {
			continue
		}
		if _, seen := sums[h.ID]; !seen && h.GoalWeek {
			sums[h.ID] = h.GoalTotal(d) - h.Entries[d]
		}
		before := sums[h.ID]
		sums[h.ID] = model.RoundAmount(before + e.Amount)
		_, done := h.Done[d]
		if done && before < h.Target && sums[h.ID] >= h.Target {
			continue
		}
		u := amountUnitOf(h.Unit)
		clock, at := clockOn(e.CreatedAt, d, loc)
		out = append(out, historyEvent{Time: clock, at: at, Title: h.Name, Tag: areas[h.AreaID].Name, Color: areas[h.AreaID].Color, Open: true,
			Detail: fmt.Sprintf("+%s · %s / %s", u.step(e.Amount), u.value(sums[h.ID]), u.step(h.Target))})
	}
	return out
}

// ledgerEvents adds what the bookings of a day tell beyond the check-ins.
func ledgerEvents(rules *game.Rules, groups []db.LedgerGroup, byID map[int64]*game.HabitData, areas map[int64]model.Area, d model.Day, loc *time.Location) ([]historyEvent, map[string]db.LedgerGroup) {
	byRef := map[string]db.LedgerGroup{}
	var out []historyEvent
	for _, g := range groups {
		byRef[g.Ref] = g
		if g.XP == 0 && g.Coins == 0 && g.Jokers == 0 {
			continue
		}
		kind, rest, _ := strings.Cut(g.Ref, ":")
		ev := historyEvent{XP: g.XP, Coins: g.Coins, Jokers: g.Jokers}
		ev.Time, ev.at = clockOn(g.At, d, loc)
		switch kind {
		case "quest":
			ev.Title, ev.Tag, ev.Color, ev.Detail = "Tagesquest", "Quest", "#f2c14e", "geschafft"
		case "level":
			level, _ := strconv.Atoi(rest)
			ev.Title, ev.Tag, ev.Color, ev.Detail = fmt.Sprintf("Level %d erreicht", level), "Level", "#a9aaf5", rules.Title(level)
		case "badge":
			b, ok := rules.BadgeByKey(rest)
			if !ok {
				continue
			}
			ev.Title, ev.Tag, ev.Color, ev.Detail = b.Name, "Abzeichen", "#f2994a", b.Hint
		case "avoid":
			id, _, _ := strings.Cut(rest, ":")
			habit, _ := strconv.ParseInt(id, 10, 64)
			h := byID[habit]
			if h == nil {
				continue
			}
			ev.Title, ev.Tag, ev.Color, ev.Detail = h.Name, areas[h.AreaID].Name, areas[h.AreaID].Color, "Tag geschafft"
			ev.Time, ev.at = "", time.Time{}
		default:
			continue
		}
		out = append(out, ev)
	}
	return out, byRef
}

// dayEvents builds the timeline of a day, newest first. What has no time comes last.
func (s *Server) dayEvents(q *db.Q, w *game.World, d model.Day) ([]historyEvent, error) {
	areaList, err := q.Areas()
	if err != nil {
		return nil, err
	}
	areas := areaMap(areaList)
	byID := make(map[int64]*game.HabitData, len(w.Habits))
	for _, h := range w.Habits {
		byID[h.ID] = h
	}
	groups, err := q.DayLedger(d)
	if err != nil {
		return nil, err
	}
	events, byRef := ledgerEvents(s.svc.Rules(), groups, byID, areas, d, s.svc.Loc)
	for _, h := range w.Habits {
		at, done := h.Done[d]
		if !done {
			continue
		}
		g := byRef[fmt.Sprintf("checkin:%d:%s", h.ID, d)]
		ev := historyEvent{Title: h.Name, Tag: areas[h.AreaID].Name, Color: areas[h.AreaID].Color, Detail: goalLine(h, d), XP: g.XP, Coins: g.Coins, Jokers: g.Jokers}
		ev.Time, ev.at = clockOn(at, d, s.svc.Loc)
		events = append(events, ev)
	}
	entries, err := q.DayEntries(d)
	if err != nil {
		return nil, err
	}
	events = append(events, entryEvents(entries, byID, areas, d, s.svc.Loc)...)
	slices.SortStableFunc(events, func(a, b historyEvent) int { return b.at.Compare(a.at) })
	return events, nil
}

func (s *Server) levelHistory(q *db.Q, w *game.World, param string) (*levelHistory, error) {
	day := w.Today
	if d, err := model.ParseDay(param); err == nil && d <= w.Today && d > w.Today-7 {
		day = d
	}
	h := &levelHistory{Date: view.FullDate(day), IsToday: day == w.Today}
	for d := w.Today - 6; d <= w.Today; d++ {
		done, planned := quotaOf(w, d, d)
		h.Days = append(h.Days, historyDay{Date: d.String(), Name: weekNames[d.WeekdayIndex()], Dom: d.Dom(), State: stripState(done, planned), Current: d == day})
	}
	byDay, err := q.XPByDay(day, day)
	if err != nil {
		return nil, err
	}
	h.XP = byDay[day.String()]
	if h.Coins, err = q.EarnedCoins(day, day); err != nil {
		return nil, err
	}
	h.Done, h.Planned = quotaOf(w, day, day)
	if h.Events, err = s.dayEvents(q, w, day); err != nil {
		return nil, err
	}
	return h, nil
}

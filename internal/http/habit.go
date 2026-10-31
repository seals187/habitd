package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/service"
	"github.com/seals187/habitd/internal/templates"
	"github.com/seals187/habitd/internal/view"
)

const heatmapWeeks = 53

// maxGoal limits the daily goal of counter and quantity habits.
const maxGoal = 1_000_000

// dayCell is one day in the week or month view.
type dayCell struct {
	Label    int
	Letter   string
	Date     string
	State    string // done, joker, missed, open or none
	Today    bool
	Editable bool
}

type heatCell struct {
	Level int // 0 to 3, -1 outside the range
	Title string
}

type heatColumn struct {
	Month string
	Cells []heatCell
}

type streakBar struct {
	Label string
	Value int
	Left  int
	Width int
	Class string
}

type weekdayBar struct {
	Label  string
	Height int
	Best   bool
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
	Kind       string
	Frequency  string
	View       string
	Streak     game.Streak
	Avoid      bool
	XPTotal    int
	CoinsTotal int
	Multiplier float64
	NextBonus  string

	MonthParam string
	MonthTitle string
	PrevMonth  string
	NextMonth  string
	Weeks      [][]dayCell
	DoneDays   int
	PlanDays   int
	Quote      int

	Week []dayCell

	YearDone    int
	YearQuote   int
	Heat        []heatColumn
	StreakBars  []streakBar
	WeekdayBars []weekdayBar

	Rewards []linkedReward
}

var dayLetters = []string{"M", "D", "M", "D", "F", "S", "S"}

// cellFor derives the display state of one day.
func cellFor(h *game.HabitData, d, today model.Day) dayCell {
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
	case hasDone(h, d):
		c.State = "done"
	case h.Joker[d]:
		c.State = "joker"
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
	streak := h.StreakAt(w.Today)
	d := &detailData{
		Habit: h.Habit, Area: areaMap(areas)[h.AreaID], Kind: "Aufbauen", Frequency: view.FrequencyLabel(h.Habit),
		View: r.FormValue("view"), Streak: streak, Avoid: h.Kind == model.KindAvoid, XPTotal: xp, CoinsTotal: coins,
		Multiplier: game.Multiplier(streak.Days),
	}
	if d.Avoid {
		d.Kind, d.Frequency = "Vermeiden", "täglich"
	}
	switch {
	case streak.Days < 7:
		d.NextBonus = fmt.Sprintf("×1,5 ab 7 Tagen, noch %d", 7-streak.Days)
	case streak.Days < 30:
		d.NextBonus = fmt.Sprintf("×2 ab 30 Tagen, noch %d", 30-streak.Days)
	default:
		d.NextBonus = "höchster Bonus erreicht"
	}
	switch d.View {
	case "week":
		for x := w.Today.WeekStart(); x < w.Today.WeekStart()+7; x++ {
			d.Week = append(d.Week, cellFor(h, x, w.Today))
		}
	case "year":
		buildYear(d, h, w.Today)
	default:
		d.View = "month"
		if err := buildMonth(d, h, w.Today, r.FormValue("m")); err != nil {
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

func buildMonth(d *detailData, h *game.HabitData, today model.Day, param string) error {
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
		week = append(week, cellFor(h, x, today))
		if len(week) == 7 {
			d.Weeks = append(d.Weeks, week)
			week = nil
		}
	}
	if len(week) > 0 {
		d.Weeks = append(d.Weeks, append(week, make([]dayCell, 7-len(week))...))
	}
	d.DoneDays, d.PlanDays = planned(h, first, min(last, today-1))
	d.Quote = percent(d.DoneDays, d.PlanDays)
	return nil
}

func buildYear(d *detailData, h *game.HabitData, today model.Day) {
	from := today - 364
	d.YearDone, _ = planned(h, from, today)
	_, plan := planned(h, from, today)
	d.YearQuote = percent(d.YearDone, plan)

	age := map[model.Day]int{}
	for _, run := range d.Streak.Runs {
		for x := run.Start; x <= run.End; x++ {
			age[x] = int(x-run.Start) + 1
		}
	}
	start := today.WeekStart() - (heatmapWeeks-1)*7
	month := time.Month(0)
	for week := 0; week < heatmapWeeks; week++ {
		col := heatColumn{}
		monday := start + model.Day(week*7)
		for offset := model.Day(0); offset < 7; offset++ {
			col.Cells = append(col.Cells, heatFor(h, monday+offset, today, age))
		}
		if m := monday.Month(); m != month && week < heatmapWeeks-1 {
			if month != 0 {
				col.Month = string([]rune(view.Months[m-1])[:3])
			}
			month = m
		}
		d.Heat = append(d.Heat, col)
	}
	d.StreakBars = streakBars(d.Streak, h, today)
	d.WeekdayBars = weekdayBars(h, from, today)
}

func heatFor(h *game.HabitData, x, today model.Day, age map[model.Day]int) heatCell {
	c := heatCell{Title: x.String()}
	switch {
	case x > today:
		c.Level = -1
	case h.Kind == model.KindAvoid && (x < h.Start || x == today || containsDay(h.Relapses, x)):
	case h.Kind != model.KindAvoid && !hasDone(h, x):
	case age[x] >= 10:
		c.Level = 3
	case age[x] >= 3:
		c.Level = 2
	default:
		c.Level = 1
	}
	return c
}

// streakBars shows where the current, the record and the earlier streak sit on
// a timeline of the habit's lifetime.
func streakBars(s game.Streak, h *game.HabitData, today model.Day) []streakBar {
	if len(s.Runs) == 0 {
		return nil
	}
	first := max(s.Runs[0].Start, today-364)
	span := max(int(today-first)+1, 1)
	bar := func(label, class string, r game.Run) streakBar {
		left := max(int(r.Start-first), 0)
		width := max(int(r.End-r.Start)+1, 1)
		return streakBar{Label: label, Value: r.Length, Left: left * 100 / span,
			Width: max(min(width*100/span, 100-left*100/span), 6), Class: class}
	}
	var out []streakBar
	current := -1
	if s.Current > 0 {
		current = len(s.Runs) - 1
		out = append(out, bar("Aktuell", "tl-current", s.Runs[current]))
	}
	record := -1
	for i, r := range s.Runs {
		if i != current && (record < 0 || r.Length >= s.Runs[record].Length) {
			record = i
		}
	}
	if record < 0 {
		return out
	}
	out = append(out, bar("Rekord", "tl-record", s.Runs[record]))
	before := -1
	for i, r := range s.Runs[:record] {
		if before < 0 || r.Length >= s.Runs[before].Length {
			before = i
		}
	}
	if before >= 0 {
		out = append(out, bar("Davor", "tl-before", s.Runs[before]))
	}
	return out
}

func weekdayBars(h *game.HabitData, from, today model.Day) []weekdayBar {
	counts := make([]int, 7)
	best := 0
	for x := max(from, h.Start); x <= today; x++ {
		done := hasDone(h, x)
		if h.Kind == model.KindAvoid {
			done = x != today && !containsDay(h.Relapses, x)
		}
		if done {
			counts[x.WeekdayIndex()]++
			best = max(best, counts[x.WeekdayIndex()])
		}
	}
	labels := []string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"}
	out := make([]weekdayBar, 7)
	for i, c := range counts {
		out[i] = weekdayBar{Label: labels[i], Height: max(percent(c, best), 4), Best: best > 0 && c == best}
	}
	return out
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

// iconChoice is an icon offered in the habit form. Icons of specific habits
// carry the tag some badges look for.
type iconChoice struct {
	Name string
	Tag  string
}

var habitIcons = []iconChoice{
	{"drop", "wasser"}, {"book", "lesen"}, {"dumb", "sport"}, {"lotus", "meditation"}, {"handheart", ""}, {"bulb", "idee"},
	{"run", "sport"}, {"bike", "velo"}, {"heart", ""}, {"phone", "anrufen"}, {"users", ""}, {"brief", ""},
	{"meal", ""}, {"moon", "schlaf"}, {"sun", "frueh"}, {"leaf", ""}, {"cup", ""}, {"ban", ""},
	{"eye", ""}, {"phoneoff", "handy"}, {"cartx", "impulskauf"}, {"wallet", ""}, {"recycle", ""}, {"target", ""},
	{"flame", ""}, {"handshake", ""}, {"cake", "geburtstag"}, {"home", ""}, {"teach", ""},
}

type dayOption struct {
	Bit     int
	Label   string
	Checked bool
}

type habitFormData struct {
	Habit    model.Habit
	New      bool
	Areas    []model.Area
	Icons    []iconChoice
	Days     []dayOption
	Freqs    []freqOption
	Expanded bool // all fields visible; a new habit starts with name and suggestions only
	Popular  []popularChip
	Total    int
	Error    string
	Config   string // JSON read by the Alpine preview
}

func (s *Server) habitForm(w http.ResponseWriter, r *http.Request, h model.Habit, status int, message string) error {
	areas, err := s.svc.Store.Q(r.Context()).Areas()
	if err != nil {
		return err
	}
	measured := h.Measured()
	if h.Measure == "" {
		h = h.Normalized()
	}
	d := habitFormData{Habit: h, New: h.ID == 0, Areas: areas, Icons: habitIcons, Error: message, Freqs: freqOptions(h), Popular: popularRows(areas), Total: len(templates.All())}
	d.Expanded = !d.New || status != http.StatusOK || h.Name != ""
	for i, l := range []string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"} {
		d.Days = append(d.Days, dayOption{Bit: i, Label: l, Checked: h.DaysMask&(1<<i) != 0})
	}
	target, step := "8", "1"
	if measured {
		target, step = strconv.Itoa(h.Target), strconv.Itoa(h.Step)
	}
	config, err := json.Marshal(map[string]any{"kind": h.Kind, "difficulty": h.Difficulty, "measure": h.Measure,
		"target": target, "step": step, "edited": measured})
	if err != nil {
		return err
	}
	d.Config = string(config)
	title := "Habit bearbeiten"
	if d.New {
		title = "Neues Habit"
	}
	return s.view.Render(w, status, "habit_form", s.page(r, title, "", d))
}

func (s *Server) habitNew(w http.ResponseWriter, r *http.Request) error {
	areas, err := s.svc.Store.Q(r.Context()).Areas()
	if err != nil {
		return err
	}
	h := model.Habit{Kind: model.KindBuild, Frequency: model.FreqDaily, Difficulty: model.Easy, Icon: habitIcons[0].Name}
	if len(areas) > 0 {
		h.AreaID = areas[0].ID
	}
	if t, ok := templates.Find(r.FormValue("template")); ok {
		if area, ok := areasByKey(areas)[t.Area]; ok {
			h = t.Habit(area.ID, s.svc.Now())
		}
	}
	if name := strings.TrimSpace(r.FormValue("name")); name != "" && h.TemplateID == "" {
		h.Name = name
	}
	return s.habitForm(w, r, h, http.StatusOK, "")
}

func (s *Server) habitEdit(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	h, err := s.svc.Store.Q(r.Context()).Habit(id)
	if err != nil {
		return err
	}
	return s.habitForm(w, r, h, http.StatusOK, "")
}

func tagForIcon(icon string) (string, bool) {
	for _, c := range habitIcons {
		if c.Name == icon {
			return c.Tag, true
		}
	}
	return "", false
}

func parseHabitForm(r *http.Request, h *model.Habit) string {
	h.Name = strings.TrimSpace(r.FormValue("name"))
	h.Kind = r.FormValue("kind")
	h.Difficulty = r.FormValue("difficulty")
	h.Icon = r.FormValue("icon")
	h.AreaID, _ = strconv.ParseInt(r.FormValue("area"), 10, 64)
	h.Frequency, h.WeeklyTarget, h.DaysMask = model.FreqDaily, 0, 0
	switch r.FormValue("freq") {
	case "w1", "w2", "w3", "w4", "w5", "w6":
		h.Frequency, h.WeeklyTarget = model.FreqWeekly, int(r.FormValue("freq")[1]-'0')
	case "m1", "m2", "m3", "m4":
		h.Frequency, h.WeeklyTarget = model.FreqMonthly, int(r.FormValue("freq")[1]-'0')
	case "custom":
		h.Frequency = model.FreqCustom
		for _, v := range r.Form["days"] {
			if bit, err := strconv.Atoi(v); err == nil && bit >= 0 && bit < 7 {
				h.DaysMask |= 1 << bit
			}
		}
	}
	if h.Kind != model.KindAvoid {
		h.Kind = model.KindBuild
	} else {
		h.Frequency, h.WeeklyTarget, h.DaysMask = model.FreqDaily, 0, 0
	}
	if h.Difficulty != model.Medium && h.Difficulty != model.Hard {
		h.Difficulty = model.Easy
	}
	h.Measure, h.Unit = r.FormValue("measure"), strings.TrimSpace(r.FormValue("unit"))
	if h.Kind == model.KindAvoid {
		h.Measure = model.MeasureCheck
	}
	h.Target, _ = strconv.Atoi(r.FormValue("target"))
	h.Step, _ = strconv.Atoi(r.FormValue("step"))
	*h = h.Normalized()
	tag, ok := tagForIcon(h.Icon)
	if !ok {
		h.Icon = habitIcons[0].Name
		tag = habitIcons[0].Tag
	}
	h.Tag = tag
	if h.ID == 0 {
		h.TemplateID = r.FormValue("template")
	}
	if t, ok := templates.Find(h.TemplateID); ok {
		h.Tag = t.Tag // keeps area-specific badges working
	} else {
		h.TemplateID = ""
	}
	switch {
	case h.Name == "":
		return "Bitte gib einen Namen ein"
	case len(h.Name) > 80:
		return "Der Name ist zu lang"
	case h.Frequency == model.FreqCustom && h.DaysMask == 0:
		return "Bitte wähle mindestens einen Wochentag"
	case h.Target < 1 || h.Target > maxGoal:
		return "Das Tagesziel muss zwischen 1 und 1.000.000 liegen"
	case h.Step < 1 || h.Step > h.Target:
		return "Pro Klick muss zwischen 1 und dem Tagesziel liegen"
	case utf8.RuneCountInString(h.Unit) > 16:
		return "Die Einheit ist zu lang"
	}
	return ""
}

func (s *Server) habitSave(w http.ResponseWriter, r *http.Request) error {
	var h model.Habit
	q := s.svc.Store.Q(r.Context())
	if r.PathValue("id") != "" {
		id, err := pathID(r)
		if err != nil {
			return err
		}
		if h, err = q.Habit(id); err != nil {
			return err
		}
	} else {
		h.CreatedAt = s.svc.Now()
	}
	if err := r.ParseForm(); err != nil {
		return badRequest("Ungültiges Formular")
	}
	if message := parseHabitForm(r, &h); message != "" {
		return s.habitForm(w, r, h, http.StatusUnprocessableEntity, message)
	}
	areas, err := q.Areas()
	if err != nil {
		return err
	}
	if _, ok := areaMap(areas)[h.AreaID]; !ok {
		return s.habitForm(w, r, h, http.StatusUnprocessableEntity, "Bitte wähle einen Bereich")
	}
	id, err := q.SaveHabit(h)
	if err != nil {
		return err
	}
	if err := s.svc.HabitSaved(r.Context()); err != nil {
		return err
	}
	if h.ID != 0 {
		return redirect(w, r, fmt.Sprintf("/habits/%d", id))
	}
	return redirect(w, r, "/")
}

type habitRow struct {
	model.Habit
	Area      model.Area
	Frequency string
}

func (s *Server) habitList(w http.ResponseWriter, r *http.Request) error {
	q := s.svc.Store.Q(r.Context())
	habits, err := q.Habits()
	if err != nil {
		return err
	}
	areas, err := q.Areas()
	if err != nil {
		return err
	}
	byID := areaMap(areas)
	var active, archived []habitRow
	for _, h := range habits {
		row := habitRow{Habit: h, Area: byID[h.AreaID], Frequency: view.FrequencyLabel(h)}
		if h.Archived {
			archived = append(archived, row)
		} else {
			active = append(active, row)
		}
	}
	return s.view.Render(w, http.StatusOK, "habits", s.page(r, "Habits verwalten", "profile", map[string]any{"Active": active, "Archived": archived}))
}

func (s *Server) habitArchive(w http.ResponseWriter, r *http.Request) error {
	return s.setArchived(w, r, true)
}

func (s *Server) habitRestore(w http.ResponseWriter, r *http.Request) error {
	return s.setArchived(w, r, false)
}

func (s *Server) setArchived(w http.ResponseWriter, r *http.Request, archived bool) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.svc.Store.Q(r.Context()).SetHabitArchived(id, archived); err != nil {
		return err
	}
	if err := s.svc.HabitSaved(r.Context()); err != nil {
		return err
	}
	return redirect(w, r, "/habits")
}

func (s *Server) habitDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.svc.DeleteHabit(r.Context(), id); err != nil {
		return err
	}
	return redirect(w, r, "/habits")
}

func (s *Server) habitDeleteAll(w http.ResponseWriter, r *http.Request) error {
	if err := s.svc.DeleteAllHabits(r.Context()); err != nil {
		return err
	}
	return redirect(w, r, "/habits")
}

func (s *Server) habitMove(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	err = s.svc.Store.Tx(r.Context(), func(q *db.Q) error { return q.MoveHabit(id, r.FormValue("dir") == "up") })
	if err != nil {
		return err
	}
	return redirect(w, r, "/habits")
}

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

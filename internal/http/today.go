package server

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/service"
	"github.com/seals187/habitd/internal/view"
)

// habitCard is a row on the Today screen.
type habitCard struct {
	ID          int64
	Name        string
	Area        model.Area
	Done        bool
	Collapsed   bool   // folded by hand or reached
	Auto        bool   // reached today or for this period, folds by itself
	Rest        string // next planned day of a habit that is off today
	Relapsed    bool
	Streak      game.Streak
	StreakLabel string
	XP          int
	Ring        int    // fill of the ring in percent
	RingLabel   string // text in the ring, empty shows an icon
	Meter       *meter // set for counter and quantity habits
	Week        []weekDay
	Reached     int    // days reached this week
	Goal        int    // days planned this week, 0 for monthly rhythms
	Count       string // progress of the week beside the strip
	Of          string // what the week asks for, empty for monthly rhythms
}

// meter is the state of a counter or quantity habit on its card.
type meter struct {
	RingLabel string
	AddLabel  string
	Percent   int
	CanUndo   bool
}

func newMeter(h model.Habit, total float64, done bool) *meter {
	value := total
	if done { // e.g. ticked before the habit became a counter
		value = max(value, h.Target)
	}
	m := &meter{AddLabel: "+" + view.Decimal(h.Step), RingLabel: view.Decimal(value), CanUndo: total > 0,
		Percent: min(int(value*100/h.Target), 100)}
	switch {
	case view.IsMinutes(h.Unit):
		m.AddLabel += "′"
		m.RingLabel += "′"
	case h.Measure == model.MeasureCount:
		m.RingLabel += "/" + view.Decimal(h.Target)
	}
	return m
}

// weekDay is one bar of the week strip on a card.
type weekDay struct {
	Letter string
	State  string // CSS class: done, part, open, missed, rest or future
	Fill   int    // filled height in percent
	Today  bool
}

var weekLetters = [7]string{"M", "D", "M", "D", "F", "S", "S"}
var weekNames = [7]string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"}

// dayTotals holds the entry sums of the days of a week.
type dayTotals map[model.Day]map[int64]float64

func (s *Server) weekTotals(q *db.Q, today model.Day) (dayTotals, error) {
	out := dayTotals{}
	for d := today.WeekStart(); d <= today; d++ {
		totals, err := q.DayTotals(d)
		if err != nil {
			return nil, err
		}
		out[d] = totals
	}
	return out, nil
}

// weekStrip shows the week of today for a habit and counts the reached days.
func weekStrip(h *game.HabitData, today model.Day, totals dayTotals) ([]weekDay, int) {
	days := make([]weekDay, 0, len(weekLetters))
	reached := 0
	for i, letter := range weekLetters {
		d := today.WeekStart() + model.Day(i)
		day := weekDay{Letter: letter, Today: d == today, State: dayState(h, d, today, totals[d][h.ID])}
		switch day.State {
		case "done":
			day.Fill = 100
			reached++
		case "joker":
			day.Fill = 100
		case "part":
			day.Fill = min(int(totals[d][h.ID]*100/h.Target), 99)
		}
		days = append(days, day)
	}
	return days, reached
}

func dayState(h *game.HabitData, d, today model.Day, total float64) string {
	if _, done := h.Done[d]; done && !h.GoalWeek {
		return "done"
	}
	switch {
	case h.Joker[d]:
		return "joker"
	case d > today || d < h.Start:
		return "future"
	case h.Kind == model.KindAvoid:
		if slices.Contains(h.Relapses, d) {
			return "missed"
		}
		return "done"
	case h.Measured() && total > 0:
		return "part"
	case d == today:
		return "open"
	case h.Scheduled(d):
		return "missed"
	}
	return "rest"
}

// weekGoal is the number of days a habit is planned for in a week.
func weekGoal(h *game.HabitData) int {
	switch {
	case h.Kind == model.KindAvoid || h.Frequency == model.FreqDaily:
		return 7
	case h.Frequency == model.FreqMonthly:
		return 0
	}
	return h.WeekTarget()
}

// nextPlanned names the next weekday a habit with fixed days is planned for.
func nextPlanned(h *game.HabitData, today model.Day) string {
	for d := today + 1; d < today+7; d++ {
		if h.Scheduled(d) {
			return "wieder am " + weekNames[d.WeekdayIndex()]
		}
	}
	return "nächste Woche"
}

func streakLabel(s game.Streak) string {
	switch {
	case s.Monthly:
		return fmt.Sprintf("%d Mo", s.Current)
	case s.Weekly:
		return fmt.Sprintf("%d Wo", s.Current)
	}
	return view.Number(s.Current)
}

type bridgeOffer struct {
	HabitID int64
	Habit   string
	Date    string
	Label   string
}

// levelSegments is the length of the level bar on the Today screen.
const levelSegments = 12

// todayReward is a reward that wants to show up on the Today screen.
type todayReward struct {
	ID    int64
	Title string
	Icon  string
	Color string
	Price int
	Mode  string
}

type todayData struct {
	Date     string
	Bal      model.Balances
	Level    game.Progress
	Title    string
	Segments []bool
	Quests   []questCard
	Build    []habitCard
	Avoid    []habitCard
	Rest     []habitCard // habits with fixed days that are off today
	Bridges  []bridgeOffer
	Treats   []todayReward
	Undo     *undoToast
}

// fillToday adds the level card and the rewards to the Today screen.
func (s *Server) fillToday(q *db.Q, d *todayData, w *game.World) error {
	rules := s.svc.Rules()
	d.Level = rules.ProgressFor(w.Balances.XP, w.Level)
	d.Title = rules.Title(d.Level.Level)
	filled := (d.Level.Percent*levelSegments + 99) / 100
	for i := range levelSegments {
		d.Segments = append(d.Segments, i < filled)
	}
	var err error
	if d.Quests, err = s.questCards(q, w); err != nil {
		return err
	}
	states, err := s.svc.RewardStates(q, w)
	if err != nil {
		return err
	}
	for _, st := range states {
		if st.ShowToday && st.Available && !st.Archived && !(st.Redeemed && !st.Repeatable) {
			d.Treats = append(d.Treats, todayReward{ID: st.ID, Title: st.Title, Icon: st.Icon, Color: rewardColor(st.Icon), Price: st.Price, Mode: st.Mode})
		}
	}
	return nil
}

func areaMap(areas []model.Area) map[int64]model.Area {
	out := make(map[int64]model.Area, len(areas))
	for _, a := range areas {
		out[a.ID] = a
	}
	return out
}

func (s *Server) todayData(r *http.Request) (*todayData, error) {
	if err := s.svc.EnsureQuests(r.Context()); err != nil {
		return nil, err
	}
	q := s.svc.Store.Q(r.Context())
	w, err := s.svc.World(q)
	if err != nil {
		return nil, err
	}
	areas, err := q.Areas()
	if err != nil {
		return nil, err
	}
	totals, err := s.weekTotals(q, w.Today)
	if err != nil {
		return nil, err
	}
	byID := areaMap(areas)
	_, kw := w.Today.Time().ISOWeek()
	d := &todayData{Date: fmt.Sprintf("%s · KW %d", view.LongDate(w.Today), kw), Bal: w.Balances}
	for _, h := range w.Habits {
		if h.Archived || w.Today < h.Start {
			continue
		}
		card := habitCard{ID: h.ID, Name: h.Name, Area: byID[h.AreaID], Streak: h.StreakAt(w.Today), Goal: weekGoal(h), Collapsed: h.Collapsed}
		card.StreakLabel, card.XP = streakLabel(card.Streak), game.BaseXP(h.Habit)
		card.Week, card.Reached = weekStrip(h, w.Today, totals)
		card.Count = strconv.Itoa(card.Reached)
		if card.Goal > 0 {
			card.Of = strconv.Itoa(card.Goal)
		}
		if h.GoalWeek {
			card.Count, card.Of = view.Decimal(h.GoalTotal(w.Today)), view.Decimal(h.Target)
		}
		if h.Kind == model.KindAvoid {
			for _, relapse := range h.Relapses {
				card.Relapsed = card.Relapsed || relapse == w.Today
			}
			if card.Relapsed {
				card.Ring = 0
			} else {
				card.Ring, card.RingLabel = 100, view.Number(card.Streak.Days)
			}
			d.Avoid = append(d.Avoid, card)
			continue
		}
		_, card.Done = h.Done[w.Today]
		if h.Measured() {
			_, reached := h.GoalCheckin(w.Today)
			card.Meter = newMeter(h.Habit, h.GoalTotal(w.Today), reached)
			card.Meter.CanUndo = h.Entries[w.Today] > 0
			card.Ring, card.RingLabel = card.Meter.Percent, card.Meter.RingLabel
		} else if card.Done {
			card.Ring = 100
		}
		periodMet := (h.Frequency == model.FreqWeekly || h.Frequency == model.FreqMonthly) && !h.DueOn(w.Today)
		if periodMet {
			card.Ring = 100
		}
		card.Auto = card.Done || periodMet
		card.Collapsed = card.Collapsed || card.Auto
		if h.Frequency == model.FreqCustom && !h.Scheduled(w.Today) && !card.Done {
			card.Rest, card.Collapsed = nextPlanned(h, w.Today), true
			d.Rest = append(d.Rest, card)
			continue
		}
		d.Build = append(d.Build, card)
	}
	if err := s.fillToday(q, d, w); err != nil {
		return nil, err
	}
	mode, err := q.Setting(service.KeyJokerMode, service.JokerAuto)
	if err != nil {
		return nil, err
	}
	if mode == service.JokerManual && w.Balances.Jokers > 0 {
		for _, b := range service.Bridgeable(w) {
			d.Bridges = append(d.Bridges, bridgeOffer{HabitID: b.Habit.ID, Habit: b.Habit.Name, Date: b.Day.String(), Label: view.LongDate(b.Day)})
		}
	}
	return d, nil
}

func (s *Server) today(w http.ResponseWriter, r *http.Request) error {
	fresh, err := s.needsOnboarding(r.Context())
	if err != nil {
		return err
	}
	if fresh {
		return redirect(w, r, "/onboarding")
	}
	d, err := s.todayData(r)
	if err != nil {
		return err
	}
	return s.view.Render(w, http.StatusOK, "today", s.page(r, "Heute", "today", d))
}

// habitFold switches a card on the Today screen between its two sizes.
func (s *Server) habitFold(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	q := s.svc.Store.Q(r.Context())
	h, err := q.Habit(id)
	if err != nil {
		return err
	}
	if err := q.SetHabitCollapsed(id, !h.Collapsed); err != nil {
		return err
	}
	return s.afterHabitChange(w, r, id)
}

// afterHabitChange answers a check-in: a pending celebration wins, otherwise
// the screen the request came from is updated in place.
func (s *Server) afterHabitChange(w http.ResponseWriter, r *http.Request, habitID int64) error {
	waiting, err := s.svc.Store.Q(r.Context()).HasCelebration()
	if err != nil {
		return err
	}
	if waiting {
		return redirect(w, r, "/celebrate")
	}
	if !isHTMX(r) {
		return redirect(w, r, "/")
	}
	if r.FormValue("from") == "detail" {
		p, err := s.detailData(r, habitID)
		if err != nil {
			return err
		}
		return s.view.Fragment(w, "habit", "habit-body", s.page(r, p.Habit.Name, "", p))
	}
	d, err := s.todayData(r)
	if err != nil {
		return err
	}
	return s.view.Fragment(w, "today", "today-body", s.page(r, "Heute", "today", d))
}

func (s *Server) habitCheck(w http.ResponseWriter, r *http.Request) error {
	id, day, err := s.habitAction(r)
	if err != nil {
		return err
	}
	if err := s.svc.Check(r.Context(), id, day); err != nil {
		return err
	}
	return s.afterHabitChange(w, r, id)
}

func (s *Server) habitAdd(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.svc.AddEntry(r.Context(), id); err != nil {
		return err
	}
	return s.afterHabitChange(w, r, id)
}

func (s *Server) habitUndo(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.svc.UndoEntry(r.Context(), id); err != nil {
		return err
	}
	return s.afterHabitChange(w, r, id)
}

func (s *Server) habitRelapse(w http.ResponseWriter, r *http.Request) error {
	id, day, err := s.habitAction(r)
	if err != nil {
		return err
	}
	if err := s.svc.Relapse(r.Context(), id, day); err != nil {
		return err
	}
	return s.afterHabitChange(w, r, id)
}

func (s *Server) habitJoker(w http.ResponseWriter, r *http.Request) error {
	id, day, err := s.habitAction(r)
	if err != nil {
		return err
	}
	if err := s.svc.UseJoker(r.Context(), id, day); err != nil {
		return err
	}
	return s.afterHabitChange(w, r, id)
}

func (s *Server) habitAction(r *http.Request) (int64, model.Day, error) {
	id, err := pathID(r)
	if err != nil {
		return 0, 0, err
	}
	day, err := s.requestDay(r)
	return id, day, err
}

type celebrateData struct {
	Level     *service.LevelUp
	Badge     *game.Badge
	Color     string
	Coins     int
	Message   string
	Confetti  []int
	CSRFToken string
}

func (s *Server) celebrate(w http.ResponseWriter, r *http.Request) error {
	_, kind, payload, err := s.svc.Store.Q(r.Context()).NextCelebration()
	if errors.Is(err, db.ErrNotFound) {
		return redirect(w, r, "/")
	}
	if err != nil {
		return err
	}
	d, err := s.celebrationView(kind, payload)
	if err != nil {
		return err
	}
	return s.view.Render(w, http.StatusOK, "celebrate", s.page(r, "Geschafft!", "", d))
}

func (s *Server) celebrateNext(w http.ResponseWriter, r *http.Request) error {
	q := s.svc.Store.Q(r.Context())
	id, _, _, err := q.NextCelebration()
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return err
	}
	if err == nil {
		if err := q.DeleteCelebration(id); err != nil {
			return err
		}
	}
	return redirect(w, r, "/celebrate")
}

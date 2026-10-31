package game

import (
	"math/bits"
	"slices"
	"time"

	"github.com/seals187/habitd/internal/model"
)

// HabitData bundles a habit with its complete history. Streaks are always
// derived from this, never stored.
type HabitData struct {
	model.Habit
	AreaKey  string
	Start    model.Day               // first day the habit counts
	Done     map[model.Day]time.Time // real check-ins with their creation time
	Joker    map[model.Day]bool      // days bridged by a joker
	Relapses []model.Day             // sorted
	Amount   float64                 // sum of all entries of counter and quantity habits
	Entries  map[model.Day]float64   // sum of the entries per day
}

// NewHabitData returns an empty history.
func NewHabitData(h model.Habit, start model.Day) *HabitData {
	return &HabitData{Habit: h, Start: start, Done: map[model.Day]time.Time{}, Joker: map[model.Day]bool{}, Entries: map[model.Day]float64{}}
}

// Run is a stretch of an unbroken streak.
type Run struct {
	Start, End model.Day
	Length     int // in units of the habit: days, or weeks for weekly rhythms
}

// Streak is the derived streak state of a habit.
type Streak struct {
	Current int   // in units
	Weekly  bool  // true when units are weeks
	Monthly bool  // true when units are months
	Days    int   // Current in days, the base of bonus and jokers
	Active  int   // days with a check-in inside the current streak, the base of Serien badges
	Longest int   // in units
	Runs    []Run // chronological, the last one may be the current run
}

// WeekBased reports whether streaks of the habit are counted in weeks.
func (h *HabitData) WeekBased() bool {
	return h.Kind == model.KindBuild && (h.Frequency == model.FreqWeekly || h.Frequency == model.FreqCustom)
}

// WeekTarget is the number of check-ins needed per week.
func (h *HabitData) WeekTarget() int {
	if h.Frequency == model.FreqCustom {
		return max(bits.OnesCount(uint(h.DaysMask&0x7f)), 1)
	}
	return max(h.WeeklyTarget, 1)
}

// Scheduled reports whether the habit is planned on a specific day. Weekly
// rhythms have no fixed days and report false.
func (h *HabitData) Scheduled(d model.Day) bool {
	if d < h.Start {
		return false
	}
	switch h.Frequency {
	case model.FreqDaily:
		return true
	case model.FreqCustom:
		return h.DaysMask&(1<<d.WeekdayIndex()) != 0
	default:
		return false
	}
}

// weekCounts returns real and joker check-ins of the week starting at w.
// Custom rhythms only count their scheduled days.
func (h *HabitData) weekCounts(w model.Day) (done, joker int) {
	for d := w; d < w+7; d++ {
		if h.Frequency == model.FreqCustom && h.DaysMask&(1<<d.WeekdayIndex()) == 0 {
			continue
		}
		if _, ok := h.Done[d]; ok {
			done++
		} else if h.Joker[d] {
			joker++
		}
	}
	return done, joker
}

// WeekMet reports whether the week starting at w reached its target.
func (h *HabitData) WeekMet(w model.Day) bool {
	done, joker := h.weekCounts(w)
	return done+joker >= h.WeekTarget()
}

// DueOn reports whether the habit still asks for a check-in on day d: planned
// for that day, or a weekly rhythm that has not reached its target yet.
func (h *HabitData) DueOn(d model.Day) bool {
	if h.Archived || h.Kind != model.KindBuild {
		return false
	}
	if _, doneToday := h.Done[d]; doneToday && h.Frequency != model.FreqDaily && h.Frequency != model.FreqCustom {
		return true
	}
	switch h.Frequency {
	case model.FreqWeekly:
		return d >= h.Start && !h.WeekMet(d.WeekStart())
	case model.FreqMonthly:
		return d >= h.Start && h.monthCount(monthStart(d)) < h.WeekTarget()
	}
	return h.Scheduled(d)
}

// StreakAt derives the streak as seen on day today; later data is ignored.
func (h *HabitData) StreakAt(today model.Day) Streak {
	var s Streak
	switch {
	case h.Kind == model.KindAvoid:
		s = h.avoidStreak(today)
	case h.WeekBased():
		s = h.weekStreak(today)
		s.Active = s.Current * h.WeekTarget()
		return s
	case h.Kind == model.KindBuild && h.Frequency == model.FreqMonthly:
		s = h.monthStreak(today)
		s.Active = s.Current * h.WeekTarget()
		return s
	default:
		s = h.dayStreak(today)
	}
	s.Active = s.Days
	return s
}

// first returns the earliest day with data, which may predate Start because
// check-ins can be backfilled.
func (h *HabitData) first() model.Day {
	first := h.Start
	for d := range h.Done {
		first = min(first, d)
	}
	for d := range h.Joker {
		first = min(first, d)
	}
	return first
}

// tally turns a sequence of steps into runs.
type tally struct {
	runs []Run
	open bool
}

func (t *tally) hit(start, end model.Day) {
	if !t.open {
		t.runs = append(t.runs, Run{Start: start})
		t.open = true
	}
	r := &t.runs[len(t.runs)-1]
	r.End = end
	r.Length++
}

func (t *tally) bridge(end model.Day) {
	if t.open {
		t.runs[len(t.runs)-1].End = end
	}
}

func (t *tally) miss() { t.open = false }

func (t *tally) result(unitDays int, weekly bool) Streak {
	s := Streak{Weekly: weekly, Runs: t.runs}
	for _, r := range t.runs {
		s.Longest = max(s.Longest, r.Length)
	}
	if t.open && len(t.runs) > 0 {
		s.Current = t.runs[len(t.runs)-1].Length
	}
	s.Days = s.Current * unitDays
	return s
}

func (h *HabitData) dayStreak(today model.Day) Streak {
	var t tally
	for d := h.first(); d <= today; d++ {
		if _, ok := h.Done[d]; ok {
			t.hit(d, d)
		} else if h.Joker[d] {
			t.bridge(d)
		} else if d != today {
			t.miss() // an open today does not break the streak yet
		}
	}
	return t.result(1, false)
}

func (h *HabitData) weekStreak(today model.Day) Streak {
	var t tally
	thisWeek := today.WeekStart()
	target := h.WeekTarget()
	for w := h.first().WeekStart(); w <= thisWeek; w += 7 {
		done, joker := h.weekCountsUntil(w, today)
		end := min(w+6, today)
		switch {
		case done >= target:
			t.hit(w, end)
		case done+joker >= target:
			t.bridge(end)
		case w != thisWeek:
			t.miss()
		}
	}
	return t.result(7, true)
}

// monthStart returns the first day of the day's month.
func monthStart(d model.Day) model.Day { return d - model.Day(d.Dom()-1) }

// nextMonth returns the first day of the following month.
func nextMonth(d model.Day) model.Day { return monthStart(monthStart(d) + 31) }

// monthCount counts the check-ins of the month starting at m.
func (h *HabitData) monthCount(m model.Day) int {
	n := 0
	for d := m; d < nextMonth(m); d++ {
		if _, ok := h.Done[d]; ok {
			n++
		}
	}
	return n
}

// monthStreak counts months that reached the target. Jokers bridge days, so
// they do not apply to this rhythm.
func (h *HabitData) monthStreak(today model.Day) Streak {
	var t tally
	thisMonth := monthStart(today)
	target := h.WeekTarget()
	for m := monthStart(h.first()); m <= thisMonth; m = nextMonth(m) {
		done := 0
		for d := m; d < nextMonth(m) && d <= today; d++ {
			if _, ok := h.Done[d]; ok {
				done++
			}
		}
		switch {
		case done >= target:
			t.hit(m, min(nextMonth(m)-1, today))
		case m != thisMonth:
			t.miss()
		}
	}
	s := t.result(30, false)
	s.Monthly = true
	return s
}

func (h *HabitData) weekCountsUntil(w, today model.Day) (done, joker int) {
	if today >= w+6 {
		return h.weekCounts(w)
	}
	limited := *h
	limited.Done = map[model.Day]time.Time{}
	limited.Joker = map[model.Day]bool{}
	for d := w; d <= today; d++ {
		if t, ok := h.Done[d]; ok {
			limited.Done[d] = t
		}
		if h.Joker[d] {
			limited.Joker[d] = true
		}
	}
	return limited.weekCounts(w)
}

// avoidStreak counts completed clean days: today only counts once it is over.
func (h *HabitData) avoidStreak(today model.Day) Streak {
	var t tally
	start := h.Start
	relapses := slices.Clone(h.Relapses)
	slices.Sort(relapses)
	for _, r := range relapses {
		if r >= today {
			break
		}
		for d := start; d < r; d++ {
			t.hit(d, d)
		}
		t.miss()
		start = r + 1
	}
	relapsedToday := slices.Contains(relapses, today)
	for d := start; d < today; d++ {
		t.hit(d, d)
	}
	if relapsedToday {
		t.miss()
	}
	return t.result(1, false)
}

package game

import (
	"math"
	"slices"

	"github.com/seals187/habitd/internal/model"
)

// TodayGroup is where a habit stands on the Today screen.
type TodayGroup uint8

const (
	GroupOpen    TodayGroup = iota // still asks for something today
	GroupReached                   // reached today, or for its week or month
	GroupAvoid                     // avoid habits, decided once the day is over
	GroupOff                       // fixed days and today is not one of them
)

// TodayGoal is how far a habit got with its goal of today.
type TodayGoal struct {
	Group    TodayGroup
	Done     bool    // checked in today
	Met      bool    // reached for today or for the running week or month
	Relapsed bool    // an avoid habit relapsed today
	Value    float64 // what counts towards a measured goal: the day, or the week of a weekly goal
	Fill     int     // percent of the goal, 0 to 100
	Total    int     // the goal in units: clicks of an amount, check-ins of a period, else one
	Units    int     // units reached, more than Total beyond the goal
	Undoable bool    // an entry of the goal can be taken back
	Reached  bool    // a measured goal has its check-in, also when ticked by hand
}

// TodayGoal tells how far a habit got with its goal of today.
func (h *HabitData) TodayGoal(today model.Day) TodayGoal {
	if h.Kind == model.KindAvoid {
		g := TodayGoal{Group: GroupAvoid, Relapsed: slices.Contains(h.Relapses, today), Total: 1}
		if !g.Relapsed {
			g.Fill = 100
		}
		return g
	}
	_, done := h.Done[today]
	g := TodayGoal{Done: done, Value: h.GoalTotal(today)}
	if h.Measured() {
		value := g.Value
		if _, g.Reached = h.GoalCheckin(today); g.Reached { // e.g. ticked before the habit became a counter
			value = max(value, h.Target)
		}
		g.Fill = min(int(value*100/h.Target), 100)
		g.Undoable = h.Entries[today] > 0 || h.GoalWeek && g.Value > 0
	} else if done {
		g.Fill = 100
	}
	periodMet := (h.Frequency == model.FreqWeekly || h.Frequency == model.FreqMonthly) && !h.DueOn(today)
	if periodMet {
		g.Fill = 100
	}
	g.Met = done || periodMet
	g.Total, g.Units = h.goalUnits(today, g.Met)
	switch {
	case h.Frequency == model.FreqCustom && !h.Scheduled(today) && !done:
		g.Group = GroupOff
	case g.Met:
		g.Group = GroupReached
	}
	return g
}

// goalUnits splits the goal into units: a click of a counter or an amount, a
// check-in of a weekly or monthly rhythm, else one.
func (h *HabitData) goalUnits(today model.Day, met bool) (total, units int) {
	total = 1
	switch {
	case h.Measured() && h.Step > 0:
		value := h.GoalTotal(today)
		if met { // ticked in the calendar or met for the period
			value = max(value, h.Target)
		}
		total = max(int(math.Ceil(h.Target/h.Step-1e-9)), 1)
		units = int(math.Floor(value/h.Step + 1e-9))
	case h.Frequency == model.FreqWeekly:
		total = h.WeekTarget()
		for d := today.WeekStart(); d < today.WeekStart()+7; d++ {
			if _, ok := h.Done[d]; ok && !h.GoalWeek {
				units++
			}
		}
	case h.Frequency == model.FreqMonthly:
		total, units = h.WeekTarget(), h.MonthCount(today)
	case met:
		units = 1
	}
	return total, units
}

// DayFill is the mean fill of the goals of today, so every click counts. Avoid
// habits and habits that are off today have no goal to fill.
func DayFill(goals []TodayGoal) int {
	sum, n := 0, 0
	for _, g := range goals {
		if g.Group == GroupOpen || g.Group == GroupReached {
			sum += g.Fill
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / n
}

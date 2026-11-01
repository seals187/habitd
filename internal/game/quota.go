package game

import (
	"slices"

	"github.com/seals187/habitd/internal/model"
)

// DayGoal counts the goal of a habit on one day. A rhythm without fixed days only
// has a goal on the days it was done.
func (h *HabitData) DayGoal(d model.Day) (done, planned int) {
	if h.Archived || d < h.Start {
		return 0, 0
	}
	if h.Kind == model.KindAvoid {
		if slices.Contains(h.Relapses, d) {
			return 0, 1
		}
		return 1, 1
	}
	_, hit := h.Done[d]
	switch {
	case !h.GoalWeek && h.Scheduled(d) && (hit || h.Joker[d]):
		return 1, 1
	case !h.GoalWeek && h.Scheduled(d):
		return 0, 1
	case hit:
		return 1, 1
	}
	return 0, 0
}

// Quota counts met and planned goals of a range that ends today at the latest. A
// period that is still running only counts what was reached, and so does a day
// that is still open: neither is missed yet.
func (h *HabitData) Quota(from, to, today model.Day) (done, planned int) {
	if h.Archived || to < h.Start {
		return 0, 0
	}
	switch {
	case h.Kind == model.KindAvoid:
		return h.dayQuota(from, to, today)
	case h.Frequency == model.FreqMonthly:
		return h.monthQuota(from, to)
	case h.GoalWeek || h.WeekBased():
		return h.weekQuota(from, to)
	}
	return h.dayQuota(from, to, today)
}

func (h *HabitData) dayQuota(from, to, today model.Day) (done, planned int) {
	for d := max(from, h.Start); d <= to; d++ {
		met, goal := h.DayGoal(d)
		if d == today && met == 0 && h.Kind != model.KindAvoid {
			continue
		}
		done, planned = done+met, planned+goal
	}
	return done, planned
}

func (h *HabitData) weekQuota(from, to model.Day) (done, planned int) {
	target := h.WeekTarget()
	if h.GoalWeek {
		target = 1
	}
	for w := from.WeekStart(); w <= to; w += 7 {
		real, joker := h.weekCountsUntil(w, to)
		count := min(real+joker, target)
		switch {
		case w+6 < h.Start || w+6 < from:
		case w+6 > to: // still running
			done, planned = done+count, planned+count
		case w < h.Start && count < target: // started in the middle of this week
		default:
			done, planned = done+count, planned+target
		}
	}
	return done, planned
}

func (h *HabitData) monthQuota(from, to model.Day) (done, planned int) {
	target := h.WeekTarget()
	for m := monthStart(from); m <= to; m = nextMonth(m) {
		count := 0
		for d := m; d < nextMonth(m) && d <= to; d++ {
			if _, ok := h.Done[d]; ok {
				count++
			}
		}
		count = min(count, target)
		switch {
		case nextMonth(m)-1 < h.Start:
		case nextMonth(m)-1 > to: // still running
			done, planned = done+count, planned+count
		case m < h.Start && count < target: // started in the middle of this month
		default:
			done, planned = done+count, planned+target
		}
	}
	return done, planned
}

// DaysLeft counts the days of the running period, today included, on which the
// streak of the habit can still grow.
func (h *HabitData) DaysLeft(today model.Day) int {
	end := today.WeekStart() + 6
	if h.Frequency == model.FreqMonthly {
		end = nextMonth(today) - 1
	}
	left := 0
	for d := today; d <= end; d++ {
		if h.Frequency != model.FreqCustom || h.Scheduled(d) {
			left++
		}
	}
	return left
}

// PeriodDone counts the check-ins of the running period up to today.
func (h *HabitData) PeriodDone(today model.Day) int {
	if h.Frequency == model.FreqMonthly {
		return h.MonthCount(today)
	}
	real, joker := h.weekCountsUntil(today.WeekStart(), today)
	return real + joker
}

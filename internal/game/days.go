package game

import (
	"slices"

	"github.com/seals187/habitd/internal/model"
)

// ForStatistics is the habit as its own statistics see it: a day filled in before
// the habit was created belongs to it, so it starts on its first day with anything
// on it. Quests, badges and rewards keep counting from the creation, which is why
// this is a copy that never goes back into a World.
func (h *HabitData) ForStatistics() *HabitData {
	c := *h
	c.Start = h.Since()
	return &c
}

// DayStatus is how one day of a habit went.
type DayStatus uint8

const (
	DayFree   DayStatus = iota // not planned, nothing happened
	DayBefore                  // before the habit counts
	DayFuture                  // after today
	DayOpen                    // today and not decided yet, also a clean day of an avoid habit
	DayDone                    // the goal of the day was reached, or a clean day that is over
	DayPart                    // an amount below the goal of the day
	DayJoker                   // bridged by a joker
	DayMissed                  // planned and over without a check-in, or a relapse
)

// DayMark is what happened on one day of a habit.
type DayMark struct {
	Status  DayStatus
	Checkin bool // a real check-in lies on the day, also the one of a weekly goal
	Fill    int  // percent of the goal of the day: 100 when done or bridged
}

// Mark tells how day d went, as seen on today. A weekly goal has no goal of the day,
// so its days are parts and its check-in only shows in Checkin.
func (h *HabitData) Mark(d, today model.Day) DayMark {
	_, checkin := h.Done[d]
	m := DayMark{Checkin: checkin}
	switch {
	case h.Kind == model.KindAvoid:
		switch {
		case d > today:
			m.Status = DayFuture
		case d < h.Start:
			m.Status = DayBefore
		case slices.Contains(h.Relapses, d):
			m.Status = DayMissed
		case d == today:
			m.Status = DayOpen
		default:
			m.Status = DayDone
		}
	case checkin && !h.GoalWeek:
		m.Status = DayDone
	case h.Joker[d]:
		m.Status = DayJoker
	case d > today:
		m.Status = DayFuture
	case d < h.Start:
		m.Status = DayBefore
	case h.Measured() && h.Entries[d] > 0:
		m.Status, m.Fill = DayPart, min(int(h.Entries[d]*100/h.Target), 99)
	case d == today:
		m.Status = DayOpen
	case h.Scheduled(d):
		m.Status = DayMissed
	default:
		m.Status = DayFree
	}
	if m.Status == DayDone || m.Status == DayJoker {
		m.Fill = 100
	}
	return m
}

// GoalValue is what a day needs: the goal of a measured habit, else one tick.
func (h *HabitData) GoalValue() float64 {
	if h.Measured() {
		return h.Target
	}
	return 1
}

// Value is what day d is worth in the charts: its amount, at least the goal when
// it was reached, and 1 for a clean day of an avoid habit once it is over.
func (h *HabitData) Value(d, today model.Day) float64 {
	if h.Kind == model.KindAvoid {
		if d < today && d >= h.Start && !slices.Contains(h.Relapses, d) {
			return 1
		}
		return 0
	}
	v := 0.0
	if h.Measured() {
		v = h.Entries[d]
	}
	if _, done := h.Done[d]; done && !h.GoalWeek {
		v = max(v, h.GoalValue())
	}
	return v
}

// Settled is the last day a statistic counts: today only once it is decided, an
// open day is neither done nor missed yet.
func (h *HabitData) Settled(today model.Day) model.Day {
	if _, done := h.Done[today]; done || slices.Contains(h.Relapses, today) {
		return today
	}
	return today - 1
}

// Achieved counts the days of a range with a check-in, or the clean days of an
// avoid habit. It is a count, not a rate: Quota is the rate.
func (h *HabitData) Achieved(from, to model.Day) int {
	n := 0
	for d := max(from, h.Start); d <= to; d++ {
		_, done := h.Done[d]
		if h.Kind == model.KindAvoid {
			done = !slices.Contains(h.Relapses, d)
		}
		if done {
			n++
		}
	}
	return n
}

// MonthQuota is the rate of one month, for months shown side by side. A week
// that is cut by the end of the month belongs to the month of its Sunday, so no
// week counts twice.
func (h *HabitData) MonthQuota(month, today model.Day) (done, planned int) {
	first, last := monthStart(month), nextMonth(month)-1
	to := min(last, today)
	if (h.GoalWeek || h.WeekBased()) && today > last && last.WeekdayIndex() != 6 {
		to = last.WeekStart() - 1
	}
	if to < first {
		return 0, 0
	}
	return h.Quota(first, to, today)
}

// Goals is the progress in a period: the goals met so far against all goals of
// the period, also those still ahead. Like in Quota a joker counts as met and a
// week, or a month, counts in the period that holds its last day; the running one
// counts too when the period reaches today.
func (h *HabitData) Goals(from, to, today model.Day) (met, planned int) {
	if h.Archived || to < h.Start {
		return 0, 0
	}
	switch {
	case h.Kind == model.KindBuild && h.Frequency == model.FreqMonthly:
		for m := monthStart(from); m <= to; m = nextMonth(m) {
			end := nextMonth(m) - 1
			if ok, count := h.periodCounts(m, end, from, to, today, func(until model.Day) int {
				n := 0
				for d := m; d <= until; d++ {
					if _, done := h.Done[d]; done {
						n++
					}
				}
				return n
			}); ok {
				met, planned = met+count, planned+h.WeekTarget()
			}
		}
	case h.Kind == model.KindBuild && (h.GoalWeek || h.WeekBased()):
		for w := from.WeekStart(); w <= to; w += 7 {
			if ok, count := h.periodCounts(w, w+6, from, to, today, func(until model.Day) int {
				real, joker := h.weekCountsUntil(w, until)
				return real + joker
			}); ok {
				met, planned = met+count, planned+h.periodTarget()
			}
		}
	default:
		for d := max(from, h.Start); d <= to; d++ {
			m, p := h.DayGoal(d)
			if d > today {
				m = 0
			}
			met, planned = met+m, planned+p
		}
	}
	return met, planned
}

// periodTarget is what a week asks for: its check-ins, or one reached weekly goal.
func (h *HabitData) periodTarget() int {
	if h.GoalWeek {
		return 1
	}
	return h.WeekTarget()
}

// periodCounts decides whether the period from begin to end counts in the range
// and how many of its goals were met, at most its target.
func (h *HabitData) periodCounts(begin, end, from, to, today model.Day, count func(until model.Day) int) (bool, int) {
	running := begin <= today && today <= end
	if end < from || end < h.Start || (end > to && !(running && to >= today)) {
		return false, 0
	}
	target := h.periodTarget()
	got := min(count(min(end, today)), target)
	if begin < h.Start && !running && got < target { // started in the middle of a period it did not meet
		return false, 0
	}
	return true, got
}

// AmountSum adds up the values of the days of a period.
type AmountSum struct {
	Total   float64
	Days    int // days that have happened
	Reached int // days with a check-in
	Best    float64
	BestDay model.Day
}

// Average is the mean value of the days that have happened.
func (a AmountSum) Average() float64 { return a.Total / float64(max(a.Days, 1)) }

// Sum adds up the days of a period, as far as they have happened.
func (h *HabitData) Sum(from, to, today model.Day) AmountSum {
	var sum AmountSum
	for x := max(from, h.Start); x <= min(to, today); x++ {
		v := h.Value(x, today)
		sum.Total += v
		sum.Days++
		if _, done := h.Done[x]; done {
			sum.Reached++
		}
		if v > sum.Best {
			sum.Best, sum.BestDay = v, x
		}
	}
	return sum
}

// BestWeekday is the weekday, Monday 0, that was achieved most often in a range,
// -1 when none was. Clean days of an avoid habit count once they are over.
func (h *HabitData) BestWeekday(from, today model.Day) int {
	counts := make([]int, 7)
	best, top := -1, 0
	for x := max(from, h.Start); x <= today; x++ {
		_, done := h.Done[x]
		if h.Kind == model.KindAvoid {
			done = x != today && !slices.Contains(h.Relapses, x)
		}
		if !done {
			continue
		}
		i := x.WeekdayIndex()
		counts[i]++
		if counts[i] > top {
			top, best = counts[i], i
		}
	}
	return best
}

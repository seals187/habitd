package game

import (
	"slices"

	"github.com/seals187/habitd/internal/model"
)

// Quest is the state of the daily quest: finish everything due today.
type Quest struct {
	Total    int
	Done     int
	Complete bool
}

// QuestFor derives the quest of a day. Avoid habits count as done unless a
// relapse was reported.
func QuestFor(habits []*HabitData, today model.Day) Quest {
	var q Quest
	builds := 0
	for _, h := range habits {
		if h.Archived || today < h.Start {
			continue
		}
		if h.Kind == model.KindAvoid {
			q.Total++
			if !slices.Contains(h.Relapses, today) {
				q.Done++
			}
			continue
		}
		if !h.DueOn(today) {
			continue
		}
		builds++
		q.Total++
		if _, ok := h.Done[today]; ok {
			q.Done++
		}
	}
	q.Complete = builds > 0 && q.Done == q.Total
	return q
}

// BridgeDay returns the day a joker has to cover so that the streak survives
// the given day, and false when nothing needs saving there. Weekly rhythms are
// evaluated on Sunday and can only be saved when one check-in is missing.
func (h *HabitData) BridgeDay(d model.Day) (model.Day, bool) {
	if h.Archived || h.Kind != model.KindBuild || d < h.Start {
		return 0, false
	}
	if h.Frequency == model.FreqWeekly {
		return h.bridgeWeek(d)
	}
	if _, done := h.Done[d]; done || h.Joker[d] || !h.Scheduled(d) {
		return 0, false
	}
	if h.StreakAt(d-1).Days == 0 {
		return 0, false
	}
	return d, true
}

func (h *HabitData) bridgeWeek(d model.Day) (model.Day, bool) {
	if d.WeekdayIndex() != 6 {
		return 0, false
	}
	week := d.WeekStart()
	done, joker := h.weekCounts(week)
	if done+joker != h.WeekTarget()-1 || h.StreakAt(week-1).Days == 0 {
		return 0, false
	}
	for free := d; free >= week; free-- {
		if _, taken := h.Done[free]; !taken && !h.Joker[free] {
			return free, true
		}
	}
	return 0, false
}

// RewardProgress is the streak counted towards a streak reward. Without
// reset the best run since the last redemption stays valid after a break.
func RewardProgress(h *HabitData, today, lastRedeemed model.Day, resetOnBreak bool) int {
	s := h.StreakAt(today)
	if resetOnBreak {
		return s.Days
	}
	unit := 1
	if s.Weekly {
		unit = 7
	}
	best := 0
	for _, r := range s.Runs {
		if r.End > lastRedeemed {
			best = max(best, r.Length*unit)
		}
	}
	return best
}

package game

import (
	"testing"
	"time"

	"github.com/seals187/habitd/internal/model"
)

func TestTodayGoalGroups(t *testing.T) {
	today := quotaMonday + 2 // a Wednesday
	daily := quotaHabit(model.FreqDaily)
	if g := daily.TodayGoal(today); g.Group != GroupOpen || g.Fill != 0 || g.Met {
		t.Errorf("an open day: %+v", g)
	}
	daily.Done[today] = time.Time{}
	if g := daily.TodayGoal(today); g.Group != GroupReached || g.Fill != 100 || !g.Done || g.Units != 1 {
		t.Errorf("a ticked day: %+v", g)
	}

	weekly := quotaHabit(model.FreqWeekly, 0, 1)
	weekly.WeeklyTarget = 2
	if g := weekly.TodayGoal(today); g.Group != GroupReached || !g.Met || g.Done || g.Fill != 100 || g.Units != 2 || g.Total != 2 {
		t.Errorf("a week that is met stays reached: %+v", g)
	}

	custom := quotaHabit(model.FreqCustom)
	custom.DaysMask = 0b0000001 // Mondays
	if g := custom.TodayGoal(today); g.Group != GroupOff {
		t.Errorf("off today: %+v", g)
	}
	custom.Done[today] = time.Time{}
	if g := custom.TodayGoal(today); g.Group != GroupReached {
		t.Errorf("done on a free day: %+v", g)
	}

	avoid := NewHabitData(model.Habit{Kind: model.KindAvoid, Frequency: model.FreqDaily}, quotaMonday)
	if g := avoid.TodayGoal(today); g.Group != GroupAvoid || g.Fill != 100 {
		t.Errorf("a clean day: %+v", g)
	}
	avoid.Relapses = []model.Day{today}
	if g := avoid.TodayGoal(today); !g.Relapsed || g.Fill != 0 {
		t.Errorf("a relapse: %+v", g)
	}
}

func TestTodayGoalAmounts(t *testing.T) {
	today := quotaMonday + 2
	h := NewHabitData(model.Habit{Kind: model.KindBuild, Frequency: model.FreqDaily, Measure: model.MeasureAmount, Target: 3, Step: 0.5}, quotaMonday)
	h.Entries[today] = 1.5
	if g := h.TodayGoal(today); g.Fill != 50 || g.Total != 6 || g.Units != 3 || !g.Undoable || g.Group != GroupOpen {
		t.Errorf("half way: %+v", g)
	}
	h.Entries[today] = 3.5
	h.Done[today] = time.Time{}
	if g := h.TodayGoal(today); g.Fill != 100 || g.Units != 7 || !g.Reached || g.Group != GroupReached {
		t.Errorf("beyond the goal: %+v", g)
	}

	ticked := NewHabitData(h.Habit, quotaMonday)
	ticked.Done[today] = time.Time{}
	if g := ticked.TodayGoal(today); g.Fill != 100 || g.Units != 6 || g.Undoable {
		t.Errorf("ticked in the calendar without an amount: %+v", g)
	}
}

func TestDayFill(t *testing.T) {
	goals := []TodayGoal{
		{Group: GroupOpen, Fill: 0},
		{Group: GroupReached, Fill: 100},
		{Group: GroupOpen, Fill: 50},
		{Group: GroupAvoid, Fill: 100},
		{Group: GroupOff},
	}
	if got := DayFill(goals); got != 50 {
		t.Errorf("mean of the goals of today: %d", got)
	}
	if DayFill(nil) != 0 {
		t.Error("no goals, no fill")
	}
}

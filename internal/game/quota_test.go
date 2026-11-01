package game

import (
	"testing"
	"time"

	"github.com/seals187/habitd/internal/model"
)

// A Monday, so weeks line up with the days of a test.
var quotaMonday = model.NewDay(2026, time.October, 5)

func quotaHabit(freq string, days ...int) *HabitData {
	h := NewHabitData(model.Habit{ID: 1, Kind: model.KindBuild, Frequency: freq}, quotaMonday)
	for _, d := range days {
		h.Done[quotaMonday+model.Day(d)] = time.Time{}
	}
	return h
}

func TestDayGoal(t *testing.T) {
	daily := quotaHabit(model.FreqDaily, 0)
	if done, planned := daily.DayGoal(quotaMonday); done != 1 || planned != 1 {
		t.Errorf("done day = %d/%d", done, planned)
	}
	if done, planned := daily.DayGoal(quotaMonday + 1); done != 0 || planned != 1 {
		t.Errorf("open day = %d/%d", done, planned)
	}
	if _, planned := daily.DayGoal(quotaMonday - 1); planned != 0 {
		t.Errorf("before the start the habit has no goal, got %d", planned)
	}
	weekly := quotaHabit(model.FreqWeekly, 2)
	weekly.WeeklyTarget = 3
	if _, planned := weekly.DayGoal(quotaMonday); planned != 0 {
		t.Errorf("a rhythm without fixed days has no goal on a free day, got %d", planned)
	}
	if done, planned := weekly.DayGoal(quotaMonday + 2); done != 1 || planned != 1 {
		t.Errorf("weekly check-in = %d/%d", done, planned)
	}
	daily.Archived = true
	if _, planned := daily.DayGoal(quotaMonday); planned != 0 {
		t.Errorf("archived habits have no goals, got %d", planned)
	}
}

func TestQuotaDaily(t *testing.T) {
	h := quotaHabit(model.FreqDaily, 0, 1, 3)
	today := quotaMonday + 4
	if done, planned := h.Quota(quotaMonday, today, today); done != 3 || planned != 4 {
		t.Errorf("an open today is not missed yet: %d/%d", done, planned)
	}
	h.Done[today] = time.Time{}
	if done, planned := h.Quota(quotaMonday, today, today); done != 4 || planned != 5 {
		t.Errorf("a done today counts: %d/%d", done, planned)
	}
	h.Joker[quotaMonday+2] = true
	if done, _ := h.Quota(quotaMonday, today, today); done != 5 {
		t.Errorf("a joker keeps the goal: %d", done)
	}
}

func TestQuotaWeekly(t *testing.T) {
	h := quotaHabit(model.FreqWeekly, 0, 1, 7, 14)
	h.WeeklyTarget = 2
	today := quotaMonday + 15 // third week is running
	if done, planned := h.Quota(quotaMonday, today, today); done != 4 || planned != 5 {
		t.Errorf("2/2 + 1/2 + running 1 = %d/%d", done, planned)
	}
}

func TestQuotaMonthly(t *testing.T) {
	first := model.NewDay(2026, time.September, 1)
	h := NewHabitData(model.Habit{ID: 1, Kind: model.KindBuild, Frequency: model.FreqMonthly, WeeklyTarget: 2}, first)
	h.Done[first+3] = time.Time{}
	h.Done[first+30] = time.Time{} // October 1, running
	today := first + 40
	if done, planned := h.Quota(first, today, today); done != 2 || planned != 3 {
		t.Errorf("1/2 and a running month = %d/%d", done, planned)
	}
}

func TestQuotaAvoid(t *testing.T) {
	h := NewHabitData(model.Habit{ID: 1, Kind: model.KindAvoid}, quotaMonday)
	h.Relapses = []model.Day{quotaMonday + 1}
	today := quotaMonday + 2
	if done, planned := h.Quota(quotaMonday, today, today); done != 2 || planned != 3 {
		t.Errorf("clean days = %d/%d", done, planned)
	}
}

func TestDaysLeft(t *testing.T) {
	weekly := quotaHabit(model.FreqWeekly)
	if got := weekly.DaysLeft(quotaMonday + 5); got != 2 {
		t.Errorf("Saturday leaves %d days", got)
	}
	custom := quotaHabit(model.FreqCustom)
	custom.DaysMask = 1 | 1<<6 // Monday and Sunday
	if got := custom.DaysLeft(quotaMonday + 1); got != 1 {
		t.Errorf("custom rhythm leaves %d days", got)
	}
}

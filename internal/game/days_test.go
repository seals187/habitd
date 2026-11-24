package game

import (
	"testing"
	"time"

	"github.com/seals187/habitd/internal/model"
)

func TestMarkBuild(t *testing.T) {
	today := quotaMonday + 9                                                                                             // a Wednesday
	h := NewHabitData(model.Habit{Kind: model.KindBuild, Frequency: model.FreqCustom, DaysMask: 0b0010101}, quotaMonday) // Mo, Mi, Fr
	h.Done[quotaMonday] = time.Time{}
	h.Joker[quotaMonday+2] = true
	for d, want := range map[model.Day]DayStatus{
		quotaMonday - 1: DayBefore,
		quotaMonday:     DayDone,
		quotaMonday + 1: DayFree,
		quotaMonday + 2: DayJoker,
		quotaMonday + 4: DayMissed,
		today:           DayOpen,
		today + 1:       DayFuture,
	} {
		if got := h.Mark(d, today).Status; got != want {
			t.Errorf("day %d: %v, want %v", d-quotaMonday, got, want)
		}
	}
	if m := h.Mark(quotaMonday+2, today); m.Fill != 100 {
		t.Errorf("a joker fills the day: %+v", m)
	}
}

func TestMarkAmounts(t *testing.T) {
	today := quotaMonday + 2
	h := NewHabitData(model.Habit{Kind: model.KindBuild, Frequency: model.FreqDaily, Measure: model.MeasureAmount, Target: 4}, quotaMonday)
	h.Entries[quotaMonday] = 1
	h.Entries[quotaMonday+1] = 4
	h.Done[quotaMonday+1] = time.Time{}
	if m := h.Mark(quotaMonday, today); m.Status != DayPart || m.Fill != 25 {
		t.Errorf("a part shows its share: %+v", m)
	}
	if m := h.Mark(quotaMonday+1, today); m.Status != DayDone || m.Fill != 100 || !m.Checkin {
		t.Errorf("a reached day is done: %+v", m)
	}

	h.GoalWeek = true
	if m := h.Mark(quotaMonday+1, today); m.Status != DayPart || !m.Checkin {
		t.Errorf("a weekly goal has no goal of the day, only its check-in: %+v", m)
	}
}

func TestMarkAvoid(t *testing.T) {
	today := quotaMonday + 3
	h := NewHabitData(model.Habit{Kind: model.KindAvoid, Frequency: model.FreqDaily}, quotaMonday)
	h.Relapses = []model.Day{quotaMonday + 1}
	for d, want := range map[model.Day]DayStatus{
		quotaMonday - 1: DayBefore,
		quotaMonday:     DayDone,
		quotaMonday + 1: DayMissed,
		today:           DayOpen, // counts once it is over
		today + 1:       DayFuture,
	} {
		if got := h.Mark(d, today).Status; got != want {
			t.Errorf("day %d: %v, want %v", d-quotaMonday, got, want)
		}
	}
	if h.Value(today, today) != 0 || h.Value(quotaMonday, today) != 1 || h.Value(quotaMonday+1, today) != 0 {
		t.Error("only clean days that are over are worth something")
	}
}

func TestSumCountsOnlyWhatHappened(t *testing.T) {
	today := model.NewDay(2026, time.October, 31)
	start := today - 3
	h := &HabitData{Habit: model.Habit{Measure: model.MeasureAmount, Target: 3, Frequency: model.FreqDaily}, Start: start,
		Done:    map[model.Day]time.Time{start: {}},
		Entries: map[model.Day]float64{start - 1: 99, start: 3.5, start + 1: 1, today: 2, today + 1: 50}}
	sum := h.Sum(start-5, today+5, today)
	if sum.Total != 6.5 || sum.Days != 4 || sum.Reached != 1 || sum.Best != 3.5 || sum.BestDay != start {
		t.Fatalf("%+v", sum)
	}
	if _, plan := h.Goals(today.WeekStart(), today.WeekStart()+6, today); plan != int(today.WeekStart()+6-start)+1 {
		t.Fatalf("the plan counts from the start: %d", plan)
	}
}

func TestGoalsOfAWeek(t *testing.T) {
	today := quotaMonday + 9 // Wednesday of the second week
	h := quotaHabit(model.FreqWeekly, 0, 2)
	h.WeeklyTarget = 3
	h.Joker[quotaMonday+4] = true
	if met, planned := h.Goals(quotaMonday, quotaMonday+13, today); met != 3 || planned != 6 {
		t.Errorf("a met week with a joker and a running one: %d/%d", met, planned)
	}
	if met, planned := h.Goals(quotaMonday+7, quotaMonday+13, today); met != 0 || planned != 3 {
		t.Errorf("the running week asks for all of its goal: %d/%d", met, planned)
	}
	busy := quotaHabit(model.FreqWeekly, 0, 1, 2, 3, 4)
	busy.WeeklyTarget = 3
	if met, planned := busy.Goals(quotaMonday, quotaMonday+6, today); met != 3 || planned != 3 {
		t.Errorf("more than the goal stays at the goal: %d/%d", met, planned)
	}
	daily := quotaHabit(model.FreqDaily, 0, 1)
	if met, planned := daily.Goals(quotaMonday, quotaMonday+6, quotaMonday+2); met != 2 || planned != 7 {
		t.Errorf("a daily habit on Wednesday: %d/%d", met, planned)
	}
	if done, planned := daily.Quota(quotaMonday, quotaMonday+2, quotaMonday+2); done != 2 || planned != 2 {
		t.Errorf("the rate leaves the open day out: %d/%d", done, planned)
	}
}

func TestMonthQuotaCountsEachWeekOnce(t *testing.T) {
	sep28 := model.NewDay(2026, time.September, 28) // a Monday
	h := NewHabitData(model.Habit{Kind: model.KindBuild, Frequency: model.FreqWeekly, WeeklyTarget: 1}, sep28)
	oct6, oct27 := model.NewDay(2026, time.October, 6), model.NewDay(2026, time.October, 27)
	h.Done[oct6], h.Done[oct27] = time.Time{}, time.Time{}
	today := model.NewDay(2026, time.November, 20)
	if done, planned := h.MonthQuota(oct6, today); done != 1 || planned != 4 {
		t.Errorf("October ends with the week of 25 October: %d/%d", done, planned)
	}
	if done, planned := h.MonthQuota(today, today); done != 1 || planned != 3 {
		t.Errorf("November holds the week of 1 November and a running week: %d/%d", done, planned)
	}
}

func TestAchievedCounts(t *testing.T) {
	h := quotaHabit(model.FreqWeekly, 0, 1, 2, 3, 4)
	if got := h.Achieved(quotaMonday-3, quotaMonday+6); got != 5 {
		t.Errorf("every check-in counts: %d", got)
	}
	avoid := NewHabitData(model.Habit{Kind: model.KindAvoid}, quotaMonday)
	avoid.Relapses = []model.Day{quotaMonday + 1}
	if got := avoid.Achieved(quotaMonday, quotaMonday+2); got != 2 {
		t.Errorf("clean days: %d", got)
	}
}
func TestForStatisticsLeavesTheHabitAlone(t *testing.T) {
	h := quotaHabit(model.FreqDaily)
	h.Entries[quotaMonday-4] = 1
	stats := h.ForStatistics()
	if stats.Start != quotaMonday-4 || h.Start != quotaMonday {
		t.Fatalf("stats start %d, habit start %d", stats.Start-quotaMonday, h.Start-quotaMonday)
	}
	if !stats.Scheduled(quotaMonday-2) || h.Scheduled(quotaMonday-2) {
		t.Error("only the statistics count the days before the creation")
	}
}

func TestSettledAndBestWeekday(t *testing.T) {
	today := quotaMonday + 8
	h := quotaHabit(model.FreqDaily, 1, 8)
	if h.Settled(today) != today {
		t.Error("a done today is settled")
	}
	if h.Settled(today-1) != today-2 {
		t.Error("an open day is not settled")
	}
	if got := h.BestWeekday(quotaMonday, today); got != 1 {
		t.Errorf("Tuesday twice: %d", got)
	}
	if got := quotaHabit(model.FreqDaily).BestWeekday(quotaMonday, today); got != -1 {
		t.Errorf("nothing done: %d", got)
	}
}

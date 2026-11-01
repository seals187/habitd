package tips

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

// today is a Sunday.
var today = model.NewDay(2026, time.November, 1)

const evening = 19

func at(d model.Day, hour, minute int) time.Time {
	return time.Date(d.Year(), d.Month(), d.Dom(), hour, minute, 0, 0, time.UTC)
}

func habit(age int, change func(*model.Habit)) *game.HabitData {
	m := model.Habit{ID: 1, Kind: model.KindBuild, Frequency: model.FreqDaily, Measure: model.MeasureCheck, Target: 1, Step: 1}
	if change != nil {
		change(&m)
	}
	return game.NewHabitData(m, today-model.Day(age))
}

func measured(m *model.Habit) { m.Measure, m.Target, m.Step, m.Unit = model.MeasureAmount, 3, 0.5, "l" }

func weekly(m *model.Habit) { m.Frequency, m.WeeklyTarget = model.FreqWeekly, 3 }

func avoid(m *model.Habit) { m.Kind = model.KindAvoid }

func input(h *game.HabitData) Input {
	return Input{Habit: h, Today: today, Now: at(today, 10, 0), Format: func(v float64) string { return fmt.Sprintf("%.1f L", v) }}
}

// tick marks the days as done and returns them as stamps.
func tick(h *game.HabitData, hour int, days ...model.Day) []Stamp {
	var out []Stamp
	for _, d := range days {
		h.Done[d] = at(d, hour, 10)
		out = append(out, Stamp{Day: d, At: h.Done[d], Amount: 1})
	}
	return out
}

// span lists the days from a days ago up to b days ago.
func span(a, b int) []model.Day {
	var out []model.Day
	for i := a; i >= b; i-- {
		out = append(out, today-model.Day(i))
	}
	return out
}

func wants(t *testing.T, r rule, in Input, text string) {
	t.Helper()
	tip, ok := r(in)
	if !ok || !strings.Contains(tip.Title+" | "+tip.Detail, text) {
		t.Fatalf("want %q, got %v %+v", text, ok, tip)
	}
}

func silent(t *testing.T, r rule, in Input) {
	t.Helper()
	if tip, ok := r(in); ok {
		t.Fatalf("want no tip, got %+v", tip)
	}
}

func TestTypicalTime(t *testing.T) {
	h := habit(20, nil)
	in := input(h)
	in.Done = tick(h, 18, span(3, 0)...)
	silent(t, typicalTime, in)
	in.Done = append(in.Done, tick(h, 16, today-5)...)
	wants(t, typicalTime, in, "Meist erledigt gegen 18:00")
	wants(t, typicalTime, in, "an 5 von 7 Tagen, am frühesten Di 16:10")
}

func TestWeekdays(t *testing.T) {
	h := habit(55, nil)
	for _, d := range span(55, 1) {
		if d.WeekdayIndex() != 2 {
			tick(h, evening, d)
		}
	}
	wants(t, weakWeekday, input(h), "Am Mi ist am meisten drin")
	silent(t, weakWeekday, input(habit(3, nil)))
	silent(t, bestWeekday, input(habit(3, nil)))

	h = habit(55, nil)
	for _, d := range span(55, 1) {
		if d.WeekdayIndex() == 5 || d.WeekdayIndex() == 0 && int(d)%2 == 0 {
			tick(h, evening, d)
		}
	}
	wants(t, bestWeekday, input(h), "Sa ist dein stärkster Tag")
	silent(t, bestWeekday, input(habit(55, weekly)))
}

func TestTrend(t *testing.T) {
	h := habit(20, measured)
	for _, d := range span(14, 8) {
		h.Entries[d] = 2
	}
	for _, d := range span(7, 1) {
		h.Entries[d] = 2.4
	}
	wants(t, trend, input(h), "+0.4 L pro Tag mehr")
	for _, d := range span(7, 1) {
		h.Entries[d] = 1.5
	}
	wants(t, trend, input(h), "0.5 L pro Tag mehr")
	for _, d := range span(7, 1) {
		h.Entries[d] = 2
	}
	silent(t, trend, input(h))

	check := habit(20, nil)
	tick(check, evening, span(10, 8)...)
	tick(check, evening, span(5, 1)...)
	wants(t, trend, input(check), "+2 Tage mehr")
	silent(t, trend, input(habit(9, nil)))
	silent(t, trend, input(habit(20, avoid)))
}

func TestStreakAtRisk(t *testing.T) {
	h := habit(20, nil)
	in := input(h)
	in.Streak = game.Streak{Current: 12}
	in.Done = tick(h, 18, span(6, 1)...)
	silent(t, streakAtRisk, in)
	in.Now = at(today, 19, 30)
	wants(t, streakAtRisk, in, "Deine Serie von 12 Tagen wartet")
	in.Streak.Current = 2
	silent(t, streakAtRisk, in)
	in.Streak.Current = 12
	tick(h, 18, today)
	silent(t, streakAtRisk, in)

	water := habit(20, measured)
	water.Entries[today] = 1
	in = input(water)
	in.Streak, in.Now = game.Streak{Current: 5}, at(today, 21, 0)
	wants(t, streakAtRisk, in, "noch 2.0 L bis zum Ziel")
}

func TestRecordInReach(t *testing.T) {
	in := input(habit(30, nil))
	in.Streak = game.Streak{Current: 12, Longest: 14}
	wants(t, recordInReach, in, "Noch 2 Tage bis zum Rekord")
	in.Streak = game.Streak{Current: 10, Longest: 10}
	wants(t, recordInReach, in, "Rekordkurs")
	in.Streak = game.Streak{Current: 5, Longest: 20}
	silent(t, recordInReach, in)
	in.Streak = game.Streak{Current: 2, Longest: 3}
	silent(t, recordInReach, in)
}

func TestMorningShare(t *testing.T) {
	in := input(habit(10, measured))
	in.Drink = true
	in.Stamps = []Stamp{{At: at(today, 9, 0), Amount: 0.5}}
	silent(t, morningShare, in)
	for range 5 {
		in.Stamps = append(in.Stamps, Stamp{At: at(today, 15, 0), Amount: 0.5})
	}
	wants(t, morningShare, in, "Morgens trinkst du wenig")
	wants(t, morningShare, in, "nur 17 % bis 12 Uhr")
	in.Drink = false
	wants(t, morningShare, in, "Morgens ist noch Luft")
	in.Stamps = append(in.Stamps, Stamp{At: at(today, 8, 0), Amount: 3})
	silent(t, morningShare, in)
}

func TestAlmostThere(t *testing.T) {
	h := habit(20, measured)
	h.Entries[today-2], h.Entries[today-3] = 2.8, 2.9
	wants(t, almostThere, input(h), "an 2 Tagen")
	h.Entries[today-3] = 1
	silent(t, almostThere, input(h))
}

func TestAboveGoal(t *testing.T) {
	h := habit(20, measured)
	for _, d := range span(7, 1) {
		h.Entries[d] = 3.5
	}
	wants(t, aboveGoal, input(h), "im Schnitt 3.5 L")
	h.Entries[today-1], h.Entries[today-2] = 0, 0
	silent(t, aboveGoal, input(h))
}

func TestCheckRules(t *testing.T) {
	h := habit(40, nil)
	in := input(h)
	in.Done = tick(h, 8, span(30, 1)...)
	wants(t, checkEarly, in, "100 % schon bis 12 Uhr")
	wants(t, checkSteady, in, "an 30 von 30 Tagen")

	late := habit(40, nil)
	in = input(late)
	in.Done = tick(late, evening, span(10, 1)...)
	silent(t, checkEarly, in)
	silent(t, checkSteady, in)
}

func TestAvoidRules(t *testing.T) {
	h := habit(60, avoid)
	in := input(h)
	in.Streak = game.Streak{Days: 12}
	wants(t, avoidRun, in, "noch 2 Tage bis 14 Tage")
	in.Streak.Days = 3
	silent(t, avoidRun, in)

	silent(t, avoidRiskyDay, in)
	wed := today - 4
	h.Relapses = []model.Day{wed - 14, wed - 7, wed, today - 2}
	wants(t, avoidRiskyDay, in, "Am Mi lohnt sich ein Plan")
	h.Relapses = []model.Day{today - 14, today - 7, today - 3}
	wants(t, avoidRiskyDay, in, "Heute ist So")
	if tip, _ := avoidRiskyDay(in); tip.Score <= 60 {
		t.Fatalf("today counts more: %+v", tip)
	}
}

func TestWeekPace(t *testing.T) {
	friday := today - 2 // three days of the week are left
	h := habit(30, weekly)
	in := input(h)
	in.Today = friday
	tick(h, evening, friday-3)
	wants(t, weekPace, in, "Noch 2 bis zum Wochenziel")
	wants(t, weekPace, in, "3 Tage bleiben")
	tick(h, evening, friday-2, friday-1)
	wants(t, weekPace, in, "Wochenziel geschafft")

	far := habit(30, func(m *model.Habit) { m.Frequency, m.WeeklyTarget = model.FreqWeekly, 7 })
	in = input(far)
	in.Today = friday
	silent(t, weekPace, in)

	goal := habit(30, func(m *model.Habit) { measured(m); m.GoalWeek, m.Target = true, 10 })
	goal.Entries[friday-1] = 4
	in = input(goal)
	in.Today = friday
	wants(t, weekPace, in, "Noch 6.0 L bis zum Wochenziel")
	silent(t, weekPace, input(habit(30, nil)))
}

func TestWeekFavorites(t *testing.T) {
	h := habit(50, weekly)
	silent(t, weekFavorites, input(h))
	monday := today.WeekStart()
	for w := range 4 {
		tick(h, evening, monday-model.Day(w*7))
	}
	tick(h, evening, monday-3, monday-10)
	wants(t, weekFavorites, input(h), "Deine Tage: Mo und Fr")
}

func TestForSortsAndLimits(t *testing.T) {
	h := habit(40, measured)
	in := input(h)
	in.Streak = game.Streak{Current: 12, Longest: 14}
	in.Now = at(today, 21, 0)
	in.Drink = true
	for _, d := range span(14, 1) {
		h.Entries[d] = 2.9
		in.Stamps = append(in.Stamps, Stamp{Day: d, At: at(d, 15, 0), Amount: 2.9})
	}
	got := For(in)
	if len(got) != MaxTips {
		t.Fatalf("at most %d tips: %+v", MaxTips, got)
	}
	if got[0].Score < got[1].Score || got[1].Score < got[2].Score {
		t.Fatalf("most relevant first: %+v", got)
	}
	if !strings.Contains(got[0].Title, "Serie") || !strings.Contains(got[1].Title, "Rekord") {
		t.Fatalf("risk and record lead: %+v", got)
	}
}

func TestNothingOnFreshHabits(t *testing.T) {
	for _, change := range []func(*model.Habit){nil, measured, avoid} {
		in := input(habit(2, change))
		in.Streak = game.Streak{Current: 3, Longest: 3, Days: 3}
		if got := For(in); len(got) != 0 {
			t.Fatalf("too early: %+v", got)
		}
	}
}

func TestHours(t *testing.T) {
	in := input(habit(10, measured))
	in.Stamps = []Stamp{{At: at(today, 18, 0), Amount: 2}, {At: at(today, 18, 30), Amount: 2}, {At: at(today, 7, 0), Amount: 1}, {At: at(today, 3, 0), Amount: 1}}
	if Hours(in) != nil {
		t.Fatal("too few stamps")
	}
	in.Stamps = append(in.Stamps, Stamp{At: at(today, 23, 0), Amount: 0.25})
	got := Hours(in)
	if len(got) != 18 || got[12] != maxLevel || got[1] != 1 || got[0] != 1 || got[17] != 1 || got[5] != 0 {
		t.Fatalf("levels: %v", got)
	}
	in.Habit = habit(10, avoid)
	if Hours(in) != nil {
		t.Fatal("no strip for avoid habits")
	}
}

package server

import (
	"slices"
	"testing"
	"time"

	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

func TestRealMoment(t *testing.T) {
	d := model.NewDay(2026, time.November, 1)
	at := func(hour, minute int, day model.Day) time.Time {
		return time.Date(day.Year(), day.Month(), day.Dom(), hour, minute, 0, 0, time.UTC)
	}
	if !realMoment(at(18, 40, d), d) || realMoment(at(12, 0, d), d) || realMoment(at(18, 40, d+1), d) {
		t.Fatal("only a click on the day itself has a real time")
	}
}

func TestBarStatus(t *testing.T) {
	today := model.NewDay(2026, time.November, 1)
	h := game.NewHabitData(model.Habit{Kind: model.KindBuild, Frequency: model.FreqDaily}, today-10)
	h.Done[today-1] = time.Date(2026, time.October, 31, 18, 40, 0, 0, time.UTC)
	h.Joker[today-2] = true
	for d, want := range map[model.Day]string{today: "läuft", today - 1: "✓18:40", today - 2: "Joker", today - 3: "verpasst"} {
		if got, _ := barStatus(h, d, today); got != want {
			t.Fatalf("%v: got %q, want %q", d, got, want)
		}
	}
}

func TestBestStreaks(t *testing.T) {
	cards := []habitCard{
		{Streak: game.Streak{Current: 5}},
		{Streak: game.Streak{Current: 12}},
		{Streak: game.Streak{Current: 3, Weekly: true}},
		{Streak: game.Streak{Current: 2, Weekly: true}},
		{Streak: game.Streak{}},
	}
	if got, want := bestStreaks(cards), []streakPart{{12, "T"}, {3, "Wo"}}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := bestStreaks(cards[4:]); got != nil {
		t.Fatalf("no running streak shows nothing: %v", got)
	}
	cards = append(cards, habitCard{Streak: game.Streak{Current: 4, Monthly: true}})
	if got := bestStreaks(cards); len(got) != 3 || got[2] != (streakPart{4, "Mo"}) {
		t.Fatalf("months come last: %v", got)
	}
}

package service

import (
	"testing"
	"time"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

func (e *env) saveQuests(fn func(game.QuestDef) game.QuestDef) {
	e.t.Helper()
	for _, q := range e.s.Rules().Quests {
		if _, err := e.q().SaveQuest(fn(q)); err != nil {
			e.t.Fatal(err)
		}
	}
	e.loadRules()
}

func (e *env) only(key string) {
	e.t.Helper()
	e.saveQuests(func(q game.QuestDef) game.QuestDef {
		q.Active = q.Key == key
		return q
	})
}

func (e *env) drawn(d model.Day) []string {
	e.t.Helper()
	rows, err := e.q().UserQuests(d)
	if err != nil {
		e.t.Fatal(err)
	}
	var keys []string
	for _, u := range rows {
		for _, q := range e.s.Rules().Quests {
			if q.ID == u.QuestID {
				keys = append(keys, q.Key)
			}
		}
	}
	return keys
}

func TestDrawHappensOncePerDay(t *testing.T) {
	e := setup(t)
	e.habit(model.Habit{}, 3)
	if err := e.s.EnsureQuests(e.ctx); err != nil {
		t.Fatal(err)
	}
	first := e.drawn(e.s.QuestDay())
	if len(first) != 1 || first[0] != "alles" {
		t.Fatalf("draw: %v", first)
	}
	e.s.Pick = func(n int) int { return n - 1 }
	if err := e.s.EnsureQuests(e.ctx); err != nil {
		t.Fatal(err)
	}
	if again := e.drawn(e.s.QuestDay()); len(again) != 1 || again[0] != "alles" {
		t.Fatalf("redrawn: %v", again)
	}
}

func TestQuestDayStartsAtRollover(t *testing.T) {
	e := setup(t)
	today := e.s.Today()
	e.now = time.Date(today.Year(), today.Month(), today.Dom(), 3, 59, 0, 0, time.UTC)
	if e.s.QuestDay() != e.s.Today()-1 {
		t.Fatal("before 04:00 it is still yesterday")
	}
	e.now = e.now.Add(time.Minute)
	if e.s.QuestDay() != e.s.Today() {
		t.Fatal("from 04:00 on it is today")
	}
}

func TestNoRepeatAndMatching(t *testing.T) {
	e := setup(t)
	e.habit(model.Habit{}, 3)
	if err := e.s.EnsureQuests(e.ctx); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.AddDate(0, 0, 1)
	if err := e.s.EnsureQuests(e.ctx); err != nil {
		t.Fatal(err)
	}
	got := e.drawn(e.s.QuestDay())
	if len(got) != 1 || got[0] == "alles" {
		t.Fatalf("same quest twice in a row: %v", got)
	}
	if got[0] == "ueberflieger" || got[0] == "sauber" {
		t.Fatalf("drew a quest without a matching habit: %v", got)
	}
}

func TestQuestRewardsAndUndo(t *testing.T) {
	e := setup(t)
	e.only("frueher-vogel")
	e.saveQuests(func(q game.QuestDef) game.QuestDef {
		q.Days, q.Count, q.Until = game.AllDays, 1, "23:00"
		q.RewardKind, q.Reward = game.RewardXP, 77
		return q
	})
	id := e.habit(model.Habit{}, 3)
	d := e.s.Today()
	if err := e.s.Check(e.ctx, id, d); err != nil {
		t.Fatal(err)
	}
	if sums, _ := e.q().LedgerSum(questRef(d)); sums[model.LedgerXP] != 77 {
		t.Fatalf("reward: %v", sums)
	}
	if err := e.s.Check(e.ctx, id, d); err != nil {
		t.Fatal(err)
	}
	if sums, _ := e.q().LedgerSum(questRef(d)); sums[model.LedgerXP] != 0 {
		t.Fatalf("undo: %v", sums)
	}
}

func TestPlannedQuestWinsAndEventDoubles(t *testing.T) {
	e := setup(t)
	id := e.habit(model.Habit{}, 3)
	d := e.s.QuestDay()
	var planned, extra game.QuestDef
	for _, q := range e.s.Rules().Quests {
		switch q.Key {
		case "vernachlaessigt":
			planned = q
		case "alles":
			extra = q
		}
	}
	if err := e.q().PlanQuest(d, planned.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.q().SaveQuestEvent(db.QuestEvent{Name: "Test", Start: d, End: d, QuestID: extra.ID, Multiplier: 2}); err != nil {
		t.Fatal(err)
	}
	if err := e.s.Check(e.ctx, id, d); err != nil {
		t.Fatal(err)
	}
	got := e.drawn(d)
	if len(got) != 2 {
		t.Fatalf("plan and event: %v", got)
	}
	coins := 0
	for i := range got {
		sums, _ := e.q().LedgerSum(questRefAt(d, i))
		coins += sums[model.LedgerCoins]
	}
	if coins != 2*(planned.Reward+extra.Reward) {
		t.Fatalf("doubled rewards: %d", coins)
	}
}

func TestAvoidQuestPaysAfterTheDay(t *testing.T) {
	e := setup(t)
	e.only("sauber")
	e.habit(model.Habit{Kind: model.KindAvoid}, 3)
	if err := e.s.EnsureQuests(e.ctx); err != nil {
		t.Fatal(err)
	}
	d := e.s.QuestDay()
	if sums, _ := e.q().LedgerSum(questRef(d)); sums[model.LedgerCoins] != 0 {
		t.Fatal("paid before the day ended")
	}
	e.now = e.now.AddDate(0, 0, 1)
	if err := e.s.Settle(e.ctx); err != nil {
		t.Fatal(err)
	}
	if sums, _ := e.q().LedgerSum(questRef(d)); sums[model.LedgerCoins] == 0 {
		t.Fatal("not paid after a clean day")
	}
}

func TestQuestStreak(t *testing.T) {
	d := model.Day(1000)
	days := map[model.Day]bool{d: true, d - 1: true, d - 2: false, d - 3: true}
	if got := QuestStreakAt(days, d); got != 2 {
		t.Fatalf("streak: %d", got)
	}
	delete(days, d)
	if got := QuestStreakAt(days, d); got != 1 {
		t.Fatalf("an open today keeps it: %d", got)
	}
}

func TestSevenQuestsInARowPayAJoker(t *testing.T) {
	e := setup(t)
	e.only("alles")
	id := e.habit(model.Habit{}, 10)
	for range 7 {
		if err := e.s.Check(e.ctx, id, e.s.Today()); err != nil {
			t.Fatal(err)
		}
		e.now = e.now.AddDate(0, 0, 1)
	}
	if sums, _ := e.q().LedgerSum("queststreak:" + (e.s.Today() - 1).String()); sums[model.LedgerJoker] != 1 {
		t.Fatalf("bonus: %v", sums)
	}
}

package service

import (
	"context"
	"testing"

	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

func (e *env) loadRules() {
	e.t.Helper()
	if err := e.s.LoadRules(context.Background()); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) unlocked(key string) bool {
	e.t.Helper()
	badges, err := e.q().UnlockedBadges()
	if err != nil {
		e.t.Fatal(err)
	}
	_, ok := badges[key]
	return ok
}

// A database round trip must not change what any badge measures.
func TestStoredRulesEqualTheDefaults(t *testing.T) {
	e := setup(t)
	e.loadRules()
	stored, want := e.s.Rules(), game.DefaultRules()
	if len(stored.Badges) != 95 || len(stored.Categories) != 16 || len(stored.Levels) != game.LevelCount {
		t.Fatalf("%d badges, %d categories, %d levels", len(stored.Badges), len(stored.Categories), len(stored.Levels))
	}
	for i, b := range want.Badges {
		got := stored.Badges[i]
		if got.Key != b.Key || got.Target != b.Target || got.Formula() != b.Formula() || got.Coins != game.BadgeCoins || !got.Active || !got.Builtin {
			t.Fatalf("%+v differs from %+v", got, b)
		}
	}
	if err := e.s.Seed(e.ctx); err != nil {
		t.Fatal(err)
	}
	w, err := e.s.World(e.q())
	if err != nil {
		t.Fatal(err)
	}
	f := game.BuildFacts(w)
	for i, b := range want.Badges {
		if a, c := stored.Badges[i].Value(f), b.Value(f); a != c {
			t.Fatalf("%s: stored %d, default %d", b.Key, a, c)
		}
	}
	for _, xp := range []int{0, 99, 8940, 500000} {
		if a, c := stored.LevelFor(xp), want.LevelFor(xp); a != c || c != game.LevelFor(xp) {
			t.Fatalf("level at %d: %d, %d", xp, a, c)
		}
	}
}

func TestConfiguredBadgeIsEvaluated(t *testing.T) {
	e := setup(t)
	e.loadRules()
	custom := game.Badge{Key: "eigenes", Name: "Eigenes", Category: "serien", Icon: "flame", Target: 2, Active: true,
		Metric: game.Metric{Rule: game.RuleCount, Scope: game.ScopeAnyHabit}, Coins: 7, Jokers: 1}
	if err := e.q().SaveBadge(custom); err != nil {
		t.Fatal(err)
	}
	e.loadRules()
	id := e.habit(model.Habit{}, 5)
	day := e.s.Today()
	if err := e.s.Check(e.ctx, id, day-1); err != nil {
		t.Fatal(err)
	}
	if e.unlocked("eigenes") {
		t.Fatal("unlocked too early")
	}
	if err := e.s.Check(e.ctx, id, day); err != nil {
		t.Fatal(err)
	}
	if !e.unlocked("eigenes") {
		t.Fatal("custom badge not unlocked")
	}
	if sums, _ := e.q().LedgerSum("badge:eigenes"); sums[model.LedgerCoins] != 7 || sums[model.LedgerJoker] != 1 {
		t.Fatalf("reward: %v", sums)
	}
}

func TestAmountBadgeSumsEntries(t *testing.T) {
	e := setup(t)
	e.loadRules()
	if err := e.q().SaveBadge(game.Badge{Key: "menge", Name: "Menge", Category: "serien", Icon: "flame", Target: 3000, Active: true,
		Metric: game.Metric{Rule: game.RuleAmount, Scope: game.ScopeAnyHabit}}); err != nil {
		t.Fatal(err)
	}
	e.loadRules()
	id := e.habit(model.Habit{Measure: model.MeasureAmount, Target: 10000, Step: 1000, Unit: "Schritte"}, 1)
	for i := range 3 {
		if e.unlocked("menge") {
			t.Fatalf("unlocked after %d clicks", i)
		}
		if err := e.s.AddEntry(e.ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if !e.unlocked("menge") {
		t.Fatal("amount badge not unlocked")
	}
}

func TestDeactivatedBadgeIsNotAwardedButKept(t *testing.T) {
	e := setup(t)
	e.loadRules()
	id := e.habit(model.Habit{}, 3)
	if err := e.s.Check(e.ctx, id, e.s.Today()-1); err != nil {
		t.Fatal(err)
	}
	if !e.unlocked("erster-schritt") {
		t.Fatal("setup: first badge missing")
	}
	for _, key := range []string{"erster-schritt", "zehner"} {
		if err := e.q().SetBadgeActive(key, false); err != nil {
			t.Fatal(err)
		}
	}
	e.loadRules()
	for d := 2; d <= 3; d++ {
		if err := e.s.Check(e.ctx, id, e.s.Today()-0); err != nil {
			t.Fatal(err)
		}
	}
	if !e.unlocked("erster-schritt") {
		t.Fatal("holders must keep a deactivated badge")
	}
	second := e.habit(model.Habit{Name: "Zweites"}, 3)
	reached := game.Badge{Key: "zehner", Target: 1, Metric: game.Metric{Rule: game.RuleCount, Scope: game.ScopeAnyHabit}}
	w, _ := e.s.World(e.q())
	if !reached.Reached(game.BuildFacts(w)) {
		t.Fatal("setup: the rule would be fulfilled")
	}
	if err := e.s.Check(e.ctx, second, e.s.Today()); err != nil {
		t.Fatal(err)
	}
	if e.unlocked("zehner") {
		t.Fatal("deactivated badge was awarded")
	}
}

func TestStricterRuleKeepsHolders(t *testing.T) {
	e := setup(t)
	e.loadRules()
	id := e.habit(model.Habit{}, 3)
	if err := e.s.Check(e.ctx, id, e.s.Today()); err != nil {
		t.Fatal(err)
	}
	b, _ := e.s.Rules().BadgeByKey("erster-schritt")
	b.Target = 500
	if err := e.q().SaveBadge(b); err != nil {
		t.Fatal(err)
	}
	e.loadRules()
	if err := e.s.HabitSaved(e.ctx); err != nil {
		t.Fatal(err)
	}
	if !e.unlocked("erster-schritt") {
		t.Fatal("holder lost the badge")
	}
}

func TestChangedThresholdsDoNotLowerLevels(t *testing.T) {
	e := setup(t)
	e.loadRules()
	q := e.q()
	if err := q.AddLedger(e.now, model.LedgerXP, 1000, "test", ""); err != nil {
		t.Fatal(err)
	}
	if err := e.s.HabitSaved(e.ctx); err != nil {
		t.Fatal(err)
	}
	level, _ := q.SettingInt(KeyLevel, 0)
	if level < 2 {
		t.Fatalf("setup: level %d", level)
	}

	levels := e.s.Rules().Levels
	raised := make([]game.Level, len(levels))
	for i, l := range levels {
		l.XP *= 100
		raised[i] = l
	}
	if err := q.SaveLevels(raised); err != nil {
		t.Fatal(err)
	}
	e.loadRules()
	if err := e.s.RecalculateLevels(context.Background()); err != nil {
		t.Fatal(err)
	}
	if now, _ := q.SettingInt(KeyLevel, 0); now != level {
		t.Fatalf("level sank from %d to %d", level, now)
	}
	w, _ := e.s.World(q)
	if w.Level != level {
		t.Fatalf("world level %d", w.Level)
	}

	s := e.s.Rules().Settings
	s.NoDrop = false
	if err := q.SaveLevelSettings(s); err != nil {
		t.Fatal(err)
	}
	e.loadRules()
	if err := e.s.RecalculateLevels(context.Background()); err != nil {
		t.Fatal(err)
	}
	if now, _ := q.SettingInt(KeyLevel, 0); now != 1 {
		t.Fatalf("without the rule the level follows the XP: %d", now)
	}
}

func TestLevelRewardsComeFromTheTable(t *testing.T) {
	e := setup(t)
	e.loadRules()
	levels := append([]game.Level(nil), e.s.Rules().Levels...)
	levels[1].Coins, levels[1].Jokers = 7, 0
	if err := e.q().SaveLevels(levels); err != nil {
		t.Fatal(err)
	}
	e.loadRules()
	if err := e.q().AddLedger(e.now, model.LedgerXP, levels[1].XP, "test", ""); err != nil {
		t.Fatal(err)
	}
	if err := e.s.HabitSaved(e.ctx); err != nil {
		t.Fatal(err)
	}
	if b := e.bal(); b.Coins != 7 || b.Jokers != 0 {
		t.Fatalf("balances: %+v", b)
	}
}

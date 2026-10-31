package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

type env struct {
	t   *testing.T
	s   *Service
	ctx context.Context
	now time.Time
}

func setup(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	store, err := db.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	e := &env{t: t, ctx: ctx, now: time.Date(2026, 10, 30, 19, 0, 0, 0, time.UTC)}
	e.s = New(store, time.UTC)
	e.s.Now = func() time.Time { return e.now }
	id, err := e.s.CreateUser(ctx, "test", "test-password", model.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	e.ctx = db.WithUser(ctx, id)
	return e
}

func (e *env) q() *db.Q { return e.s.Store.Q(e.ctx) }

func (e *env) habit(h model.Habit, age int) int64 {
	e.t.Helper()
	if h.Kind == "" {
		h.Kind = model.KindBuild
	}
	if h.Frequency == "" {
		h.Frequency = model.FreqDaily
	}
	if h.Difficulty == "" {
		h.Difficulty = model.Medium
	}
	if h.Name == "" {
		h.Name = "Test"
	}
	areas, err := e.q().Areas()
	if err != nil {
		e.t.Fatal(err)
	}
	h.AreaID = areas[1].ID
	h.CreatedAt = e.now.AddDate(0, 0, -age)
	id, err := e.q().SaveHabit(h)
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (e *env) bal() model.Balances {
	e.t.Helper()
	b, err := e.q().Balances(e.s.Today())
	if err != nil {
		e.t.Fatal(err)
	}
	return b
}

func (e *env) credit(kind string, amount int) {
	e.t.Helper()
	if err := e.q().AddLedger(e.now, kind, amount, "test", ""); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) check(id int64, d model.Day) {
	e.t.Helper()
	if err := e.s.Check(e.ctx, id, d); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) nextDay() { e.now = e.now.AddDate(0, 0, 1) }

// noLevels keeps level ups out of tests about jokers.
func (e *env) noLevels() {
	e.t.Helper()
	if err := e.q().SetSetting(KeyLevel, "50"); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) badgeBonus() int {
	unlocked, err := e.q().UnlockedBadges()
	if err != nil {
		e.t.Fatal(err)
	}
	return len(unlocked) * game.BadgeCoins
}

func TestCheckAwardsAndUndoReverses(t *testing.T) {
	e := setup(t)
	id := e.habit(model.Habit{}, 0)
	today := e.s.Today()
	e.check(id, today)
	b := e.bal()
	if b.XP != 15 {
		t.Fatalf("xp %d", b.XP)
	}
	// One habit, so the daily quest is done too. Level 1 costs 20 XP.
	if want := 5 + game.QuestCoins + e.badgeBonus(); b.Coins != want {
		t.Fatalf("coins %d want %d", b.Coins, want)
	}
	e.check(id, today)
	b = e.bal()
	if b.XP != 0 {
		t.Fatalf("xp after undo %d", b.XP)
	}
	if b.Coins != e.badgeBonus() {
		t.Fatalf("coins after undo %d", b.Coins)
	}
}

func TestBackfillWindow(t *testing.T) {
	e := setup(t)
	id := e.habit(model.Habit{}, 30)
	today := e.s.Today()
	e.check(id, today-7)
	if err := e.s.Check(e.ctx, id, today-8); !errors.Is(err, ErrOutOfRange) {
		t.Fatalf("got %v", err)
	}
	if err := e.s.Check(e.ctx, id, today+1); !errors.Is(err, ErrOutOfRange) {
		t.Fatalf("got %v", err)
	}
}

func TestStreakBonusApplies(t *testing.T) {
	e := setup(t)
	id := e.habit(model.Habit{}, 30)
	today := e.s.Today()
	for d := today - 6; d < today; d++ {
		e.check(id, d)
	}
	before := e.bal().XP
	if before != 6*15 {
		t.Fatalf("xp %d", before)
	}
	e.check(id, today) // seventh day: 15 * 1.5, rounded
	if got := e.bal().XP - before; got != 23 {
		t.Fatalf("gain %d", got)
	}
}

func TestLevelIsKeptAfterUndoAndNotPaidTwice(t *testing.T) {
	e := setup(t)
	id := e.habit(model.Habit{Difficulty: model.Hard}, 0)
	today := e.s.Today()
	e.check(id, today) // 25 XP reaches level 2
	level, _ := e.q().SettingInt(KeyLevel, 1)
	if level != 2 {
		t.Fatalf("level %d", level)
	}
	jokers := e.bal().Jokers
	coins := e.bal().Coins
	if jokers != 1 {
		t.Fatalf("jokers %d", jokers)
	}
	e.check(id, today)
	e.check(id, today)
	level, _ = e.q().SettingInt(KeyLevel, 1)
	if level != 2 {
		t.Fatalf("level after undo %d", level)
	}
	w, err := e.s.World(e.q())
	if err != nil {
		t.Fatal(err)
	}
	if w.Level != 2 {
		t.Fatalf("world level %d", w.Level)
	}
	if e.bal().Jokers != jokers || e.bal().Coins != coins {
		t.Fatalf("bonus paid twice: %+v", e.bal())
	}
}

func TestStreakJokerOnceAndCapped(t *testing.T) {
	e := setup(t)
	e.noLevels()
	id := e.habit(model.Habit{}, 30)
	today := e.s.Today()
	for d := today - 5; d <= today; d++ {
		e.check(id, d)
	}
	base := e.bal().Jokers
	e.check(id, today-6) // the run now reaches 7 days on today
	e.check(id, today)
	e.check(id, today)
	if got := e.bal().Jokers; got != base+1 {
		t.Fatalf("jokers %d, base %d", got, base)
	}
	e.credit(model.LedgerJoker, game.MaxJokers-e.bal().Jokers)
	e.check(id, today)
	e.check(id, today)
	if got := e.bal().Jokers; got != game.MaxJokers {
		t.Fatalf("cap exceeded: %d", got)
	}
}

func TestQuestNeedsAllAndIsPaidOnce(t *testing.T) {
	e := setup(t)
	a := e.habit(model.Habit{Name: "A"}, 0)
	b := e.habit(model.Habit{Name: "B"}, 0)
	today := e.s.Today()
	quest := func() int {
		sums, err := e.q().LedgerSum(questRef(today))
		if err != nil {
			t.Fatal(err)
		}
		return sums[model.LedgerCoins]
	}
	e.check(a, today)
	if quest() != 0 {
		t.Fatal("quest paid early")
	}
	e.check(b, today)
	if quest() != game.QuestCoins {
		t.Fatal("quest not paid")
	}
	e.check(b, today)
	if quest() != 0 {
		t.Fatal("quest not reversed")
	}
	e.check(b, today)
	if quest() != game.QuestCoins {
		t.Fatal("quest not paid again")
	}
}

func TestRelapseBlocksQuest(t *testing.T) {
	e := setup(t)
	a := e.habit(model.Habit{Name: "A"}, 0)
	x := e.habit(model.Habit{Name: "X", Kind: model.KindAvoid}, 0)
	today := e.s.Today()
	e.check(a, today)
	if err := e.s.Relapse(e.ctx, x, today); err != nil {
		t.Fatal(err)
	}
	if sums, _ := e.q().LedgerSum(questRef(today)); sums[model.LedgerCoins] != 0 {
		t.Fatal("quest should be reversed")
	}
}

func TestAvoidSettlesCleanDaysOnce(t *testing.T) {
	e := setup(t)
	x := e.habit(model.Habit{Kind: model.KindAvoid}, 0)
	e.nextDay()
	e.nextDay()
	e.nextDay()
	if err := e.s.Settle(e.ctx); err != nil {
		t.Fatal(err)
	}
	if err := e.s.Settle(e.ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.bal().XP; got != 3*game.AvoidXP {
		t.Fatalf("xp %d", got)
	}
	_ = x
}

func TestRelapseStopsPayButKeepsXP(t *testing.T) {
	e := setup(t)
	x := e.habit(model.Habit{Kind: model.KindAvoid}, 0)
	e.nextDay()
	if err := e.s.Settle(e.ctx); err != nil {
		t.Fatal(err)
	}
	earned := e.bal().XP
	if err := e.s.Relapse(e.ctx, x, e.s.Today()); err != nil {
		t.Fatal(err)
	}
	if e.bal().XP != earned {
		t.Fatal("relapse must not remove XP")
	}
	e.nextDay()
	if err := e.s.Settle(e.ctx); err != nil {
		t.Fatal(err)
	}
	if e.bal().XP != earned {
		t.Fatalf("relapse day was paid: %d", e.bal().XP)
	}
	w, err := e.s.World(e.q())
	if err != nil {
		t.Fatal(err)
	}
	if got := Find(w, x).StreakAt(w.Today).Current; got != 0 {
		t.Fatalf("streak %d", got)
	}
}

func TestAutoJokerBridgesMissedDay(t *testing.T) {
	e := setup(t)
	id := e.habit(model.Habit{}, 10)
	today := e.s.Today()
	e.check(id, today-1)
	e.check(id, today)
	e.credit(model.LedgerJoker, 2)
	jokers := e.bal().Jokers
	e.nextDay() // tomorrow stays open
	e.nextDay() // now tomorrow counts as missed
	if err := e.s.Settle(e.ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.bal().Jokers; got != jokers-1 {
		t.Fatalf("jokers %d want %d", got, jokers-1)
	}
	e.check(id, e.s.Today())
	w, _ := e.s.World(e.q())
	if got := Find(w, id).StreakAt(w.Today).Current; got != 3 {
		t.Fatalf("streak %d", got)
	}
}

func TestManualModeLeavesTheChoice(t *testing.T) {
	e := setup(t)
	if err := e.q().SetSetting(KeyJokerMode, JokerManual); err != nil {
		t.Fatal(err)
	}
	id := e.habit(model.Habit{}, 10)
	e.check(id, e.s.Today())
	e.credit(model.LedgerJoker, 1)
	jokers := e.bal().Jokers
	missed := e.s.Today() + 1
	e.nextDay()
	e.nextDay()
	if err := e.s.Settle(e.ctx); err != nil {
		t.Fatal(err)
	}
	if e.bal().Jokers != jokers {
		t.Fatal("joker used in manual mode")
	}
	w, _ := e.s.World(e.q())
	offers := Bridgeable(w)
	if len(offers) != 1 || offers[0].Day != missed {
		t.Fatalf("offers %+v", offers)
	}
	if err := e.s.UseJoker(e.ctx, id, missed); err != nil {
		t.Fatal(err)
	}
	if e.bal().Jokers != jokers-1 {
		t.Fatal("joker not spent")
	}
}

func TestLateCheckinRefundsJoker(t *testing.T) {
	e := setup(t)
	e.noLevels()
	id := e.habit(model.Habit{}, 10)
	today := e.s.Today()
	e.check(id, today-2)
	e.credit(model.LedgerJoker, 1)
	if err := e.s.UseJoker(e.ctx, id, today-1); err != nil {
		t.Fatal(err)
	}
	jokers := e.bal().Jokers
	e.check(id, today-1)
	if e.bal().Jokers != jokers+1 {
		t.Fatalf("no refund: %d", e.bal().Jokers)
	}
}

func TestRedeemCoins(t *testing.T) {
	e := setup(t)
	if err := e.q().SaveReward(model.Reward{Title: "Kaffee", Mode: model.RewardCoins, Price: 100, Repeatable: false}); err != nil {
		t.Fatal(err)
	}
	if err := e.s.Redeem(e.ctx, 1); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("got %v", err)
	}
	e.credit(model.LedgerCoins, 300)
	if err := e.s.Redeem(e.ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := e.s.Redeem(e.ctx, 1); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("second redeem: %v", err)
	}
	reds, _ := e.q().Redemptions()
	if len(reds) != 1 || reds[0].PricePaid != 100 {
		t.Fatalf("redemptions %+v", reds)
	}
	if want := 200 + e.badgeBonus(); e.bal().Coins != want {
		t.Fatalf("coins %d want %d", e.bal().Coins, want)
	}
	if e.bal().CoinsEarned != 300+e.badgeBonus() {
		t.Fatalf("spending must not lower earned: %+v", e.bal())
	}
}

func TestStreakRewardUnlocksAndResets(t *testing.T) {
	e := setup(t)
	id := e.habit(model.Habit{}, 30)
	if err := e.q().SaveReward(model.Reward{Title: "Buch", Mode: model.RewardStreak, HabitID: id, StreakTarget: 3, ResetOnRelapse: true, Repeatable: true}); err != nil {
		t.Fatal(err)
	}
	if err := e.q().SaveReward(model.Reward{Title: "Sticky", Mode: model.RewardStreak, HabitID: id, StreakTarget: 3}); err != nil {
		t.Fatal(err)
	}
	if err := e.s.Redeem(e.ctx, 1); !errors.Is(err, ErrLocked) {
		t.Fatalf("got %v", err)
	}
	today := e.s.Today()
	for d := today - 2; d <= today; d++ {
		e.check(id, d)
	}
	e.nextDay()
	e.nextDay() // streak broken now
	w, _ := e.s.World(e.q())
	states, err := e.s.RewardStates(e.q(), w)
	if err != nil {
		t.Fatal(err)
	}
	if states[0].Available {
		t.Fatal("reset reward should be locked again")
	}
	if !states[1].Available {
		t.Fatal("sticky reward should stay available")
	}
}

func TestBuyJoker(t *testing.T) {
	e := setup(t)
	if err := e.s.BuyJoker(e.ctx); !errors.Is(err, ErrInsufficient) {
		t.Fatalf("got %v", err)
	}
	e.credit(model.LedgerCoins, 500)
	if err := e.s.BuyJoker(e.ctx); err != nil {
		t.Fatal(err)
	}
	if b := e.bal(); b.Jokers != 1 || b.Coins != 380+e.badgeBonus() {
		t.Fatalf("got %+v", b)
	}
	e.credit(model.LedgerJoker, 2)
	if err := e.s.BuyJoker(e.ctx); !errors.Is(err, ErrJokerLimit) {
		t.Fatalf("got %v", err)
	}
}

func TestBadgeCelebrationAndBonus(t *testing.T) {
	e := setup(t)
	id := e.habit(model.Habit{Difficulty: model.Easy}, 0)
	e.check(id, e.s.Today())
	unlocked, _ := e.q().UnlockedBadges()
	if _, ok := unlocked["erster-schritt"]; !ok {
		t.Fatalf("got %v", unlocked)
	}
	_, kind, payload, err := e.q().NextCelebration()
	if err != nil {
		t.Fatal(err)
	}
	if kind != "badge" || payload == "" {
		t.Fatalf("got %s %s", kind, payload)
	}
}

func TestLevelCelebrationComesFirst(t *testing.T) {
	e := setup(t)
	id := e.habit(model.Habit{Difficulty: model.Hard}, 0)
	e.check(id, e.s.Today())
	_, kind, _, err := e.q().NextCelebration()
	if err != nil {
		t.Fatal(err)
	}
	if kind != "level" {
		t.Fatalf("got %s", kind)
	}
}

func TestSeedMatchesMockups(t *testing.T) {
	e := setup(t)
	if err := e.s.Seed(e.ctx); err != nil {
		t.Fatal(err)
	}
	b := e.bal()
	if b.XP != 8940 || b.Coins != 340 || b.CoinsEarned != 2115 || b.CoinsToday != 45 || b.Jokers != 2 {
		t.Fatalf("balances %+v", b)
	}
	w, err := e.s.World(e.q())
	if err != nil {
		t.Fatal(err)
	}
	p := game.ProgressFor(b.XP, w.Level)
	if p.Level != 12 || p.Into != 1440 || p.Cost != 2000 {
		t.Fatalf("progress %+v", p)
	}
	if len(w.Redemptions) != 17 {
		t.Fatalf("redemptions %d", len(w.Redemptions))
	}
	wantDays := map[string]int{"20 Min. lesen": 27, "Kein Alkohol": 40}
	wantWeeks := map[string]int{"Sport": 5, "Mama anrufen": 6, "Neue Idee einbringen": 3}
	for _, h := range w.Habits {
		s := h.StreakAt(w.Today)
		if want, ok := wantDays[h.Name]; ok && (s.Current != want || s.Weekly) {
			t.Errorf("%s: %+v", h.Name, s)
		}
		if want, ok := wantWeeks[h.Name]; ok && (s.Current != want || !s.Weekly) {
			t.Errorf("%s: %+v", h.Name, s)
		}
	}
	reading := w.Habits[0]
	if len(reading.Done) != 212 {
		t.Errorf("reading days %d", len(reading.Done))
	}
	if s := reading.StreakAt(w.Today); s.Longest != 41 {
		t.Errorf("record %d", s.Longest)
	}
	if got := len(reading.Done); got != 212 {
		t.Errorf("done %d", got)
	}
	seen19 := false
	for _, r := range reading.StreakAt(w.Today).Runs {
		if r.Length == 19 {
			seen19 = true
		}
		if r.Length > 19 && r.Length != 27 && r.Length != 41 {
			t.Errorf("unexpected run %+v", r)
		}
	}
	if !seen19 {
		t.Error("run of 19 missing")
	}
	if err := e.s.Seed(e.ctx); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("second seed: %v", err)
	}
	unlocked, _ := e.q().UnlockedBadges()
	t.Logf("%d badges unlocked: %v", len(unlocked), keys(unlocked))
}

func keys(m map[string]time.Time) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestExportImportRoundTrip(t *testing.T) {
	e := setup(t)
	if err := e.s.Seed(e.ctx); err != nil {
		t.Fatal(err)
	}
	before := e.bal()
	dump, err := e.q().DumpAll()
	if err != nil {
		t.Fatal(err)
	}
	if err := e.s.Import(e.ctx, dump); err != nil {
		t.Fatal(err)
	}
	if after := e.bal(); after != before {
		t.Fatalf("before %+v after %+v", before, after)
	}
	habits, _ := e.q().Habits()
	if len(habits) != 5 {
		t.Fatalf("habits %d", len(habits))
	}
}

func TestImportRejectsUnknownVersion(t *testing.T) {
	e := setup(t)
	if err := e.s.Import(e.ctx, &db.Export{Version: 99}); err == nil {
		t.Fatal("expected error")
	}
}

func TestMigrationKeepsChildRows(t *testing.T) {
	e := setup(t)
	id := e.habit(model.Habit{}, 0)
	e.check(id, e.s.Today())
	h, err := e.q().Habit(id)
	if err != nil {
		t.Fatal(err)
	}
	if h.TemplateID != "" {
		t.Fatalf("got %+v", h)
	}
	areas, _ := e.q().Areas()
	if len(areas) != 10 || areas[5].Color != "#F5A3A3" {
		t.Fatalf("areas %+v", areas)
	}
}

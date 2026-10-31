// Package service applies the game rules to stored data. Every write runs in
// one transaction, so balances, badges and celebrations always stay in step.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

// Setting keys.
const (
	KeyLegacyUnnamed = "legacy_admin_unnamed"
	KeyInstalledOn   = "installed_on"
	KeyOnboarded     = "onboarded"
	KeyLevel         = "level"
	KeyLastSettled   = "last_settled"
	KeyJokerMode     = "joker_mode"
	KeyFigure        = "figure"
	KeyReminderOn    = "reminder_enabled"
	KeyReminderTime  = "reminder_time"
	KeyReminderSent  = "reminder_sent"
	KeyHolidaysOn    = "holidays_enabled"
	KeyHolidayExtra  = "holidays_extra"
	KeyVAPIDPublic   = "vapid_public"
	KeyVAPIDPrivate  = "vapid_private"
)

// Joker modes.
const (
	JokerAuto   = "auto"
	JokerManual = "manual"
)

// Errors shown to the user.
var (
	ErrNotAllowed   = errors.New("not allowed")
	ErrOutOfRange   = errors.New("day out of range")
	ErrInsufficient = errors.New("not enough coins")
	ErrLocked       = errors.New("reward locked")
	ErrJokerLimit   = errors.New("joker limit reached")
)

// Service bundles store and clock.
type Service struct {
	Store   *db.Store
	Loc     *time.Location
	Now     func() time.Time
	settled sync.Map // user id to the day settled last
	rules   atomic.Pointer[game.Rules]
	Pick    func(n int) int // random number below n, replaced in tests
}

// New creates a service using the wall clock.
func New(store *db.Store, loc *time.Location) *Service {
	s := &Service{Store: store, Loc: loc, Now: func() time.Time { return time.Now().In(loc) }}
	s.Pick = s.pickDefault
	s.rules.Store(game.DefaultRules())
	return s
}

// Rules returns the cached global rules.
func (s *Service) Rules() *game.Rules { return s.rules.Load() }

// LoadRules stores missing start values and refreshes the cache. The admin area
// calls it after every change.
func (s *Service) LoadRules(ctx context.Context) error {
	var rules *game.Rules
	err := s.Store.Tx(ctx, func(q *db.Q) error {
		if err := q.SeedRules(game.DefaultRules()); err != nil {
			return err
		}
		if err := q.SeedQuests(game.DefaultQuests(), game.DefaultQuestConfig()); err != nil {
			return err
		}
		var err error
		if rules, err = q.LoadRules(); err != nil {
			return err
		}
		rules.Quests, rules.QuestConfig, err = q.LoadQuests()
		return err
	})
	if err != nil {
		return err
	}
	s.rules.Store(rules)
	return nil
}

// Today is the current local day; it changes at 00:00 local time.
func (s *Service) Today() model.Day { return model.DayOf(s.Now(), s.Loc) }

// World loads everything the game rules look at.
func (s *Service) World(q *db.Q) (*game.World, error) {
	today := s.Today()
	habits, err := q.LoadHabits(s.Loc)
	if err != nil {
		return nil, err
	}
	bal, err := q.Balances(today)
	if err != nil {
		return nil, err
	}
	reds, err := q.Redemptions()
	if err != nil {
		return nil, err
	}
	lastJoker, err := q.LastJokerDay()
	if err != nil {
		return nil, err
	}
	level, err := q.SettingInt(KeyLevel, 1)
	if err != nil {
		return nil, err
	}
	installed, err := q.Setting(KeyInstalledOn, "")
	if err != nil {
		return nil, err
	}
	installedDay, err := model.ParseDay(installed)
	if err != nil {
		installedDay = today
	}
	holiday, err := s.holidays(q)
	if err != nil {
		return nil, err
	}
	questDays, err := q.QuestDays()
	if err != nil {
		return nil, err
	}
	return &game.World{Today: today, Installed: installedDay, Habits: habits, Redemptions: reds, Balances: bal, QuestStreak: QuestStreakAt(questDays, s.QuestDay()),
		Level: s.levelOf(bal.XP, level), LastJoker: lastJoker, Holiday: holiday}, nil
}

// levelOf derives the current level from the XP and the stored level.
func (s *Service) levelOf(xp, stored int) int {
	reached := s.Rules().LevelFor(xp)
	if s.Rules().Settings.NoDrop {
		return max(stored, reached)
	}
	return reached
}

// holidays builds the configured holiday check.
func (s *Service) holidays(q *db.Q) (func(model.Day) bool, error) {
	on, err := q.Setting(KeyHolidaysOn, "1")
	if err != nil {
		return nil, err
	}
	extra, err := q.Setting(KeyHolidayExtra, "")
	if err != nil {
		return nil, err
	}
	dates := strings.FieldsFunc(extra, func(r rune) bool { return r == ',' || r == ' ' })
	return func(d model.Day) bool {
		if on == "1" && game.IsGermanHoliday(d) {
			return true
		}
		return slices.Contains(dates, d.Time().Format("01-02"))
	}, nil
}

// Find returns one habit's data from a world.
func Find(w *game.World, id int64) *game.HabitData {
	for _, h := range w.Habits {
		if h.ID == id {
			return h
		}
	}
	return nil
}

func (s *Service) inRange(d model.Day) error {
	if today := s.Today(); d > today || d < today-game.BackfillDays {
		return ErrOutOfRange
	}
	return nil
}

// noon gives ledger entries of a past day a stable time on that day.
func (s *Service) atDay(d model.Day) time.Time {
	now := s.Now()
	if d == model.DayOf(now, s.Loc) {
		return now
	}
	return time.Date(d.Year(), d.Month(), d.Dom(), 12, 0, 0, 0, s.Loc)
}

func checkinRef(habitID int64, d model.Day) string { return fmt.Sprintf("checkin:%d:%s", habitID, d) }
func avoidRef(habitID int64, d model.Day) string   { return fmt.Sprintf("avoid:%d:%s", habitID, d) }
func questRef(d model.Day) string                  { return "quest:" + d.String() }

// Check toggles a check-in of a build habit, including XP and Taler.
func (s *Service) Check(ctx context.Context, habitID int64, d model.Day) error {
	if err := s.inRange(d); err != nil {
		return err
	}
	return s.Store.Tx(ctx, func(q *db.Q) error {
		w, err := s.World(q)
		if err != nil {
			return err
		}
		h := Find(w, habitID)
		if h == nil {
			return db.ErrNotFound
		}
		if h.Kind != model.KindBuild || h.Archived {
			return ErrNotAllowed
		}
		var trigger *Trigger
		if h.GoalWeek {
			if trigger, err = s.toggleWeekDay(q, w, h, d); err != nil {
				return err
			}
		} else if _, done := h.Done[d]; done {
			if err := s.removeCheckin(q, h, d); err != nil {
				return err
			}
			if err := q.DeleteDayEntries(h.ID, d); err != nil {
				return err
			}
		} else {
			if err := s.fillDay(q, h, d); err != nil {
				return err
			}
			if trigger, err = s.addCheckin(q, w, h, d); err != nil {
				return err
			}
		}
		if err := s.updateQuest(q, w); err != nil {
			return err
		}
		return s.evaluate(q, trigger, false)
	})
}

func (s *Service) removeCheckin(q *db.Q, h *game.HabitData, d model.Day) error {
	if err := q.DeleteCheckin(h.ID, d); err != nil {
		return err
	}
	delete(h.Done, d)
	return s.reverse(q, checkinRef(h.ID, d))
}

// toggleWeekDay clears a day of a weekly goal, or books one click on it. Days are
// only parts of the week here, so a tap never fills the whole goal.
func (s *Service) toggleWeekDay(q *db.Q, w *game.World, h *game.HabitData, d model.Day) (*Trigger, error) {
	if h.Entries[d] > 0 {
		if err := q.DeleteDayEntries(h.ID, d); err != nil {
			return nil, err
		}
		h.Entries[d] = 0
	} else {
		if err := q.AddEntry(model.Entry{HabitID: h.ID, Date: d, Amount: h.Step, CreatedAt: s.atDay(d)}); err != nil {
			return nil, err
		}
		h.Entries[d] = h.Step
	}
	trigger, _, err := s.syncGoal(q, w, h, d)
	return trigger, err
}

// syncGoal aligns the check-in with the entries: reaching the goal adds it,
// falling below takes it back.
func (s *Service) syncGoal(q *db.Q, w *game.World, h *game.HabitData, d model.Day) (*Trigger, bool, error) {
	day, done := h.GoalCheckin(d)
	met := h.GoalTotal(d) >= h.Target
	switch {
	case met && !done:
		trigger, err := s.addCheckin(q, w, h, d)
		return trigger, true, err
	case done && !met:
		return nil, true, s.removeCheckin(q, h, day)
	}
	return nil, false, nil
}

// fillDay tops a counter or quantity habit up to its goal, for days marked done
// in the calendar instead of on the card.
func (s *Service) fillDay(q *db.Q, h *game.HabitData, d model.Day) error {
	if !h.Measured() {
		return nil
	}
	totals, err := q.DayTotals(d)
	if err != nil {
		return err
	}
	if missing := model.RoundAmount(h.Target - totals[h.ID]); missing > 0 {
		return q.AddEntry(model.Entry{HabitID: h.ID, Date: d, Amount: missing, CreatedAt: s.atDay(d)})
	}
	return nil
}

// measured finds a counter or quantity habit that can take entries today.
func (s *Service) measured(q *db.Q, habitID int64) (*game.World, *game.HabitData, error) {
	w, err := s.World(q)
	if err != nil {
		return nil, nil, err
	}
	h := Find(w, habitID)
	if h == nil {
		return nil, nil, db.ErrNotFound
	}
	if !h.Measured() || h.Kind != model.KindBuild || h.Archived {
		return nil, nil, ErrNotAllowed
	}
	return w, h, nil
}

// AddEntry books one click. The check-in with XP, streak and badges follows
// when the click reaches the daily goal.
func (s *Service) AddEntry(ctx context.Context, habitID int64) error {
	return s.Store.Tx(ctx, func(q *db.Q) error {
		w, h, err := s.measured(q, habitID)
		if err != nil {
			return err
		}
		d := w.Today
		if err := q.AddEntry(model.Entry{HabitID: h.ID, Date: d, Amount: h.Step, CreatedAt: s.Now()}); err != nil {
			return err
		}
		h.Entries[d] = model.RoundAmount(h.Entries[d] + h.Step)
		trigger, changed, err := s.syncGoal(q, w, h, d)
		if err != nil {
			return err
		}
		if !changed {
			return s.evaluate(q, nil, false) // amount badges count every click
		}
		if err := s.updateQuest(q, w); err != nil {
			return err
		}
		return s.evaluate(q, trigger, false)
	})
}

// UndoEntry takes back the last click of today, for a weekly goal the last click
// of the week. Falling below the goal also takes back the check-in.
func (s *Service) UndoEntry(ctx context.Context, habitID int64) error {
	return s.Store.Tx(ctx, func(q *db.Q) error {
		w, h, err := s.measured(q, habitID)
		if err != nil {
			return err
		}
		d := w.Today
		for h.GoalWeek && d > d.WeekStart() && h.Entries[d] == 0 {
			d--
		}
		if err := q.DeleteLastEntry(h.ID, d); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				return ErrNotAllowed
			}
			return err
		}
		totals, err := q.DayTotals(d)
		if err != nil {
			return err
		}
		h.Entries[d] = totals[h.ID]
		_, changed, err := s.syncGoal(q, w, h, d)
		if err != nil || !changed {
			return err
		}
		if err := s.updateQuest(q, w); err != nil {
			return err
		}
		return s.evaluate(q, nil, false)
	})
}

// DeleteHabit removes a habit for good. Earned XP and Taler stay.
func (s *Service) DeleteHabit(ctx context.Context, habitID int64) error {
	return s.Store.Tx(ctx, func(q *db.Q) error {
		if _, err := q.Habit(habitID); err != nil {
			return err
		}
		if err := q.DeleteHabit(habitID); err != nil {
			return err
		}
		return s.afterDelete(q)
	})
}

// DeleteAllHabits removes every habit, archived ones included. Earned XP and
// Taler stay.
func (s *Service) DeleteAllHabits(ctx context.Context) error {
	return s.Store.Tx(ctx, func(q *db.Q) error {
		habits, err := q.Habits()
		if err != nil {
			return err
		}
		for _, h := range habits {
			if err := q.DeleteHabit(h.ID); err != nil {
				return err
			}
		}
		return s.afterDelete(q)
	})
}

// Reset starts over: everything earned is gone and the install date moves to
// today, so time-based badges start from zero too. The onboarding runs again.
func (s *Service) Reset(ctx context.Context) error {
	today := s.Today()
	return s.Store.Tx(ctx, func(q *db.Q) error {
		if err := q.ResetProgress(); err != nil {
			return err
		}
		for key, value := range map[string]string{
			KeyInstalledOn: today.String(),
			KeyLastSettled: (today - 1).String(),
			KeyLevel:       "1",
			KeyOnboarded:   "0",
		} {
			if err := q.SetSetting(key, value); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) afterDelete(q *db.Q) error {
	w, err := s.World(q)
	if err != nil {
		return err
	}
	if err := s.updateQuest(q, w); err != nil {
		return err
	}
	return s.evaluate(q, nil, false)
}

func (s *Service) addCheckin(q *db.Q, w *game.World, h *game.HabitData, d model.Day) (*Trigger, error) {
	now := s.Now()
	if h.Joker[d] { // a real check-in replaces the joker and gives it back
		delete(h.Joker, d)
		if w.Balances.Jokers < game.MaxJokers {
			if err := q.AddLedger(now, model.LedgerJoker, 1, "joker_refund", ""); err != nil {
				return nil, err
			}
			w.Balances.Jokers++
		}
	}
	if err := q.Checkin(model.Checkin{HabitID: h.ID, Date: d, CreatedAt: now}); err != nil {
		return nil, err
	}
	h.Done[d] = now
	streak := h.StreakAt(d)
	xp := game.AwardXP(h.Habit, streak.Days)
	at, ref := s.atDay(d), checkinRef(h.ID, d)
	if err := q.AddLedger(at, model.LedgerXP, xp, "checkin", ref); err != nil {
		return nil, err
	}
	if err := q.AddLedger(at, model.LedgerCoins, game.CoinsFor(xp), "checkin", ref); err != nil {
		return nil, err
	}
	if err := s.streakJoker(q, w, h.ID, d, streak.Days); err != nil {
		return nil, err
	}
	return &Trigger{Habit: h.Name, Tag: h.Tag, Days: streak.Days}, nil
}

// streakJoker grants the joker for every seven streak days, once per milestone.
func (s *Service) streakJoker(q *db.Q, w *game.World, habitID int64, d model.Day, days int) error {
	if days == 0 || days%game.JokerInterval != 0 || w.Balances.Jokers >= game.MaxJokers {
		return nil
	}
	ref := fmt.Sprintf("streak:%d:%s:%d", habitID, d, days)
	seen, err := q.LedgerHas(ref)
	if err != nil || seen {
		return err
	}
	w.Balances.Jokers++
	return q.AddLedger(s.Now(), model.LedgerJoker, 1, "streak", ref)
}

// reverse books counter entries for everything booked under a ref. The ledger
// is append-only, so the history of the undo stays visible.
func (s *Service) reverse(q *db.Q, ref string) error {
	sums, err := q.LedgerSum(ref)
	if err != nil {
		return err
	}
	for _, kind := range []string{model.LedgerXP, model.LedgerCoins} {
		if sums[kind] != 0 {
			if err := q.AddLedger(s.Now(), kind, -sums[kind], "undo", ref); err != nil {
				return err
			}
		}
	}
	return nil
}

// Relapse toggles a relapse of an avoid habit. XP are never taken away.
func (s *Service) Relapse(ctx context.Context, habitID int64, d model.Day) error {
	if err := s.inRange(d); err != nil {
		return err
	}
	return s.Store.Tx(ctx, func(q *db.Q) error {
		w, err := s.World(q)
		if err != nil {
			return err
		}
		h := Find(w, habitID)
		if h == nil {
			return db.ErrNotFound
		}
		if h.Kind != model.KindAvoid || h.Archived {
			return ErrNotAllowed
		}
		if i := slices.Index(h.Relapses, d); i >= 0 {
			if err := q.DeleteRelapse(h.ID, d); err != nil {
				return err
			}
			h.Relapses = slices.Delete(h.Relapses, i, i+1)
		} else {
			if err := q.AddRelapse(h.ID, d); err != nil {
				return err
			}
			h.Relapses = append(h.Relapses, d)
			slices.Sort(h.Relapses)
		}
		if err := s.updateQuest(q, w); err != nil {
			return err
		}
		return s.evaluate(q, nil, false)
	})
}

// UseJoker bridges a missed day by hand.
func (s *Service) UseJoker(ctx context.Context, habitID int64, d model.Day) error {
	if err := s.inRange(d); err != nil {
		return err
	}
	return s.Store.Tx(ctx, func(q *db.Q) error {
		w, err := s.World(q)
		if err != nil {
			return err
		}
		h := Find(w, habitID)
		if h == nil {
			return db.ErrNotFound
		}
		if w.Balances.Jokers <= 0 {
			return ErrJokerLimit
		}
		if d >= w.Today {
			return ErrNotAllowed
		}
		if _, done := h.Done[d]; done || h.Joker[d] || h.Kind != model.KindBuild {
			return ErrNotAllowed
		}
		return s.bridge(q, h, d)
	})
}

func (s *Service) bridge(q *db.Q, h *game.HabitData, d model.Day) error {
	now := s.Now()
	if err := q.Checkin(model.Checkin{HabitID: h.ID, Date: d, CreatedAt: now, UsedJoker: true}); err != nil {
		return err
	}
	h.Joker[d] = true
	return q.AddLedger(now, model.LedgerJoker, -1, "joker_use", fmt.Sprintf("bridge:%d:%s", h.ID, d))
}

// Bridge is a missed day a joker could still save.
type Bridge struct {
	Habit *game.HabitData
	Day   model.Day
}

// Bridgeable lists the days in the backfill window that a joker could save.
func Bridgeable(w *game.World) []Bridge {
	var out []Bridge
	for _, h := range w.Habits {
		for d := w.Today - game.BackfillDays; d < w.Today; d++ {
			if day, ok := h.BridgeDay(d); ok {
				out = append(out, Bridge{Habit: h, Day: day})
			}
		}
	}
	return out
}

// Settle books everything that follows from finished days. It is safe to
// call on every request: the work runs once per local day.
func (s *Service) Settle(ctx context.Context) error {
	today := s.Today()
	uid := db.UserID(ctx)
	if uid == 0 {
		return nil
	}
	if last, ok := s.settled.Load(uid); ok && last == today {
		return nil
	}
	err := s.Store.Tx(ctx, func(q *db.Q) error {
		last, err := q.Setting(KeyLastSettled, "")
		if err != nil {
			return err
		}
		lastDay, err := model.ParseDay(last)
		if err != nil {
			lastDay = today - 1
		}
		if lastDay >= today-1 {
			return nil
		}
		w, err := s.World(q)
		if err != nil {
			return err
		}
		mode, err := q.Setting(KeyJokerMode, JokerAuto)
		if err != nil {
			return err
		}
		for d := lastDay + 1; d < today; d++ {
			if err := s.settleDay(q, w, d, mode); err != nil {
				return err
			}
			if err := s.bookQuests(q, w, d, func(def game.QuestDef) bool { return def.Kind == game.QuestAvoid }); err != nil {
				return err
			}
		}
		if err := q.SetSetting(KeyLastSettled, (today - 1).String()); err != nil {
			return err
		}
		return s.evaluate(q, nil, false)
	})
	if err != nil {
		return err
	}
	s.settled.Store(uid, today)
	return nil
}

func (s *Service) settleDay(q *db.Q, w *game.World, d model.Day, mode string) error {
	for _, h := range w.Habits {
		if h.Archived {
			continue
		}
		if h.Kind == model.KindAvoid {
			if err := s.settleAvoid(q, w, h, d); err != nil {
				return err
			}
			continue
		}
		if mode != JokerAuto || w.Balances.Jokers <= 0 {
			continue
		}
		if day, ok := h.BridgeDay(d); ok {
			if err := s.bridge(q, h, day); err != nil {
				return err
			}
			w.Balances.Jokers--
		}
	}
	return nil
}

// settleAvoid pays a finished clean day.
func (s *Service) settleAvoid(q *db.Q, w *game.World, h *game.HabitData, d model.Day) error {
	if d < h.Start || slices.Contains(h.Relapses, d) {
		return nil
	}
	ref := avoidRef(h.ID, d)
	seen, err := q.LedgerHas(ref)
	if err != nil || seen {
		return err
	}
	days := h.StreakAt(d + 1).Days
	xp := game.AwardXP(h.Habit, days)
	at := s.atDay(d)
	if err := q.AddLedger(at, model.LedgerXP, xp, "avoid", ref); err != nil {
		return err
	}
	if err := q.AddLedger(at, model.LedgerCoins, game.CoinsFor(xp), "avoid", ref); err != nil {
		return err
	}
	return s.streakJoker(q, w, h.ID, d, days)
}

// Trigger names what caused a level up, for the celebration text.
type Trigger struct {
	Habit string
	Tag   string
	Days  int
}

// LevelUp is the payload of a level celebration.
type LevelUp struct {
	Level    int    `json:"level"`
	Title    string `json:"title"`
	NewTitle bool   `json:"new_title"`
	Joker    bool   `json:"joker"`
	Coins    int    `json:"coins"`
	Habit    string `json:"habit"`
	Tag      string `json:"tag"`
	Days     int    `json:"days"`
}

// evaluate handles level ups and badges. Bonuses can unlock further badges,
// so it loops until nothing changes. Silent skips bonuses and celebrations.
func (s *Service) evaluate(q *db.Q, trigger *Trigger, silent bool) error {
	for range 8 {
		w, err := s.World(q)
		if err != nil {
			return err
		}
		changed, err := s.applyLevels(q, w, trigger, silent)
		if err != nil {
			return err
		}
		unlocked, err := q.UnlockedBadges()
		if err != nil {
			return err
		}
		known := make(map[string]bool, len(unlocked))
		for k := range unlocked {
			known[k] = true
		}
		fresh := s.Rules().NewlyReached(game.BuildFacts(w), known)
		for _, b := range fresh {
			if err := s.unlock(q, w, b, silent); err != nil {
				return err
			}
		}
		if !changed && len(fresh) == 0 {
			return nil
		}
	}
	return nil
}

func (s *Service) applyLevels(q *db.Q, w *game.World, trigger *Trigger, silent bool) (bool, error) {
	stored, err := q.SettingInt(KeyLevel, 1)
	if err != nil {
		return false, err
	}
	rules := s.Rules()
	reached := rules.LevelFor(w.Balances.XP)
	if reached <= stored {
		if !rules.Settings.NoDrop && reached < stored {
			return false, q.SetSetting(KeyLevel, fmt.Sprint(reached))
		}
		return false, nil
	}
	jokers := w.Balances.Jokers
	for level := stored + 1; level <= reached && !silent; level++ {
		now := s.Now()
		row := rules.Level(level)
		up := LevelUp{Level: level, Title: row.Title, NewTitle: row.Title != rules.Title(level-1), Coins: row.Coins}
		if trigger != nil {
			up.Habit, up.Tag, up.Days = trigger.Habit, trigger.Tag, trigger.Days
		}
		ref := fmt.Sprintf("level:%d", level)
		if row.Coins != 0 {
			if err := q.AddLedger(now, model.LedgerCoins, row.Coins, "level", ref); err != nil {
				return false, err
			}
		}
		for i := 0; i < row.Jokers && jokers < game.MaxJokers; i++ {
			if err := q.AddLedger(now, model.LedgerJoker, 1, "level", ref); err != nil {
				return false, err
			}
			jokers++
			up.Joker = true
		}
		if !rules.Settings.Celebrate {
			continue
		}
		payload, err := json.Marshal(up)
		if err != nil {
			return false, err
		}
		if err := q.AddCelebration("level", string(payload)); err != nil {
			return false, err
		}
	}
	return true, q.SetSetting(KeyLevel, fmt.Sprint(reached))
}

func (s *Service) unlock(q *db.Q, w *game.World, b game.Badge, silent bool) error {
	now := s.Now()
	if err := q.UnlockBadge(b.Key, now); err != nil {
		return err
	}
	if silent {
		return nil
	}
	if b.Coins != 0 {
		if err := q.AddLedger(now, model.LedgerCoins, b.Coins, "badge", "badge:"+b.Key); err != nil {
			return err
		}
	}
	for i := 0; i < b.Jokers && w.Balances.Jokers < game.MaxJokers; i++ {
		if err := q.AddLedger(now, model.LedgerJoker, 1, "badge", "badge:"+b.Key); err != nil {
			return err
		}
		w.Balances.Jokers++
	}
	return q.AddCelebration("badge", b.Key)
}

// RecalculateLevels applies changed thresholds to every user. Nobody gets the
// rewards of levels that were only reached by lowering a threshold.
func (s *Service) RecalculateLevels(ctx context.Context) error {
	users, err := s.Store.Q(ctx).Users()
	if err != nil {
		return err
	}
	for _, u := range users {
		err := s.Store.Tx(db.WithUser(ctx, u.ID), func(q *db.Q) error {
			w, err := s.World(q)
			if err != nil {
				return err
			}
			_, err = s.applyLevels(q, w, nil, true)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// RewardState is a reward with its derived availability.
type RewardState struct {
	model.Reward
	Habit      string
	Available  bool // can be redeemed now
	Missing    int  // Taler missing, for coin rewards
	Progress   int  // streak days, for streak rewards
	Redeemed   bool
	Count      int // how often it was redeemed
	Last       model.Day
	WeekLocked bool // a weekly reward that was already redeemed this week
}

// RewardStates derives the state of all rewards.
func (s *Service) RewardStates(q *db.Q, w *game.World) ([]RewardState, error) {
	rewards, err := q.Rewards()
	if err != nil {
		return nil, err
	}
	out := make([]RewardState, 0, len(rewards))
	for _, r := range rewards {
		out = append(out, s.rewardState(w, r))
	}
	return out, nil
}

func (s *Service) rewardState(w *game.World, r model.Reward) RewardState {
	st := RewardState{Reward: r}
	var last model.Day
	for _, red := range w.Redemptions {
		if red.RewardID == r.ID {
			st.Redeemed = true
			st.Count++
			last = max(last, model.DayOf(red.RedeemedAt, s.Loc))
		}
	}
	st.Last = last
	st.WeekLocked = r.Weekly && last >= w.Today.WeekStart()
	if r.Mode == model.RewardCoins {
		st.Available = w.Balances.Coins >= r.Price && !st.WeekLocked && !r.Paused
		st.Missing = max(r.Price-w.Balances.Coins, 0)
		return st
	}
	if h := Find(w, r.HabitID); h != nil {
		st.Habit = h.Name
		st.Progress = game.RewardProgress(h, w.Today, last, r.ResetOnRelapse)
		st.Available = st.Progress >= r.StreakTarget && !st.WeekLocked && !r.Paused
	}
	return st
}

// Redeem spends a reward.
func (s *Service) Redeem(ctx context.Context, rewardID int64) error {
	_, err := s.Spend(ctx, rewardID, false)
	return err
}

// Spend redeems a reward. With free it counts as enjoyed without any Taler or
// without the streak it asks for.
func (s *Service) Spend(ctx context.Context, rewardID int64, free bool) (model.Redemption, error) {
	var red model.Redemption
	err := s.Store.Tx(ctx, func(q *db.Q) error {
		w, err := s.World(q)
		if err != nil {
			return err
		}
		r, err := q.Reward(rewardID)
		if err != nil {
			return err
		}
		st := s.rewardState(w, r)
		if r.Archived || r.Paused || st.WeekLocked || (st.Redeemed && !r.Repeatable) {
			return ErrNotAllowed
		}
		if !st.Available && !free {
			if r.Mode == model.RewardCoins {
				return ErrInsufficient
			}
			return ErrLocked
		}
		price := 0
		now := s.Now()
		ref := ""
		if r.Mode == model.RewardCoins && !free {
			price = r.Price
			ref = fmt.Sprintf("redeem:%d:%d", r.ID, now.UnixNano())
			if err := q.AddLedger(now, model.LedgerCoins, -price, "redeem", ref); err != nil {
				return err
			}
		}
		id, err := q.AddRedemption(r.ID, now, price, ref)
		if err != nil {
			return err
		}
		red = model.Redemption{ID: id, RewardID: r.ID, Title: r.Title, RedeemedAt: now, PricePaid: price}
		return s.evaluate(q, nil, false)
	})
	return red, err
}

// UndoRedemption takes a redemption back and refunds its Taler. The refund keeps
// the reason of the spending, so it does not count as newly earned.
func (s *Service) UndoRedemption(ctx context.Context, id int64) error {
	return s.Store.Tx(ctx, func(q *db.Q) error {
		red, ref, err := q.Redemption(id)
		if err != nil {
			return err
		}
		if err := q.DeleteRedemption(id); err != nil {
			return err
		}
		if red.PricePaid > 0 {
			if err := q.AddLedger(s.Now(), model.LedgerCoins, red.PricePaid, "redeem", ref); err != nil {
				return err
			}
		}
		return s.evaluate(q, nil, false)
	})
}

// BuyJoker buys a joker in the shop.
func (s *Service) BuyJoker(ctx context.Context) error {
	return s.Store.Tx(ctx, func(q *db.Q) error {
		bal, err := q.Balances(s.Today())
		if err != nil {
			return err
		}
		if bal.Jokers >= game.MaxJokers {
			return ErrJokerLimit
		}
		if bal.Coins < game.JokerPrice {
			return ErrInsufficient
		}
		now := s.Now()
		if err := q.AddLedger(now, model.LedgerCoins, -game.JokerPrice, "joker_buy", ""); err != nil {
			return err
		}
		if err := q.AddLedger(now, model.LedgerJoker, 1, "joker_buy", ""); err != nil {
			return err
		}
		return s.evaluate(q, nil, false)
	})
}

// HabitSaved updates quest and badges after habits changed.
func (s *Service) HabitSaved(ctx context.Context) error {
	return s.Store.Tx(ctx, func(q *db.Q) error {
		w, err := s.World(q)
		if err != nil {
			return err
		}
		if err := s.updateQuest(q, w); err != nil {
			return err
		}
		return s.evaluate(q, nil, false)
	})
}

// Import replaces all data with an export.
func (s *Service) Import(ctx context.Context, e *db.Export) error {
	if e.Version != 1 {
		return fmt.Errorf("unsupported export version %d", e.Version)
	}
	return s.Store.Tx(ctx, func(q *db.Q) error { return q.RestoreAll(e) })
}

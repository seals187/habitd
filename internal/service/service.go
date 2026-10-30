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
	"sync/atomic"
	"time"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

// Setting keys.
const (
	KeyPasswordHash = "password_hash"
	KeyAdminUser    = "admin_user"
	KeyInstalledOn  = "installed_on"
	KeyLevel        = "level"
	KeyLastSettled  = "last_settled"
	KeyJokerMode    = "joker_mode"
	KeyReminderOn   = "reminder_enabled"
	KeyReminderTime = "reminder_time"
	KeyReminderSent = "reminder_sent"
	KeyHolidaysOn   = "holidays_enabled"
	KeyHolidayExtra = "holidays_extra"
	KeyVAPIDPublic  = "vapid_public"
	KeyVAPIDPrivate = "vapid_private"
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
	settled atomic.Int64
}

// New creates a service using the wall clock.
func New(store *db.Store, loc *time.Location) *Service {
	return &Service{Store: store, Loc: loc, Now: func() time.Time { return time.Now().In(loc) }}
}

// Today is the current local day; it changes at 00:00 local time.
func (s *Service) Today() model.Day { return model.DayOf(s.Now(), s.Loc) }

// Init records the first start.
func (s *Service) Init(ctx context.Context) error {
	return s.Store.Tx(ctx, func(q *db.Q) error {
		installed, err := q.Setting(KeyInstalledOn, "")
		if err != nil || installed != "" {
			return err
		}
		if err := q.SetSetting(KeyInstalledOn, s.Today().String()); err != nil {
			return err
		}
		return q.SetSetting(KeyLastSettled, (s.Today() - 1).String())
	})
}

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
	return &game.World{Today: today, Installed: installedDay, Habits: habits, Redemptions: reds, Balances: bal,
		Level: max(level, game.LevelFor(bal.XP)), LastJoker: lastJoker, Holiday: holiday}, nil
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
		if _, done := h.Done[d]; done {
			if err := q.DeleteCheckin(h.ID, d); err != nil {
				return err
			}
			delete(h.Done, d)
			if err := s.reverse(q, checkinRef(h.ID, d)); err != nil {
				return err
			}
		} else {
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

func (s *Service) updateQuest(q *db.Q, w *game.World) error {
	ref := questRef(w.Today)
	sums, err := q.LedgerSum(ref)
	if err != nil {
		return err
	}
	complete := game.QuestFor(w.Habits, w.Today).Complete
	switch {
	case complete && sums[model.LedgerCoins] == 0:
		return q.AddLedger(s.Now(), model.LedgerCoins, game.QuestCoins, "quest", ref)
	case !complete && sums[model.LedgerCoins] != 0:
		return s.reverse(q, ref)
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
	if s.settled.Load() == int64(today) {
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
		}
		if err := q.SetSetting(KeyLastSettled, (today - 1).String()); err != nil {
			return err
		}
		return s.evaluate(q, nil, false)
	})
	if err != nil {
		return err
	}
	s.settled.Store(int64(today))
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
		fresh := game.NewlyReached(game.BuildFacts(w), known)
		for _, b := range fresh {
			if err := s.unlock(q, b, silent); err != nil {
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
	reached := game.LevelFor(w.Balances.XP)
	if reached <= stored {
		return false, nil
	}
	jokers := w.Balances.Jokers
	for level := stored + 1; level <= reached && !silent; level++ {
		now := s.Now()
		up := LevelUp{Level: level, Title: game.Title(level), NewTitle: game.Title(level) != game.Title(level-1)}
		if trigger != nil {
			up.Habit, up.Tag, up.Days = trigger.Habit, trigger.Tag, trigger.Days
		}
		ref := fmt.Sprintf("level:%d", level)
		if err := q.AddLedger(now, model.LedgerCoins, game.LevelUpCoins, "level", ref); err != nil {
			return false, err
		}
		if jokers < game.MaxJokers {
			if err := q.AddLedger(now, model.LedgerJoker, 1, "level", ref); err != nil {
				return false, err
			}
			jokers++
			up.Joker = true
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

func (s *Service) unlock(q *db.Q, b game.Badge, silent bool) error {
	now := s.Now()
	if err := q.UnlockBadge(b.Key, now); err != nil {
		return err
	}
	if silent {
		return nil
	}
	if err := q.AddLedger(now, model.LedgerCoins, game.BadgeCoins, "badge", "badge:"+b.Key); err != nil {
		return err
	}
	return q.AddCelebration("badge", b.Key)
}

// RewardState is a reward with its derived availability.
type RewardState struct {
	model.Reward
	Habit     string
	Available bool // can be redeemed now
	Missing   int  // Taler missing, for coin rewards
	Progress  int  // streak days, for streak rewards
	Redeemed  bool
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
			last = max(last, model.DayOf(red.RedeemedAt, s.Loc))
		}
	}
	if r.Mode == model.RewardCoins {
		st.Available = w.Balances.Coins >= r.Price
		st.Missing = max(r.Price-w.Balances.Coins, 0)
		return st
	}
	if h := Find(w, r.HabitID); h != nil {
		st.Habit = h.Name
		st.Progress = game.RewardProgress(h, w.Today, last, r.ResetOnRelapse)
		st.Available = st.Progress >= r.StreakTarget
	}
	return st
}

// Redeem spends a reward.
func (s *Service) Redeem(ctx context.Context, rewardID int64) error {
	return s.Store.Tx(ctx, func(q *db.Q) error {
		w, err := s.World(q)
		if err != nil {
			return err
		}
		r, err := q.Reward(rewardID)
		if err != nil {
			return err
		}
		st := s.rewardState(w, r)
		if r.Archived || (st.Redeemed && !r.Repeatable) {
			return ErrNotAllowed
		}
		if !st.Available {
			if r.Mode == model.RewardCoins {
				return ErrInsufficient
			}
			return ErrLocked
		}
		price := 0
		now := s.Now()
		if r.Mode == model.RewardCoins {
			price = r.Price
			ref := fmt.Sprintf("redeem:%d:%d", r.ID, now.UnixNano())
			if err := q.AddLedger(now, model.LedgerCoins, -price, "redeem", ref); err != nil {
				return err
			}
		}
		if err := q.AddRedemption(r.ID, now, price); err != nil {
			return err
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

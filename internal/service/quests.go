package service

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

// QuestJokerStreak is the length of a quest streak that pays a joker.
const QuestJokerStreak = 7

// QuestDay is the day quests count for: it starts at the rollover time.
func (s *Service) QuestDay() model.Day {
	now, day := s.Now(), s.Today()
	if now.Format("15:04") < s.Rules().QuestConfig.Rollover {
		return day - 1
	}
	return day
}

// DailyQuest is a drawn quest with its progress.
type DailyQuest struct {
	game.QuestDef
	State  game.QuestState
	Factor int
}

func pickWeighted(pool []game.QuestDef, weighted bool, pick func(int) int) int {
	total := 0
	weight := func(q game.QuestDef) int {
		if weighted {
			return max(q.Weight, 1)
		}
		return 1
	}
	for _, q := range pool {
		total += weight(q)
	}
	roll := pick(total)
	for i, q := range pool {
		if roll -= weight(q); roll < 0 {
			return i
		}
	}
	return 0
}

// draw chooses the quests of a day: planned ones first, the rest from the pool.
func (s *Service) draw(q *db.Q, w *game.World, d model.Day) error {
	rules := s.Rules()
	cfg := rules.QuestConfig
	chosen := []int64{}
	fixed, err := q.PlannedQuests(d)
	if err != nil {
		return err
	}
	chosen = append(chosen, fixed...)
	events, err := q.QuestEvents()
	if err != nil {
		return err
	}
	byID := map[int64]game.QuestDef{}
	for _, def := range rules.Quests {
		byID[def.ID] = def
	}
	if cfg.WeekendRule && d.WeekdayIndex() >= 5 {
		for _, def := range rules.Quests {
			if def.Key == "wochenend" && def.Active {
				chosen = append(chosen, def.ID)
			}
		}
	}
	extra := 0
	for _, e := range events {
		if d >= e.Start && d <= e.End && e.QuestID != 0 && !slices.Contains(chosen, e.QuestID) {
			chosen = append(chosen, e.QuestID)
			extra++
		}
	}
	yesterday, err := q.UserQuests(d - 1)
	if err != nil {
		return err
	}
	var pool []game.QuestDef
	for _, def := range rules.Quests {
		if !def.Active || slices.Contains(chosen, def.ID) || !def.Offered(w, d) || (cfg.Matching && !def.Eligible(w, d)) {
			continue
		}
		pool = append(pool, def)
	}
	if cfg.NoRepeat {
		fresh := slices.DeleteFunc(slices.Clone(pool), func(def game.QuestDef) bool {
			return slices.ContainsFunc(yesterday, func(u db.UserQuest) bool { return u.QuestID == def.ID })
		})
		if len(fresh) > 0 {
			pool = fresh
		}
	}
	for len(chosen)-extra < cfg.PerDay && len(pool) > 0 {
		i := pickWeighted(pool, cfg.Weighted, s.Pick)
		chosen = append(chosen, pool[i].ID)
		pool = slices.Delete(pool, i, i+1)
	}
	for _, id := range chosen {
		if _, ok := byID[id]; !ok {
			continue
		}
		if err := q.AddUserQuest(d, id); err != nil {
			return err
		}
	}
	return nil
}

// questFactor multiplies the rewards of a day.
func (s *Service) questFactor(q *db.Q, d model.Day) (int, error) {
	factor := 1
	if s.Rules().QuestConfig.FirstRule && d.Dom() == 1 {
		factor = 2
	}
	events, err := q.QuestEvents()
	if err != nil {
		return 0, err
	}
	for _, e := range events {
		if d >= e.Start && d <= e.End {
			factor = max(factor, e.Multiplier)
		}
	}
	return factor, nil
}

// EnsureQuests draws the quests of the current day if that has not happened yet.
func (s *Service) EnsureQuests(ctx context.Context) error {
	if db.UserID(ctx) == 0 {
		return nil
	}
	d := s.QuestDay()
	have, err := s.Store.Q(ctx).UserQuests(d)
	if err != nil || len(have) > 0 {
		return err
	}
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

// Quests returns the quests of a day with their progress.
func (s *Service) Quests(q *db.Q, w *game.World, d model.Day) ([]DailyQuest, error) {
	rows, err := q.UserQuests(d)
	if err != nil {
		return nil, err
	}
	factor, err := s.questFactor(q, d)
	if err != nil {
		return nil, err
	}
	out := make([]DailyQuest, 0, len(rows))
	rules := s.Rules()
	for _, def := range rules.Quests {
		if slices.ContainsFunc(rows, func(u db.UserQuest) bool { return u.QuestID == def.ID }) {
			out = append(out, DailyQuest{QuestDef: def, State: def.Eval(w, d), Factor: factor})
		}
	}
	return out, nil
}

// updateQuest draws missing quests and books what changed. The first quest of a
// day keeps the plain reference from before there was a pool.
func (s *Service) updateQuest(q *db.Q, w *game.World) error {
	d := s.QuestDay()
	rows, err := q.UserQuests(d)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		if err := s.draw(q, w, d); err != nil {
			return err
		}
	}
	return s.bookQuests(q, w, d, func(game.QuestDef) bool { return true })
}

func questRefAt(d model.Day, i int) string {
	if i == 0 {
		return questRef(d)
	}
	return fmt.Sprintf("%s:%d", questRef(d), i)
}

// bookQuests pays finished quests of a day and takes back those no longer finished.
func (s *Service) bookQuests(q *db.Q, w *game.World, d model.Day, which func(game.QuestDef) bool) error {
	quests, err := s.Quests(q, w, d)
	if err != nil {
		return err
	}
	for i, dq := range quests {
		if !which(dq.QuestDef) {
			continue
		}
		if err := s.bookQuest(q, w, d, questRefAt(d, i), dq); err != nil {
			return err
		}
	}
	return s.questStreakBonus(q, w, d)
}

func (s *Service) bookQuest(q *db.Q, w *game.World, d model.Day, ref string, dq DailyQuest) error {
	sums, err := q.LedgerSum(ref)
	if err != nil {
		return err
	}
	paid := sums[model.LedgerCoins] != 0 || sums[model.LedgerXP] != 0 || sums[model.LedgerJoker] != 0
	if err := q.SetUserQuestDone(d, dq.ID, dq.State.Complete); err != nil {
		return err
	}
	switch {
	case dq.State.Complete && !paid:
		amount := dq.Reward * dq.Factor
		switch dq.RewardKind {
		case game.RewardXP:
			return q.AddLedger(s.Now(), model.LedgerXP, amount, "quest", ref)
		case game.RewardJoker:
			room := game.MaxJokers - w.Balances.Jokers
			if room <= 0 {
				return nil
			}
			amount = min(amount, room)
			w.Balances.Jokers += amount
			return q.AddLedger(s.Now(), model.LedgerJoker, amount, "quest", ref)
		}
		return q.AddLedger(s.Now(), model.LedgerCoins, amount, "quest", ref)
	case !dq.State.Complete && paid:
		if sums[model.LedgerJoker] != 0 {
			taken := min(sums[model.LedgerJoker], max(w.Balances.Jokers, 0))
			w.Balances.Jokers -= taken
			if err := q.AddLedger(s.Now(), model.LedgerJoker, -sums[model.LedgerJoker], "undo", ref); err != nil {
				return err
			}
		}
		return s.reverse(q, ref)
	}
	return nil
}

// QuestStreakAt counts the finished quest days in a row up to a day. An
// unfinished current day does not break it yet.
func QuestStreakAt(days map[model.Day]bool, d model.Day) int {
	if !days[d] {
		d--
	}
	n := 0
	for days[d-model.Day(n)] {
		n++
	}
	return n
}

// questStreakBonus pays a joker for every seven quests in a row.
func (s *Service) questStreakBonus(q *db.Q, w *game.World, d model.Day) error {
	if !s.Rules().QuestConfig.StreakBonus {
		return nil
	}
	days, err := q.QuestDays()
	if err != nil {
		return err
	}
	streak := QuestStreakAt(days, d)
	if !days[d] || streak == 0 || streak%QuestJokerStreak != 0 || w.Balances.Jokers >= game.MaxJokers {
		return nil
	}
	ref := "queststreak:" + d.String()
	seen, err := q.LedgerHas(ref)
	if err != nil || seen {
		return err
	}
	w.Balances.Jokers++
	return q.AddLedger(s.Now(), model.LedgerJoker, 1, "quest", ref)
}

func (s *Service) pickDefault(n int) int { return rand.IntN(n) }

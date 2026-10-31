package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/service"
	"github.com/seals187/habitd/internal/view"
)

const estimateDays = 14

type rewardRow struct {
	service.RewardState
	Percent int
}

type historyRow struct {
	Title string
	Date  string
	Price int
}

type rewardsData struct {
	Tab        string
	Bal        model.Balances
	Rows       []rewardRow
	History    []historyRow
	JokerPrice int
	MaxJokers  int
	CanBuy     bool
	AtLimit    bool
	Missing    int
}

func (s *Server) rewardsData(r *http.Request) (*rewardsData, error) {
	q := s.svc.Store.Q(r.Context())
	w, err := s.svc.World(q)
	if err != nil {
		return nil, err
	}
	states, err := s.svc.RewardStates(q, w)
	if err != nil {
		return nil, err
	}
	d := &rewardsData{Tab: r.FormValue("tab"), Bal: w.Balances, JokerPrice: game.JokerPrice, MaxJokers: game.MaxJokers}
	if d.Tab != "joker" && d.Tab != "history" {
		d.Tab = "rewards"
	}
	for _, st := range states {
		if st.Archived || (st.Redeemed && !st.Repeatable) {
			continue
		}
		d.Rows = append(d.Rows, rewardRow{RewardState: st, Percent: percent(st.Progress, st.StreakTarget)})
	}
	for _, red := range w.Redemptions {
		d.History = append(d.History, historyRow{Title: red.Title, Date: view.ShortDate(red.RedeemedAt), Price: red.PricePaid})
	}
	d.AtLimit = w.Balances.Jokers >= game.MaxJokers
	d.CanBuy = !d.AtLimit && w.Balances.Coins >= game.JokerPrice
	d.Missing = max(game.JokerPrice-w.Balances.Coins, 0)
	return d, nil
}

func (s *Server) rewards(w http.ResponseWriter, r *http.Request) error {
	d, err := s.rewardsData(r)
	if err != nil {
		return err
	}
	return s.view.Render(w, http.StatusOK, "rewards", s.page(r, "Gönn dir", "rewards", d))
}

func (s *Server) afterShop(w http.ResponseWriter, r *http.Request) error {
	waiting, err := s.svc.Store.Q(r.Context()).HasCelebration()
	if err != nil {
		return err
	}
	if waiting {
		return redirect(w, r, "/celebrate")
	}
	if !isHTMX(r) {
		return redirect(w, r, "/rewards")
	}
	d, err := s.rewardsData(r)
	if err != nil {
		return err
	}
	return s.view.Fragment(w, "rewards", "rewards-body", s.page(r, "Gönn dir", "rewards", d))
}

func (s *Server) rewardRedeem(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.svc.Redeem(r.Context(), id); err != nil {
		return err
	}
	return s.afterShop(w, r)
}

func (s *Server) jokerBuy(w http.ResponseWriter, r *http.Request) error {
	if err := s.svc.BuyJoker(r.Context()); err != nil {
		return err
	}
	return s.afterShop(w, r)
}

type habitOption struct {
	ID   int64
	Name string
	Days int
}

type rewardFormData struct {
	Reward  model.Reward
	New     bool
	Habits  []habitOption
	Presets []int
	Error   string
	Config  string
}

func (s *Server) rewardForm(w http.ResponseWriter, r *http.Request, rw model.Reward, status int, message string) error {
	q := s.svc.Store.Q(r.Context())
	world, err := s.svc.World(q)
	if err != nil {
		return err
	}
	today := s.svc.Today()
	rate, err := q.AverageDailyCoins(today-estimateDays+1, today)
	if err != nil {
		return err
	}
	if rate == 0 { // nothing earned yet: estimate from the active habits
		for _, h := range world.Habits {
			if !h.Archived {
				rate += game.CoinsFor(game.BaseXP(h.Habit))
			}
		}
	}
	d := rewardFormData{Reward: rw, New: rw.ID == 0, Presets: []int{50, 100, 250, 500}, Error: message}
	days := map[string]int{}
	for _, h := range world.Habits {
		if !h.Archived {
			current := h.StreakAt(today).Days
			d.Habits = append(d.Habits, habitOption{ID: h.ID, Name: h.Name, Days: current})
			days[strconv.FormatInt(h.ID, 10)] = current
		}
	}
	config, err := json.Marshal(map[string]any{"mode": rw.Mode, "price": rw.Price, "target": rw.StreakTarget,
		"habit": strconv.FormatInt(rw.HabitID, 10), "rate": rate, "days": days})
	if err != nil {
		return err
	}
	d.Config = string(config)
	title := "Belohnung bearbeiten"
	if d.New {
		title = "Neue Belohnung"
	}
	return s.view.Render(w, status, "reward_form", s.page(r, title, "", d))
}

func (s *Server) rewardNew(w http.ResponseWriter, r *http.Request) error {
	return s.rewardForm(w, r, model.Reward{Mode: model.RewardCoins, Price: 250, StreakTarget: 30, Repeatable: true}, http.StatusOK, "")
}

func (s *Server) rewardEdit(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	rw, err := s.svc.Store.Q(r.Context()).Reward(id)
	if err != nil {
		return err
	}
	return s.rewardForm(w, r, rw, http.StatusOK, "")
}

func (s *Server) rewardSave(w http.ResponseWriter, r *http.Request) error {
	var rw model.Reward
	q := s.svc.Store.Q(r.Context())
	if r.PathValue("id") != "" {
		id, err := pathID(r)
		if err != nil {
			return err
		}
		if rw, err = q.Reward(id); err != nil {
			return err
		}
	}
	rw.Title = strings.TrimSpace(r.FormValue("title"))
	rw.Mode = r.FormValue("mode")
	rw.Repeatable = r.FormValue("repeatable") == "on"
	rw.ResetOnRelapse = r.FormValue("reset") == "on"
	rw.Price, _ = strconv.Atoi(r.FormValue("price"))
	rw.StreakTarget, _ = strconv.Atoi(r.FormValue("target"))
	rw.HabitID, _ = strconv.ParseInt(r.FormValue("habit"), 10, 64)
	if rw.Mode != model.RewardStreak {
		rw.Mode = model.RewardCoins
		rw.HabitID, rw.StreakTarget, rw.ResetOnRelapse = 0, 0, false
	} else {
		rw.Price = 0
	}
	message := ""
	switch {
	case rw.Title == "":
		message = "Bitte gib einen Titel ein"
	case rw.Mode == model.RewardCoins && (rw.Price < 1 || rw.Price > math.MaxInt32):
		message = "Bitte gib einen Preis ein"
	case rw.Mode == model.RewardStreak && !ownsHabit(q, rw.HabitID):
		message = "Bitte wähle ein Habit"
	case rw.Mode == model.RewardStreak && (rw.StreakTarget < 1 || rw.StreakTarget > 10000):
		message = "Bitte gib eine Serienlänge ein"
	}
	if message != "" {
		return s.rewardForm(w, r, rw, http.StatusUnprocessableEntity, message)
	}
	if err := q.SaveReward(rw); err != nil {
		return err
	}
	return redirect(w, r, "/rewards")
}

// ownsHabit keeps rewards from pointing at the habit of another user.
func ownsHabit(q *db.Q, id int64) bool {
	_, err := q.Habit(id)
	return err == nil
}

func (s *Server) rewardArchive(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	q := s.svc.Store.Q(r.Context())
	rw, err := q.Reward(id)
	if err != nil {
		return err
	}
	rw.Archived = true
	if err := q.SaveReward(rw); err != nil {
		return err
	}
	return redirect(w, r, "/rewards")
}

type badgeView struct {
	game.Badge
	Color        string
	Unlocked     bool
	Percent      int
	Value        int
	Label        string
	Rule         string
	ShowProgress bool
}

type badgeSection struct {
	game.Category
	Badges   []badgeView
	Unlocked int
	Total    int
}

type badgeFilter struct {
	Key   string
	Label string
}

type badgesData struct {
	Unlocked int
	Total    int
	Percent  int
	Active   string
	Detail   bool
	Filters  []badgeFilter
	Sections []badgeSection
}

// Sections show at most two rows in the overview, like the design.
const overviewBadges = 8

func (s *Server) badges(w http.ResponseWriter, r *http.Request) error {
	q := s.svc.Store.Q(r.Context())
	world, err := s.svc.World(q)
	if err != nil {
		return err
	}
	unlocked, err := q.UnlockedBadges()
	if err != nil {
		return err
	}
	facts := game.BuildFacts(world)
	active := r.FormValue("cat")
	if _, ok := game.CategoryByKey(active); !ok {
		active = ""
	}
	d := badgesData{Unlocked: len(unlocked), Total: len(game.Badges), Active: active, Detail: active != ""}
	d.Percent = percent(d.Unlocked, d.Total)
	d.Filters = append(d.Filters, badgeFilter{"", fmt.Sprintf("Alle · %d", d.Total)})
	for _, c := range game.Categories {
		d.Filters = append(d.Filters, badgeFilter{c.Key, c.Name})
		if active != "" && c.Key != active {
			continue
		}
		section := badgeSection{Category: c}
		for _, b := range game.Badges {
			if b.Category != c.Key {
				continue
			}
			_, open := unlocked[b.Key]
			section.Total++
			v := badgeView{Badge: b, Color: c.Color, Unlocked: open, Value: b.Value(facts), Label: b.Name}
			v.Percent = percent(v.Value, b.Target)
			v.Rule, v.ShowProgress = b.Rule(), b.Target > 1
			if open {
				section.Unlocked++
				v.Percent, v.ShowProgress = 100, false
			} else if b.Secret {
				v.Label, v.Percent, v.Icon, v.Hint = "???", 0, "lock", ""
				v.Rule, v.ShowProgress = game.SecretRule, false
			}
			if active != "" || len(section.Badges) < overviewBadges {
				section.Badges = append(section.Badges, v)
			}
		}
		d.Sections = append(d.Sections, section)
	}
	return s.view.Render(w, http.StatusOK, "badges", s.page(r, "Abzeichen", "badges", d))
}

func celebrationView(kind, payload string) (*celebrateData, error) {
	d := &celebrateData{Coins: game.BadgeCoins, Confetti: []int{1, 2, 3, 4, 5, 6, 7, 8}}
	if kind == "level" {
		var up service.LevelUp
		if err := json.Unmarshal([]byte(payload), &up); err != nil {
			return nil, fmt.Errorf("decode celebration: %w", err)
		}
		d.Level = &up
		d.Coins = game.LevelUpCoins
		d.Color = "#4FB283"
		switch {
		case up.Tag == "lesen" && up.Days > 1:
			d.Message = fmt.Sprintf("%d Tage gelesen hat sich gelohnt. Das hast du dir verdient:", up.Days)
		case up.Habit != "":
			d.Message = fmt.Sprintf("Dranbleiben bei „%s“ hat sich gelohnt. Das hast du dir verdient:", up.Habit)
		default:
			d.Message = "Dranbleiben hat sich gelohnt. Das hast du dir verdient:"
		}
		return d, nil
	}
	b, ok := game.BadgeByKey(payload)
	if !ok {
		return nil, errors.New("unknown badge " + payload)
	}
	d.Badge = &b
	if c, ok := game.CategoryByKey(b.Category); ok {
		d.Color = c.Color
	}
	return d, nil
}

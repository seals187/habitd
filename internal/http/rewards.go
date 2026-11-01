package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/service"
	"github.com/seals187/habitd/internal/view"
)

const estimateDays = 14

// rewardIcon is an icon offered for rewards, each with its own colour.
type rewardIcon struct {
	Name  string
	Color string
}

var rewardIcons = []rewardIcon{{"moon", "#C79BF2"}, {"cup", "#F2994A"}, {"eye", "#7FB8F0"}, {"meal", "#F2C14E"},
	{"book", "#F28BB0"}, {"gift", "#C79BF2"}, {"trophy", "#8C8CF5"}, {"globe", "#4FB283"}}

func rewardColor(icon string) string {
	for _, i := range rewardIcons {
		if i.Name == icon {
			return i.Color
		}
	}
	return "#F2C14E"
}

// rewardIdea is a suggestion that becomes a reward with one tap.
type rewardIdea struct {
	Key   string
	Title string
	Icon  string
	Color string
	Price int
}

func idea(key, title, icon string, price int) rewardIdea {
	return rewardIdea{Key: key, Title: title, Icon: icon, Color: rewardColor(icon), Price: price}
}

var rewardIdeas = []rewardIdea{idea("kaffee", "Lieblingskaffee", "cup", 80), idea("ausschlafen", "Ausschlafen", "moon", 150),
	idea("buch", "Neues Buch", "book", 300), idea("essen", "Essen gehen", "meal", 400),
	idea("geschenk", "Kleines Geschenk", "gift", 200), idea("ausflug", "Ausflug", "globe", 500)}

type rewardRow struct {
	service.RewardState
	Percent int
	Color   string
	Rhythm  string
	Note    string // what is missing
}

type historyRow struct {
	Title string
	Date  string
	Price int
}

// undoToast offers to take back the redemption that just happened.
type undoToast struct {
	ID   int64
	Text string
	From string
}

type rewardsData struct {
	Tab        string
	Bal        model.Balances
	Rate       int
	Afford     []rewardRow
	Goals      []rewardRow
	Streaks    []rewardRow
	Paused     []rewardRow
	Ideas      []rewardIdea
	History    []historyRow
	JokerPrice int
	MaxJokers  int
	CanBuy     bool
	AtLimit    bool
	Missing    int
	Undo       *undoToast
	Zoom       int // size of the saving goals in percent
}

// GoalScale is the size of the saving goals as a CSS factor.
func (d *rewardsData) GoalScale() float64 { return float64(d.Zoom) / 100 }

func rewardRhythm(rw model.Reward) string {
	switch {
	case rw.Weekly:
		return "1× pro Woche"
	case rw.Repeatable:
		return "mehrfach"
	}
	return "einmalig"
}

// savingNote estimates when a coin reward comes into reach.
func savingNote(missing, rate int) string {
	note := "noch " + view.Number(missing)
	if rate <= 0 {
		return note
	}
	switch days := int(math.Ceil(float64(missing) / float64(rate))); days {
	case 1:
		return note + " · ca. morgen"
	default:
		return fmt.Sprintf("%s · ca. %d Tage", note, days)
	}
}

// coinRate is the average income per day; without any income yet it is estimated
// from the active habits.
func (s *Server) coinRate(q *db.Q, w *game.World) (int, error) {
	today := s.svc.Today()
	rate, err := q.AverageDailyCoins(today-estimateDays+1, today)
	if err != nil || rate > 0 {
		return rate, err
	}
	for _, h := range w.Habits {
		if !h.Archived {
			rate += game.CoinsFor(game.BaseXP(h.Habit))
		}
	}
	return rate, nil
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
	rate, err := s.coinRate(q, w)
	if err != nil {
		return nil, err
	}
	d := &rewardsData{Tab: r.FormValue("tab"), Bal: w.Balances, Rate: rate, JokerPrice: game.JokerPrice, MaxJokers: game.MaxJokers, Zoom: zoomOf(r, "goal_zoom")}
	if d.Tab != "joker" && d.Tab != "history" {
		d.Tab = "rewards"
	}
	for _, st := range states {
		if st.Archived || (st.Redeemed && !st.Repeatable) {
			continue
		}
		row := rewardRow{RewardState: st, Color: rewardColor(st.Icon), Rhythm: rewardRhythm(st.Reward)}
		switch {
		case st.Paused:
			d.Paused = append(d.Paused, row)
		case st.Available || st.WeekLocked:
			d.Afford = append(d.Afford, row)
		case st.Mode == model.RewardCoins:
			row.Percent = percent(w.Balances.Coins, st.Price)
			row.Note = savingNote(st.Missing, rate)
			d.Goals = append(d.Goals, row)
		default:
			row.Percent = percent(st.Progress, st.StreakTarget)
			d.Streaks = append(d.Streaks, row)
		}
	}
	if len(d.Afford)+len(d.Goals)+len(d.Streaks)+len(d.Paused) == 0 {
		d.Ideas = rewardIdeas
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

// afterShop answers a change on a reward, on the screen it came from.
func (s *Server) afterShop(w http.ResponseWriter, r *http.Request, undo *undoToast) error {
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
	if r.FormValue("from") == "today" {
		d, err := s.todayData(r)
		if err != nil {
			return err
		}
		d.Undo = undo
		return s.view.Fragment(w, "today", "today-body", s.page(r, "Heute", "today", d))
	}
	d, err := s.rewardsData(r)
	if err != nil {
		return err
	}
	d.Undo = undo
	return s.view.Fragment(w, "rewards", "rewards-body", s.page(r, "Gönn dir", "rewards", d))
}

func (s *Server) spend(w http.ResponseWriter, r *http.Request, free bool) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	red, err := s.svc.Spend(r.Context(), id, free)
	if err != nil {
		return err
	}
	text := fmt.Sprintf("„%s“ eingelöst", red.Title)
	if red.PricePaid > 0 {
		text += fmt.Sprintf(" · −%s Taler", view.Number(red.PricePaid))
	}
	return s.afterShop(w, r, &undoToast{ID: red.ID, Text: text, From: r.FormValue("from")})
}

func (s *Server) rewardRedeem(w http.ResponseWriter, r *http.Request) error {
	return s.spend(w, r, false)
}

// rewardFree records a reward that was enjoyed without paying for it.
func (s *Server) rewardFree(w http.ResponseWriter, r *http.Request) error { return s.spend(w, r, true) }

func (s *Server) redemptionUndo(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.svc.UndoRedemption(r.Context(), id); err != nil {
		return err
	}
	return s.afterShop(w, r, nil)
}

func (s *Server) jokerBuy(w http.ResponseWriter, r *http.Request) error {
	if err := s.svc.BuyJoker(r.Context()); err != nil {
		return err
	}
	return s.afterShop(w, r, nil)
}

func (s *Server) rewardDuplicate(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	q := s.svc.Store.Q(r.Context())
	rw, err := q.Reward(id)
	if err != nil {
		return err
	}
	rw.ID, rw.Title, rw.Paused = 0, rw.Title+" (Kopie)", false
	if err := q.SaveReward(rw); err != nil {
		return err
	}
	return s.afterShop(w, r, nil)
}

func (s *Server) rewardPause(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.svc.Store.Q(r.Context()).SetRewardPaused(id, r.FormValue("paused") == "on"); err != nil {
		return err
	}
	return s.afterShop(w, r, nil)
}

func (s *Server) rewardDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.svc.Store.Q(r.Context()).DeleteReward(id); err != nil {
		return err
	}
	return s.afterShop(w, r, nil)
}

func (s *Server) rewardIdeaAdd(w http.ResponseWriter, r *http.Request) error {
	i := slices.IndexFunc(rewardIdeas, func(x rewardIdea) bool { return x.Key == r.PathValue("key") })
	if i < 0 {
		return notFound()
	}
	x := rewardIdeas[i]
	rw := model.Reward{Title: x.Title, Icon: x.Icon, Mode: model.RewardCoins, Price: x.Price, Repeatable: true}
	if err := s.svc.Store.Q(r.Context()).SaveReward(rw); err != nil {
		return err
	}
	return s.afterShop(w, r, nil)
}

type habitOption struct {
	ID   int64
	Name string
	Days int
}

type rewardFormData struct {
	Reward model.Reward
	New    bool
	Habits []habitOption
	Icons  []rewardIcon
	Rhythm string
	Used   string // how often it was redeemed
	Error  string
	Config string
}

// usedLabel sums up the redemptions of a reward.
func usedLabel(st service.RewardState) string {
	if st.Count == 0 {
		return ""
	}
	return fmt.Sprintf("%d× eingelöst · zuletzt %s", st.Count, view.LongDateShort(st.Last))
}

func (s *Server) rewardForm(w http.ResponseWriter, r *http.Request, rw model.Reward, status int, message string) error {
	// The fields of the other mode stay in the form, hidden. Their browser
	// validation must not block saving, so they get usable start values.
	if rw.StreakTarget < 1 {
		rw.StreakTarget = 30
	}
	if rw.Price < 1 {
		rw.Price = 250
	}
	q := s.svc.Store.Q(r.Context())
	world, err := s.svc.World(q)
	if err != nil {
		return err
	}
	today := s.svc.Today()
	rate, err := s.coinRate(q, world)
	if err != nil {
		return err
	}
	d := rewardFormData{Reward: rw, New: rw.ID == 0, Icons: rewardIcons, Rhythm: rhythmValue(rw), Error: message}
	if !d.New {
		states, err := s.svc.RewardStates(q, world)
		if err != nil {
			return err
		}
		for _, st := range states {
			if st.ID == rw.ID {
				d.Used = usedLabel(st)
			}
		}
	}
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

func rhythmValue(rw model.Reward) string {
	switch {
	case rw.Weekly:
		return "weekly"
	case rw.Repeatable:
		return "always"
	}
	return "once"
}

func (s *Server) rewardNew(w http.ResponseWriter, r *http.Request) error {
	return s.rewardForm(w, r, model.Reward{Mode: model.RewardCoins, Price: 250, StreakTarget: 30, Repeatable: true, Icon: "gift"}, http.StatusOK, "")
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
	rw := model.Reward{Icon: "gift"}
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
	rhythm := r.FormValue("rhythm")
	rw.Repeatable, rw.Weekly = rhythm == "always" || rhythm == "weekly", rhythm == "weekly"
	rw.ShowToday = r.FormValue("show_today") == "on"
	rw.ResetOnRelapse = r.FormValue("reset") == "on"
	if icon := r.FormValue("icon"); slices.ContainsFunc(rewardIcons, func(i rewardIcon) bool { return i.Name == icon }) {
		rw.Icon = icon
	}
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
	if s.svc.Rules().BadgesOff {
		return notFound()
	}
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
	rules := s.svc.Rules()
	active := r.FormValue("cat")
	if _, ok := rules.CategoryByKey(active); !ok {
		active = ""
	}
	d := badgesData{Active: active, Detail: active != ""}
	// Deactivated badges only stay in view for those who hold them.
	visible := func(b game.Badge) bool {
		_, have := unlocked[b.Key]
		return b.Active || have
	}
	for _, b := range rules.Badges {
		if !visible(b) {
			continue
		}
		d.Total++
		if _, have := unlocked[b.Key]; have {
			d.Unlocked++
		}
	}
	d.Percent = percent(d.Unlocked, d.Total)
	d.Filters = append(d.Filters, badgeFilter{"", fmt.Sprintf("Alle · %d", d.Total)})
	for _, c := range rules.Categories {
		d.Filters = append(d.Filters, badgeFilter{c.Key, c.Name})
		if active != "" && c.Key != active {
			continue
		}
		section := badgeSection{Category: c}
		for _, b := range rules.Badges {
			if b.Category != c.Key || !visible(b) {
				continue
			}
			_, open := unlocked[b.Key]
			section.Total++
			v := badgeView{Badge: b, Color: badgeColor(b, c), Unlocked: open, Value: b.Value(facts), Label: b.Name}
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

func badgeColor(b game.Badge, c game.Category) string {
	if b.Color != "" {
		return b.Color
	}
	return c.Color
}

func (s *Server) celebrationView(kind, payload string) (*celebrateData, error) {
	rules := s.svc.Rules()
	d := &celebrateData{Confetti: []int{1, 2, 3, 4, 5, 6, 7, 8}}
	if kind == "level" {
		var up service.LevelUp
		if err := json.Unmarshal([]byte(payload), &up); err != nil {
			return nil, fmt.Errorf("decode celebration: %w", err)
		}
		d.Level = &up
		d.Coins = up.Coins
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
	b, ok := rules.BadgeByKey(payload)
	if !ok {
		return nil, errors.New("unknown badge " + payload)
	}
	d.Badge = &b
	d.Coins = b.Coins
	if c, ok := rules.CategoryByKey(b.Category); ok {
		d.Color = badgeColor(b, c)
	}
	return d, nil
}

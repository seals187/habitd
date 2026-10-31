package server

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/service"
)

// questCard is a quest of the day.
type questCard struct {
	service.DailyQuest
	Label    string
	Reward   string
	Percent  int
	Deadline string
	Dots     []bool
}

// timeLeft formats the hours until a clock time; ok is false once it has passed.
func timeLeft(now time.Time, until string) (string, bool) {
	t, err := time.ParseInLocation("15:04", until, now.Location())
	if err != nil {
		return "", false
	}
	end := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
	if !end.After(now) {
		return "", false
	}
	left := end.Sub(now)
	return fmt.Sprintf("noch %d:%02d Std.", int(left.Hours()), int(left.Minutes())%60), true
}

func (s *Server) deadline(q service.DailyQuest) string {
	now := s.svc.Now()
	switch {
	case q.State.Complete:
		return "geschafft"
	case q.Kind == game.QuestAvoid:
		return "bis Mitternacht"
	case q.Kind == game.QuestUntil:
		if left, ok := timeLeft(now, q.Until); ok {
			return left
		}
		return "Zeit abgelaufen"
	}
	end := now.Add(24 * time.Hour)
	if left, ok := timeLeft(now, s.svc.Rules().QuestConfig.Rollover); ok {
		return left
	}
	t, err := time.ParseInLocation("15:04", s.svc.Rules().QuestConfig.Rollover, now.Location())
	if err != nil {
		return ""
	}
	next := time.Date(end.Year(), end.Month(), end.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
	left := next.Sub(now)
	return fmt.Sprintf("noch %d:%02d Std.", int(left.Hours()), int(left.Minutes())%60)
}

func (s *Server) questCards(q *db.Q, w *game.World) ([]questCard, error) {
	quests, err := s.svc.Quests(q, w, s.svc.QuestDay())
	if err != nil {
		return nil, err
	}
	out := make([]questCard, 0, len(quests))
	for _, dq := range quests {
		c := questCard{DailyQuest: dq, Label: dq.Label(), Reward: dq.RewardLabel(dq.Factor), Percent: percent(dq.State.Done, dq.State.Total), Deadline: s.deadline(dq)}
		for i := range dq.State.Total {
			c.Dots = append(c.Dots, i < dq.State.Done)
		}
		out = append(out, c)
	}
	return out, nil
}

// streakDay is one day of the quest streak in the app.
type streakDay struct {
	Name  string
	State string // done, missed, today, future or none
	Pie   int
}

type questPage struct {
	Quests []questCard
	Streak int
	Days   []streakDay
	ToNext int
	Bonus  bool
}

func (s *Server) questDetail(w http.ResponseWriter, r *http.Request) error {
	if err := s.svc.EnsureQuests(r.Context()); err != nil {
		return err
	}
	q := s.svc.Store.Q(r.Context())
	world, err := s.svc.World(q)
	if err != nil {
		return err
	}
	cards, err := s.questCards(q, world)
	if err != nil {
		return err
	}
	days, err := q.QuestDays()
	if err != nil {
		return err
	}
	today := s.svc.QuestDay()
	d := questPage{Quests: cards, Streak: service.QuestStreakAt(days, today), Bonus: s.svc.Rules().QuestConfig.StreakBonus}
	d.ToNext = service.QuestJokerStreak - d.Streak%service.QuestJokerStreak
	for i, name := range dayNames {
		x := today.WeekStart() + model.Day(i)
		done, drawn := days[x]
		day := streakDay{Name: name, State: "none"}
		switch {
		case x > today:
			day.State = "future"
		case x == today && !done:
			day.State = "today"
			if len(cards) > 0 {
				day.Pie = cards[0].Percent
			}
		case done:
			day.State = "done"
		case drawn:
			day.State = "missed"
		}
		d.Days = append(d.Days, day)
	}
	return s.view.Render(w, http.StatusOK, "quest", s.page(r, "Tagesquest", "", d))
}

// questChoice is a quest whose habits the user can choose.
type questChoice struct {
	game.QuestDef
	Label  string
	When   string
	Habits []questChoiceHabit
}

type questChoiceHabit struct {
	ID   int64
	Name string
	On   bool
}

// questHabits lets a user choose which habits count for the "all habits" quests.
func (s *Server) questHabits(w http.ResponseWriter, r *http.Request) error {
	q := s.svc.Store.Q(r.Context())
	habits, err := q.Habits()
	if err != nil {
		return err
	}
	skips, err := q.QuestSkips()
	if err != nil {
		return err
	}
	var d []questChoice
	for _, def := range s.svc.Rules().Quests {
		if !def.Active || def.Kind != game.QuestAll {
			continue
		}
		c := questChoice{QuestDef: def, Label: def.Label(), When: whenLabel(def)}
		for _, h := range habits {
			if h.Archived {
				continue
			}
			c.Habits = append(c.Habits, questChoiceHabit{ID: h.ID, Name: h.Name, On: !slices.Contains(skips, db.QuestSkip{QuestID: def.ID, HabitID: h.ID})})
		}
		d = append(d, c)
	}
	return s.view.Render(w, http.StatusOK, "quest_habits", s.page(r, "Quest-Habits", "", d))
}

func (s *Server) questHabitSave(w http.ResponseWriter, r *http.Request) error {
	habit, err := pathID(r)
	if err != nil {
		return err
	}
	quest, err := strconv.ParseInt(r.PathValue("quest"), 10, 64)
	if err != nil {
		return notFound()
	}
	if !slices.ContainsFunc(s.svc.Rules().Quests, func(def game.QuestDef) bool { return def.ID == quest && def.Kind == game.QuestAll }) {
		return notFound()
	}
	if err := s.svc.SetQuestSkip(r.Context(), quest, habit, r.FormValue("on") != "on"); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type questRow struct {
	game.QuestDef
	Label   string
	Reward  string
	When    string
	Weight  []bool
	Percent int
	Low     bool
}

type questsData struct {
	Rows   []questRow
	Total  int
	Active int
	Config game.QuestConfig
	PerDay []int
}

func whenLabel(q game.QuestDef) string {
	switch q.Kind {
	case game.QuestCounter:
		return "nur mit Zähler-Habit"
	case game.QuestCatchUp:
		return "nur wenn gestern verpasst"
	case game.QuestAvoid:
		return "nur mit Vermeiden-Habit"
	}
	switch q.Days {
	case game.AllDays:
		if q.Kind == game.QuestAll {
			return "Täglich möglich"
		}
		return "Täglich"
	case 31:
		return "Mo–Fr"
	case 96:
		return "Sa–So"
	}
	var names []string
	for i, name := range dayNames {
		if q.Days&(1<<i) != 0 {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

func (s *Server) adminQuests(w http.ResponseWriter, r *http.Request) error {
	rules := s.svc.Rules()
	stats, err := s.svc.Store.Q(r.Context()).QuestStats(s.svc.Today() - 29)
	if err != nil {
		return err
	}
	d := questsData{Total: len(rules.Quests), Config: rules.QuestConfig, PerDay: []int{1, 2, 3}}
	for _, q := range rules.Quests {
		st := stats[q.ID]
		row := questRow{QuestDef: q, Label: q.Label(), Reward: q.RewardLabel(1), When: whenLabel(q), Percent: percent(st.Done, st.Drawn)}
		row.Low = row.Percent < 40
		for i := 1; i <= 3; i++ {
			row.Weight = append(row.Weight, i <= q.Weight)
		}
		if q.Active {
			d.Active++
		}
		d.Rows = append(d.Rows, row)
	}
	return s.view.Render(w, http.StatusOK, "admin_quests", s.adminPage(r, "Tagesquests", "quests", d))
}

func (s *Server) adminQuestsSettings(w http.ResponseWriter, r *http.Request) error {
	c := s.svc.Rules().QuestConfig
	perDay, err := strconv.Atoi(r.FormValue("per_day"))
	if err != nil || perDay < 1 || perDay > 3 {
		return badRequest("Quests pro Tag: 1 bis 3")
	}
	rollover := r.FormValue("rollover")
	if !timePattern.MatchString(rollover) {
		return badRequest("Ungültige Uhrzeit")
	}
	c.PerDay, c.Rollover = perDay, rollover
	c.Weighted = r.FormValue("mode") != "equal"
	c.NoRepeat = r.FormValue("no_repeat") == "on"
	c.Matching = r.FormValue("matching") == "on"
	c.StreakBonus = r.FormValue("streak_bonus") == "on"
	if err := s.svc.Store.Q(r.Context()).SaveQuestConfig(c); err != nil {
		return err
	}
	if err := s.svc.LoadRules(r.Context()); err != nil {
		return err
	}
	return redirect(w, r, "/admin/quests")
}

func (s *Server) adminQuestActive(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.svc.Store.Q(r.Context()).SetQuestActive(id, r.FormValue("active") == "on"); err != nil {
		return err
	}
	if err := s.svc.LoadRules(r.Context()); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

var questIcons = []string{"target", "sun", "moon", "layers", "drop", "refresh-cw", "shield-check", "star", "flame", "trophy", "heart", "book", "zap"}

var questKinds = []struct{ Key, Label, Icon string }{
	{game.QuestAll, "Alle Habits", "target"}, {game.QuestUntil, "Bis Uhrzeit", "sun"}, {game.QuestArea, "Bereich", "layers"},
	{game.QuestCounter, "Zähler-Ziel", "chart-column"}, {game.QuestCatchUp, "Nachholen", "refresh-cw"}, {game.QuestAvoid, "Vermeiden", "shield-check"},
}

type questDay struct {
	Bit  int
	Name string
	On   bool
}

type questFormData struct {
	Quest  game.QuestDef
	New    bool
	Kinds  []struct{ Key, Label, Icon string }
	Icons  []string
	Colors []string
	Days   []questDay
	Areas  []model.Area
	Drawn  int
	Quote  int
	Config string
	Error  string
}

func (s *Server) questForm(w http.ResponseWriter, r *http.Request, q game.QuestDef, isNew bool, status int, message string) error {
	areas, err := s.defaultAreas(r)
	if err != nil {
		return err
	}
	stats, err := s.svc.Store.Q(r.Context()).QuestStats(s.svc.Today() - 29)
	if err != nil {
		return err
	}
	st := stats[q.ID]
	d := questFormData{Quest: q, New: isNew, Kinds: questKinds, Icons: questIcons, Colors: badgeColors, Areas: areas, Drawn: st.Drawn, Quote: percent(st.Done, st.Drawn), Error: message}
	for i, name := range dayNames {
		d.Days = append(d.Days, questDay{Bit: i, Name: name, On: q.Days&(1<<i) != 0})
	}
	config := fmt.Sprintf(`{"name":%q,"text":%q,"icon":%q,"color":%q,"kind":%q,"count":"%d","until":%q,"percent":"%d","rewardKind":%q,"reward":"%d","sprite":%q}`,
		q.Name, q.Text, q.Icon, q.Color, q.Kind, q.Count, q.Until, q.Percent, q.RewardKind, q.Reward, "/static/icons/sprite.svg?v="+s.view.Version)
	d.Config = config
	title := "Quest bearbeiten"
	if isNew {
		title = "Neue Quest"
	}
	return s.view.Render(w, status, "admin_quest_form", s.adminPage(r, title, "quests", d))
}

func (s *Server) adminQuestNew(w http.ResponseWriter, r *http.Request) error {
	q := game.QuestDef{Name: "", Text: "Erledige {{anzahl}} Habits vor {{uhrzeit}} Uhr", Icon: "sun", Color: "#F2C14E", Kind: game.QuestUntil, Count: 3, Until: "12:00",
		Percent: 120, RewardKind: game.RewardCoins, Reward: 30, Weight: 2, Days: game.AllDays, Active: true}
	return s.questForm(w, r, q, true, http.StatusOK, "")
}

func (s *Server) findQuest(r *http.Request) (game.QuestDef, error) {
	id, err := pathID(r)
	if err != nil {
		return game.QuestDef{}, err
	}
	for _, q := range s.svc.Rules().Quests {
		if q.ID == id {
			return q, nil
		}
	}
	return game.QuestDef{}, notFound()
}

func (s *Server) adminQuestEdit(w http.ResponseWriter, r *http.Request) error {
	q, err := s.findQuest(r)
	if err != nil {
		return err
	}
	return s.questForm(w, r, q, false, http.StatusOK, "")
}

func parseQuest(r *http.Request, q *game.QuestDef) string {
	q.Name = strings.TrimSpace(r.FormValue("name"))
	q.Text = strings.TrimSpace(r.FormValue("text"))
	q.Icon, q.Color, q.Kind = r.FormValue("icon"), r.FormValue("color"), r.FormValue("kind")
	q.Until, q.Area, q.OnlyArea = r.FormValue("until"), r.FormValue("area"), ""
	q.RewardKind = r.FormValue("reward_kind")
	q.Count, _ = strconv.Atoi(r.FormValue("count"))
	q.Percent, _ = strconv.Atoi(r.FormValue("percent"))
	q.Reward, _ = strconv.Atoi(r.FormValue("reward"))
	q.Weight, _ = strconv.Atoi(r.FormValue("weight"))
	q.MinHabits, q.MinLevel = 0, 0
	if r.FormValue("need_habits") == "on" {
		q.MinHabits, _ = strconv.Atoi(r.FormValue("min_habits"))
	}
	if r.FormValue("need_level") == "on" {
		q.MinLevel, _ = strconv.Atoi(r.FormValue("min_level"))
	}
	if r.FormValue("need_area") == "on" {
		q.OnlyArea = r.FormValue("only_area")
	}
	q.Days = 0
	for _, v := range r.Form["days"] {
		if bit, err := strconv.Atoi(v); err == nil && bit >= 0 && bit < 7 {
			q.Days |= 1 << bit
		}
	}
	switch {
	case q.Name == "" || len(q.Name) > 60:
		return "Bitte gib einen Namen ein"
	case q.Text == "":
		return "Bitte gib einen Text ein"
	case !colorPattern.MatchString(q.Color):
		return "Ungültige Farbe"
	case q.Days == 0:
		return "Bitte wähle mindestens einen Tag"
	case q.Reward < 1 || q.Reward > 100000:
		return "Die Belohnung muss mindestens 1 sein"
	case q.Weight < 1 || q.Weight > 3:
		return "Ungültiges Gewicht"
	case q.Kind == game.QuestUntil && !timePattern.MatchString(q.Until):
		return "Bitte gib eine Uhrzeit an"
	case q.Kind == game.QuestCounter && q.Percent < 1:
		return "Bitte gib einen Prozentwert an"
	case q.Count < 0:
		return "Die Anzahl darf nicht negativ sein"
	}
	if !strings.Contains(" all until area counter catchup avoid ", " "+q.Kind+" ") {
		return "Bitte wähle eine Aufgabe"
	}
	if !strings.Contains(" coins xp joker ", " "+q.RewardKind+" ") {
		return "Bitte wähle eine Belohnung"
	}
	known := false
	for _, icon := range questIcons {
		known = known || icon == q.Icon
	}
	if !known {
		return "Bitte wähle ein Icon"
	}
	return ""
}

func (s *Server) adminQuestSave(w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return badRequest("Ungültiges Formular")
	}
	q := game.QuestDef{Active: true, Sort: len(s.svc.Rules().Quests) + 1}
	isNew := r.PathValue("id") == ""
	if !isNew {
		var err error
		if q, err = s.findQuest(r); err != nil {
			return err
		}
	}
	if message := parseQuest(r, &q); message != "" {
		return s.questForm(w, r, q, isNew, http.StatusUnprocessableEntity, message)
	}
	if _, err := s.svc.Store.Q(r.Context()).SaveQuest(q); err != nil {
		return err
	}
	if err := s.svc.LoadRules(r.Context()); err != nil {
		return err
	}
	return redirect(w, r, "/admin/quests")
}

func (s *Server) adminQuestDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.svc.Store.Q(r.Context()).DeleteQuest(id); err != nil {
		return err
	}
	if err := s.svc.LoadRules(r.Context()); err != nil {
		return err
	}
	return redirect(w, r, "/admin/quests")
}

type planDay struct {
	Date   string
	Title  string
	Quests []game.QuestDef
}

type planEvent struct {
	db.QuestEvent
	Text   string
	Status string
}

type planData struct {
	Title  string
	Week   string
	Prev   string
	Next   string
	Days   []planDay
	Pool   []game.QuestDef
	Events []planEvent
	Config game.QuestConfig
	Today  string
}

func (s *Server) adminQuestPlan(w http.ResponseWriter, r *http.Request) error {
	today := s.svc.Today()
	monday := today.WeekStart() + 7
	if param := r.FormValue("week"); param != "" {
		d, err := model.ParseDay(param)
		if err != nil {
			return badRequest("Ungültige Woche")
		}
		monday = d.WeekStart()
	}
	_, week := monday.Time().ISOWeek()
	rules := s.svc.Rules()
	d := planData{Title: fmt.Sprintf("KW %d · %s", week, rangeLabel(monday, monday+6)), Week: monday.String(), Prev: (monday - 7).String(), Next: (monday + 7).String(), Config: rules.QuestConfig, Today: today.String()}
	for _, q := range rules.Quests {
		if q.Active {
			d.Pool = append(d.Pool, q)
		}
	}
	q := s.svc.Store.Q(r.Context())
	for i, name := range dayNames {
		x := monday + model.Day(i)
		ids, err := q.PlannedQuests(x)
		if err != nil {
			return err
		}
		day := planDay{Date: x.String(), Title: fmt.Sprintf("%s %d.", name, x.Dom())}
		for _, def := range rules.Quests {
			for _, id := range ids {
				if id == def.ID {
					day.Quests = append(day.Quests, def)
				}
			}
		}
		d.Days = append(d.Days, day)
	}
	events, err := q.QuestEvents()
	if err != nil {
		return err
	}
	for _, e := range events {
		row := planEvent{QuestEvent: e, Status: "geplant", Text: rangeLabel(e.Start, e.End)}
		switch {
		case today > e.End:
			row.Status = "vorbei"
		case today >= e.Start:
			row.Status = "läuft"
		}
		for _, def := range rules.Quests {
			if def.ID == e.QuestID {
				row.Text += fmt.Sprintf(" · täglich zusätzlich „%s“", def.Name)
			}
		}
		if e.Multiplier == 2 {
			row.Text += " · doppelte Belohnung"
		} else if e.Multiplier > 2 {
			row.Text += fmt.Sprintf(" · %d-fache Belohnung", e.Multiplier)
		}
		d.Events = append(d.Events, row)
	}
	return s.view.Render(w, http.StatusOK, "admin_quest_plan", s.adminPage(r, "Wochenplan", "quests", d))
}

func planURL(r *http.Request) string { return "/admin/quests/plan?week=" + r.FormValue("week") }

func (s *Server) adminPlanChange(w http.ResponseWriter, r *http.Request) error {
	day, err := model.ParseDay(r.FormValue("date"))
	if err != nil {
		return badRequest("Ungültiges Datum")
	}
	id, err := strconv.ParseInt(r.FormValue("quest"), 10, 64)
	if err != nil {
		return badRequest("Bitte wähle eine Quest")
	}
	q := s.svc.Store.Q(r.Context())
	if r.FormValue("remove") != "" {
		err = q.UnplanQuest(day, id)
	} else {
		err = q.PlanQuest(day, id)
	}
	if err != nil {
		return err
	}
	return redirect(w, r, planURL(r))
}

func (s *Server) adminPlanRules(w http.ResponseWriter, r *http.Request) error {
	c := s.svc.Rules().QuestConfig
	c.WeekendRule = r.FormValue("weekend_rule") == "on"
	c.FirstRule = r.FormValue("first_rule") == "on"
	if err := s.svc.Store.Q(r.Context()).SaveQuestConfig(c); err != nil {
		return err
	}
	if err := s.svc.LoadRules(r.Context()); err != nil {
		return err
	}
	return redirect(w, r, planURL(r))
}

func (s *Server) adminEventAdd(w http.ResponseWriter, r *http.Request) error {
	start, err := model.ParseDay(r.FormValue("start"))
	if err != nil {
		return badRequest("Ungültiger Beginn")
	}
	end, err := model.ParseDay(r.FormValue("end"))
	if err != nil || end < start {
		return badRequest("Ungültiges Ende")
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		return badRequest("Bitte gib einen Namen ein")
	}
	quest, _ := strconv.ParseInt(r.FormValue("quest"), 10, 64)
	multiplier, _ := strconv.Atoi(r.FormValue("multiplier"))
	e := db.QuestEvent{Name: name, Start: start, End: end, QuestID: quest, Multiplier: max(multiplier, 1)}
	if err := s.svc.Store.Q(r.Context()).SaveQuestEvent(e); err != nil {
		return err
	}
	return redirect(w, r, planURL(r))
}

func (s *Server) adminEventDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	if err := s.svc.Store.Q(r.Context()).DeleteQuestEvent(id); err != nil {
		return err
	}
	return redirect(w, r, planURL(r))
}

package game

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/seals187/habitd/internal/model"
)

// Quest kinds.
const (
	QuestAll     = "all"     // everything due today
	QuestUntil   = "until"   // a number of habits before a time
	QuestArea    = "area"    // habits of an area, by default the weakest of the week
	QuestCounter = "counter" // a counter or quantity habit beyond its goal
	QuestCatchUp = "catchup" // a habit that was missed yesterday
	QuestAvoid   = "avoid"   // no relapse all day, decided once the day is over
)

// Quest rewards.
const (
	RewardCoins = "coins"
	RewardXP    = "xp"
	RewardJoker = "joker"
)

// AllDays is the mask of a quest without limits on the weekday.
const AllDays = 127

// QuestDef is one quest of the pool.
type QuestDef struct {
	ID         int64
	Key        string // stable name of the built-in quests
	Name       string
	Text       string // with {{anzahl}}, {{uhrzeit}}, {{prozent}} and {{bereich}}
	Icon       string
	Color      string
	Kind       string
	Count      int    // habits to finish, 0 means all
	Until      string // HH:MM
	Area       string // empty means the weakest area of the week
	Percent    int
	RewardKind string
	Reward     int
	Weight     int // 1 rarely, 2 normal, 3 often
	Days       int // weekday mask, bit 0 is Monday
	MinHabits  int
	MinLevel   int
	OnlyArea   string
	Active     bool
	Sort       int
}

// QuestConfig holds how quests are drawn.
type QuestConfig struct {
	PerDay      int
	Rollover    string // HH:MM, the quest day starts here
	Weighted    bool
	NoRepeat    bool
	Matching    bool // only quests that can be done with the habits at hand
	StreakBonus bool
	WeekendRule bool
	FirstRule   bool // twice the reward on the first day of a month
}

// QuestItem is a habit that counts for a quest.
type QuestItem struct {
	Name string
	Note string
	Done bool
}

// QuestState is the progress of a quest on one day.
type QuestState struct {
	Done     int
	Total    int
	Pool     int // habits that can count at all
	Complete bool
	Items    []QuestItem
}

// DefaultQuestConfig are the start values of the draw.
func DefaultQuestConfig() QuestConfig {
	return QuestConfig{PerDay: 1, Rollover: "04:00", Weighted: true, NoRepeat: true, Matching: true,
		StreakBonus: true, WeekendRule: true, FirstRule: true}
}

// DefaultQuests is the start pool.
func DefaultQuests() []QuestDef {
	base := func(q QuestDef) QuestDef {
		if q.Days == 0 {
			q.Days = AllDays
		}
		if q.Weight == 0 {
			q.Weight = 2
		}
		if q.RewardKind == "" {
			q.RewardKind = RewardCoins
		}
		q.Active = true
		return q
	}
	out := []QuestDef{
		base(QuestDef{Key: "alles", Name: "Alles erledigt", Text: "Alle Habits des Tages abhaken", Icon: "target", Color: "#4FB283", Kind: QuestAll, Reward: QuestCoins, Weight: 3}),
		base(QuestDef{Key: "frueher-vogel", Name: "Früher Vogel", Text: "Erledige {{anzahl}} Habits vor {{uhrzeit}} Uhr", Icon: "sun", Color: "#F2C14E", Kind: QuestUntil, Count: 3, Until: "12:00", Reward: 30, Days: 31}),
		base(QuestDef{Key: "vernachlaessigt", Name: "Vernachlässigt?", Text: "{{anzahl}} Habit aus dem schwächsten Bereich der Woche", Icon: "layers", Color: "#C79BF2", Kind: QuestArea, Count: 1, Reward: 40}),
		base(QuestDef{Key: "ueberflieger", Name: "Überflieger", Text: "Ein Zähler-Habit auf {{prozent}} % bringen", Icon: "drop", Color: "#5FD3C6", Kind: QuestCounter, Percent: 120, Reward: 25, Weight: 1}),
		base(QuestDef{Key: "zurueck", Name: "Zurück in die Spur", Text: "Habit erledigen, das gestern verpasst wurde", Icon: "refresh-cw", Color: "#F2994A", Kind: QuestCatchUp, Reward: 30}),
		base(QuestDef{Key: "sauber", Name: "Sauber bleiben", Text: "Alle Vermeiden-Habits heute ohne Rückfall", Icon: "shield-check", Color: "#7FB8F0", Kind: QuestAvoid, Reward: 30, Weight: 1}),
		base(QuestDef{Key: "wochenend", Name: "Wochenend-Held", Text: "Alle Habits am Samstag und Sonntag", Icon: "star", Color: "#E8A0E8", Kind: QuestAll, RewardKind: RewardJoker, Reward: 1, Days: 96, Weight: 1}),
		base(QuestDef{Key: "abendruhe", Name: "Abendruhe", Text: "Kein Bildschirm-Habit nach 22 Uhr verletzt", Icon: "moon", Color: "#8C8CF5", Kind: QuestAvoid, Area: "digital", Reward: 20, Weight: 1}),
	}
	out[len(out)-1].Active = false
	for i := range out {
		out[i].Sort = i + 1
	}
	return out
}

// Label is the text shown in the app, with its placeholders filled in.
func (q QuestDef) Label() string {
	count := strconv.Itoa(q.Count)
	if q.Count == 0 {
		count = "alle"
	}
	return strings.NewReplacer("{{anzahl}}", count, "{{uhrzeit}}", q.Until, "{{prozent}}", strconv.Itoa(q.Percent), "{{bereich}}", q.Area).Replace(q.Text)
}

// RewardLabel formats the reward, with an event or rule factor applied.
func (q QuestDef) RewardLabel(factor int) string {
	amount := q.Reward * max(factor, 1)
	switch q.RewardKind {
	case RewardJoker:
		return fmt.Sprintf("+%d Joker", amount)
	case RewardXP:
		return fmt.Sprintf("+%d XP", amount)
	}
	return fmt.Sprintf("+%d Taler", amount)
}

// Formula is the task in short form for the admin area.
func (q QuestDef) Formula() string {
	need := "all"
	if q.Count > 0 {
		need = strconv.Itoa(q.Count)
	}
	switch q.Kind {
	case QuestUntil:
		return fmt.Sprintf("count(checkins where time < %s) >= %s", q.Until, need)
	case QuestArea:
		area := q.Area
		if area == "" {
			area = "weakest"
		}
		return fmt.Sprintf("count(checkins where area = %s) >= %s", area, need)
	case QuestCounter:
		return fmt.Sprintf("any(counter >= %d%%)", q.Percent)
	case QuestCatchUp:
		return "any(checkin where missed yesterday)"
	case QuestAvoid:
		return "count(relapses) = 0"
	}
	return "count(checkins) >= all"
}

// Offered reports whether a quest may be drawn on a day at all.
func (q QuestDef) Offered(w *World, d model.Day) bool {
	if q.Days&(1<<d.WeekdayIndex()) == 0 || w.Level < q.MinLevel {
		return false
	}
	active, inArea := 0, q.OnlyArea == ""
	for _, h := range w.Habits {
		if h.Archived {
			continue
		}
		active++
		inArea = inArea || h.AreaKey == q.OnlyArea
	}
	return active >= q.MinHabits && inArea
}

func running(h *HabitData, d model.Day) bool { return !h.Archived && d >= h.Start }

func noteFor(h *HabitData, d model.Day) string {
	if at, ok := h.Done[d]; ok {
		if model.DayOf(at, at.Location()) == d {
			return at.Format("15:04")
		}
		return "erledigt"
	}
	if h.Measured() && h.Entries[d] > 0 {
		return fmt.Sprintf("%s / %s %s", amount(h.Entries[d]), amount(h.Target), h.Unit)
	}
	return "offen"
}

func amount(v float64) string {
	return strings.ReplaceAll(strconv.FormatFloat(v, 'f', -1, 64), ".", ",")
}

func beforeTime(h *HabitData, d model.Day, until string) bool {
	at, ok := h.Done[d]
	return ok && model.DayOf(at, at.Location()) == d && at.Format("15:04") < until
}

// weakestArea is the area with the lowest share of done days this week.
func weakestArea(w *World, d model.Day) string {
	done, due := map[string]int{}, map[string]int{}
	for _, h := range w.Habits {
		if !running(h, d) || h.Kind != model.KindBuild {
			continue
		}
		for x := d.WeekStart(); x <= d; x++ {
			if h.DueOn(x) || x == d {
				due[h.AreaKey]++
			}
			if _, ok := h.Done[x]; ok && x < d {
				done[h.AreaKey]++
			}
		}
	}
	keys := make([]string, 0, len(due))
	for k := range due {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	best, rate := "", math.Inf(1)
	for _, k := range keys {
		if r := float64(done[k]) / float64(due[k]); r < rate {
			best, rate = k, r
		}
	}
	return best
}

// Eval computes the progress of a quest on a day.
func (q QuestDef) Eval(w *World, d model.Day) QuestState {
	var st QuestState
	builds := 0
	add := func(h *HabitData, done bool) {
		st.Pool++
		st.Items = append(st.Items, QuestItem{Name: h.Name, Note: noteFor(h, d), Done: done})
		if done {
			st.Done++
		}
	}
	area := q.Area
	if q.Kind == QuestArea && area == "" {
		area = weakestArea(w, d)
	}
	for _, h := range w.Habits {
		if !running(h, d) {
			continue
		}
		_, done := h.Done[d]
		build := h.Kind == model.KindBuild
		switch q.Kind {
		case QuestAll:
			switch {
			case build && h.DueOn(d):
				builds++
				add(h, done)
			case !build:
				clean := !slices.Contains(h.Relapses, d)
				add(h, clean)
				if clean {
					st.Items[len(st.Items)-1].Note = "bis jetzt sauber"
				} else {
					st.Items[len(st.Items)-1].Note = "Rückfall"
				}
			}
		case QuestUntil:
			if build && h.DueOn(d) {
				add(h, beforeTime(h, d, q.Until))
			}
		case QuestArea:
			if build && h.AreaKey == area {
				add(h, done)
			}
		case QuestCounter:
			if build && h.Measured() {
				add(h, h.Entries[d] >= h.Target*float64(q.Percent)/100)
			}
		case QuestCatchUp:
			if _, had := h.Done[d-1]; build && !had && h.Scheduled(d-1) && !h.Joker[d-1] {
				add(h, done)
			}
		case QuestAvoid:
			if !build && (q.Area == "" || h.AreaKey == q.Area) {
				clean := !slices.Contains(h.Relapses, d)
				add(h, clean && d < w.Today)
				if !clean {
					st.Items[len(st.Items)-1].Note = "Rückfall"
				} else if d >= w.Today {
					st.Items[len(st.Items)-1].Note = "bis jetzt sauber"
				}
			}
		}
	}
	switch {
	case q.Kind == QuestAll && builds == 0:
		st.Total = 0
	case q.Kind == QuestCounter || q.Kind == QuestCatchUp:
		st.Total = 1
	case q.Count > 0 && q.Kind != QuestAll && q.Kind != QuestAvoid:
		st.Total = q.Count
	default:
		st.Total = st.Pool
	}
	st.Complete = st.Total > 0 && st.Done >= st.Total
	st.Done = min(st.Done, st.Total)
	return st
}

// Eligible reports whether a quest can be done with the habits at hand.
func (q QuestDef) Eligible(w *World, d model.Day) bool {
	st := q.Eval(w, d)
	return st.Total > 0 && st.Pool >= st.Total
}

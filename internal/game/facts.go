package game

import (
	"slices"
	"time"

	"github.com/seals187/habitd/internal/model"
)

// World is everything the badge rules can look at.
type World struct {
	Today       model.Day
	Installed   model.Day
	Habits      []*HabitData // including archived ones
	Redemptions []model.Redemption
	Balances    model.Balances
	Level       int
	LastJoker   model.Day // last day a joker was used, 0 if never
	Holiday     func(model.Day) bool
}

// Facts are the numbers all badge rules are written against.
type Facts struct {
	BestStreak      int // best current streak of a build habit, in active days
	Checkins        int
	ActiveHabits    int
	PerfectRun      int // longest run of days with all habits done
	AvoidBest       int // best current clean streak
	AvoidsAt30      int
	ComebackDays    int // best clean streak after a relapse
	Phoenix         bool
	DaysNoJoker     int
	Jokers          int
	EarlyCheckins   int
	LateCheckins    int
	Weekends        int
	HolidayCheckins int
	MonthsActive    int
	Seasons         int
	HealthAt30      int
	AreaCount       map[string]int
	AreaStreak      map[string]int // best current streak per area key, in days
	AreaSpan        map[string]int // days between first and last check-in
	TagCount        map[string]int
	TagStreak       map[string]int
	TagDays         map[string]int // check-ins plus finished clean days
	AreaDays        map[string]int // check-ins plus finished clean days
	AvoidAreaStreak map[string]int // best clean streak of the area's avoid habits
	DigitalClean    int            // digital avoid habits with at least one clean day in a row
	Redeemed        int
	MaxRedeemPrice  int
	Coins           int
	CoinsEarned     int
	Level           int
	Midnight        bool
	PiDay           bool
	Palindrome      bool // streak of exactly 111 or 222
	DaysInstalled   int
	Blitz           bool
	Habits          []habitFact // the base of the rules built in the admin area
}

// habitFact holds what a configurable rule can ask about one habit.
type habitFact struct {
	kind, area, tag string
	archived        bool
	days            int // current streak in days, bridged days included
	active          int // days with a real check-in inside the streak
	done            int
	jokers          int
	amount          int
}

func (h habitFact) matches(m Metric) bool {
	switch m.Scope {
	case ScopeHabitTag:
		return h.tag == m.Value
	case ScopeArea:
		return h.area == m.Value
	case ScopeKindBuild:
		return h.kind == model.KindBuild
	case ScopeKindAvoid:
		return h.kind == model.KindAvoid
	}
	return true
}

// bestStreak is the longest current streak among the matching active habits.
func (f *Facts) bestStreak(m Metric) int {
	best := 0
	for _, h := range f.Habits {
		if h.archived || !h.matches(m) {
			continue
		}
		if m.Jokers || h.kind == model.KindAvoid {
			best = max(best, h.days)
		} else {
			best = max(best, h.active)
		}
	}
	return best
}

// count sums the check-ins of the matching habits, archived ones included.
func (f *Facts) count(m Metric) int {
	sum := 0
	for _, h := range f.Habits {
		if !h.matches(m) {
			continue
		}
		sum += h.done
		if m.Jokers {
			sum += h.jokers
		}
	}
	return sum
}

// amount sums what counter and quantity habits have recorded.
func (f *Facts) amount(m Metric) int {
	sum := 0
	for _, h := range f.Habits {
		if h.matches(m) {
			sum += h.amount
		}
	}
	return sum
}

// BuildFacts derives all facts from the world.
func BuildFacts(w *World) *Facts {
	f := &Facts{
		AreaCount: map[string]int{}, AreaStreak: map[string]int{}, AreaSpan: map[string]int{},
		TagCount: map[string]int{}, TagStreak: map[string]int{}, TagDays: map[string]int{}, AreaDays: map[string]int{},
		AvoidAreaStreak: map[string]int{},
		Jokers:          w.Balances.Jokers, Coins: w.Balances.Coins, CoinsEarned: w.Balances.CoinsEarned,
		Level: w.Level, Redeemed: len(w.Redemptions), DaysInstalled: int(w.Today - w.Installed),
	}
	for _, r := range w.Redemptions {
		f.MaxRedeemPrice = max(f.MaxRedeemPrice, r.PricePaid)
	}
	f.DaysNoJoker = int(w.Today - max(w.Installed, w.LastJoker))

	months := map[int]bool{}
	for _, h := range w.Habits {
		s := h.StreakAt(w.Today)
		f.Habits = append(f.Habits, habitFact{kind: h.Kind, area: h.AreaKey, tag: h.Tag, archived: h.Archived,
			days: s.Days, active: s.Active, done: len(h.Done), jokers: len(h.Joker), amount: h.Amount})
		f.addCheckins(h, months, w)
		f.addCleanDays(h, w.Today)
		if h.Archived {
			continue
		}
		f.ActiveHabits++
		f.addStreak(h, w.Today)
	}
	f.MonthsActive = longestMonthRun(months)
	f.PerfectRun, f.Weekends, f.Blitz = scanDays(w)
	f.Seasons = goodSeasons(w)
	return f
}

func (f *Facts) addCheckins(h *HabitData, months map[int]bool, w *World) {
	var first, last model.Day
	for d, at := range h.Done {
		f.Checkins++
		f.AreaCount[h.AreaKey]++
		f.AreaDays[h.AreaKey]++
		if h.Tag != "" {
			f.TagCount[h.Tag]++
			f.TagDays[h.Tag]++
		}
		months[d.Year()*12+int(d.Month())] = true
		if first == 0 || d < first {
			first = d
		}
		last = max(last, d)
		if w.Holiday != nil && w.Holiday(d) {
			f.HolidayCheckins++
		}
		if d.Month() == time.March && d.Dom() == 14 {
			f.PiDay = true
		}
		// The time of day only means something for live check-ins, not backfills.
		if model.DayOf(at, at.Location()) != d {
			continue
		}
		hour := at.Hour()
		switch {
		case hour < 7:
			f.EarlyCheckins++
		case hour >= 22:
			f.LateCheckins++
		}
		if hour == 0 && (at.Minute() < 5 || at.Minute() == 5 && at.Second() == 0) {
			f.Midnight = true
		}
	}
	if len(h.Done) > 0 {
		f.AreaSpan[h.AreaKey] = max(f.AreaSpan[h.AreaKey], int(last-first))
	}
}

func (f *Facts) addCleanDays(h *HabitData, today model.Day) {
	if h.Kind != model.KindAvoid {
		return
	}
	clean := 0
	for _, r := range h.StreakAt(today).Runs {
		clean += r.Length
	}
	f.AreaDays[h.AreaKey] += clean
	if h.Tag != "" {
		f.TagDays[h.Tag] += clean
	}
}

func (f *Facts) addStreak(h *HabitData, today model.Day) {
	s := h.StreakAt(today)
	f.AreaStreak[h.AreaKey] = max(f.AreaStreak[h.AreaKey], s.Days)
	if h.Tag != "" {
		f.TagStreak[h.Tag] = max(f.TagStreak[h.Tag], s.Days)
	}
	if s.Days == 111 || s.Days == 222 {
		f.Palindrome = true
	}
	if h.AreaKey == "gesundheit" && s.Days >= 30 {
		f.HealthAt30++
	}
	if h.Kind == model.KindBuild {
		f.BestStreak = max(f.BestStreak, s.Active)
		return
	}
	f.AvoidBest = max(f.AvoidBest, s.Days)
	f.AvoidAreaStreak[h.AreaKey] = max(f.AvoidAreaStreak[h.AreaKey], s.Days)
	if h.AreaKey == "digital" && s.Days > 0 {
		f.DigitalClean++
	}
	if s.Days >= 30 {
		f.AvoidsAt30++
	}
	if len(h.Relapses) == 0 {
		return
	}
	f.ComebackDays = max(f.ComebackDays, s.Days)
	before := 0
	for _, r := range s.Runs[:max(len(s.Runs)-1, 0)] {
		before = max(before, r.Length)
	}
	if s.Current > before && before > 0 {
		f.Phoenix = true
	}
}

func longestMonthRun(months map[int]bool) int {
	keys := make([]int, 0, len(months))
	for m := range months {
		keys = append(keys, m)
	}
	slices.Sort(keys)
	best, run := 0, 0
	for i, m := range keys {
		if i > 0 && m == keys[i-1]+1 {
			run++
		} else {
			run = 1
		}
		best = max(best, run)
	}
	return best
}

// perfectMinHabits: "all habits" means nothing with a single one.
const perfectMinHabits = 2

// dayState tells how a day went across all active habits.
type dayState struct {
	habits   int  // active habits on that day
	planned  int  // habits with a fixed plan for that day
	complete bool // all planned habits done and no relapse
	weeksMet bool // all weekly rhythms reached their target
	early    bool // all planned habits done before 8:00
}

func evalDay(w *World, d model.Day) dayState {
	s := dayState{complete: true, weeksMet: true, early: true}
	for _, h := range w.Habits {
		if h.Archived || d < h.Start {
			continue
		}
		s.habits++
		if h.Kind == model.KindAvoid {
			if slices.Contains(h.Relapses, d) {
				s.complete = false
			}
			continue
		}
		if h.Frequency == model.FreqMonthly {
			continue
		}
		if h.Frequency == model.FreqWeekly {
			if !h.WeekMet(d.WeekStart()) {
				s.weeksMet = false
			}
			continue
		}
		if !h.Scheduled(d) {
			continue
		}
		s.planned++
		at, ok := h.Done[d]
		if !ok {
			s.complete, s.early = false, false
			continue
		}
		if model.DayOf(at, at.Location()) != d || at.Hour() >= 8 {
			s.early = false
		}
	}
	return s
}

// scanDays walks the whole history once for the day-based facts.
func scanDays(w *World) (perfectRun, weekends int, blitz bool) {
	first := w.Today
	for _, h := range w.Habits {
		first = min(first, h.Start)
	}
	run := 0
	var saturdayOK bool
	for d := first; d <= w.Today; d++ {
		s := evalDay(w, d)
		perfect := s.complete && s.weeksMet && s.planned > 0 && s.habits >= perfectMinHabits
		if perfect {
			run++
			perfectRun = max(perfectRun, run)
		} else if d != w.Today {
			run = 0
		}
		good := s.complete && s.planned > 0
		switch d.WeekdayIndex() {
		case 5:
			saturdayOK = good
		case 6:
			if saturdayOK && good {
				weekends++
			}
		}
		if good && s.early {
			blitz = true
		}
	}
	return perfectRun, weekends, blitz
}

// minSeasonDays keeps a season from counting after just a few lucky days.
const minSeasonDays = 60

// season maps a day to a year-specific season key and the season index 0-3.
func season(d model.Day) (key, index int) {
	y, m := d.Year(), int(d.Month())
	index = (m % 12) / 3 // Dec-Feb 0, Mar-May 1, Jun-Aug 2, Sep-Nov 3
	if m == 12 {
		y++
	}
	return y*4 + index, index
}

// goodSeasons counts the seasons of the year with a completion rate of at
// least 80 percent in any year. Only habits with fixed days are measured.
func goodSeasons(w *World) int {
	type tally struct{ planned, done, index int }
	seasons := map[int]*tally{}
	for _, h := range w.Habits {
		if h.Kind != model.KindBuild || h.Frequency == model.FreqWeekly {
			continue
		}
		for d := h.Start; d <= w.Today; d++ {
			if !h.Scheduled(d) {
				continue
			}
			key, index := season(d)
			t := seasons[key]
			if t == nil {
				t = &tally{index: index}
				seasons[key] = t
			}
			t.planned++
			if _, ok := h.Done[d]; ok {
				t.done++
			}
		}
	}
	reached := map[int]bool{}
	for _, t := range seasons {
		if t.planned >= minSeasonDays && t.done*100 >= t.planned*80 {
			reached[t.index] = true
		}
	}
	return len(reached)
}

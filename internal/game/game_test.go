package game

import (
	"testing"
	"time"

	"github.com/seals187/habitd/internal/model"
)

func day(s string) model.Day {
	d, err := model.ParseDay(s)
	if err != nil {
		panic(err)
	}
	return d
}

func daily(start string, done ...string) *HabitData {
	h := NewHabitData(model.Habit{Kind: model.KindBuild, Frequency: model.FreqDaily, Difficulty: model.Medium}, day(start))
	for _, d := range done {
		mark(h, day(d))
	}
	return h
}

func mark(h *HabitData, d model.Day) {
	h.Done[d] = time.Date(d.Year(), d.Month(), d.Dom(), 19, 0, 0, 0, time.UTC)
}

func markRange(h *HabitData, from, to model.Day) {
	for d := from; d <= to; d++ {
		mark(h, d)
	}
}

func TestDayArithmetic(t *testing.T) {
	if got := day("2026-10-30").String(); got != "2026-10-30" {
		t.Fatal(got)
	}
	if got := day("2026-10-30").WeekdayIndex(); got != 4 {
		t.Fatalf("friday index = %d", got)
	}
	if got := day("2024-02-29") + 1; got != day("2024-03-01") {
		t.Fatalf("leap day: %v", got)
	}
	if got := day("2026-01-01").WeekStart(); got != day("2025-12-29") {
		t.Fatalf("week start across years: %v", got)
	}
}

func TestDayOfRespectsTimezone(t *testing.T) {
	zurich, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		t.Skip("no tzdata")
	}
	instant := time.Date(2026, 3, 28, 23, 30, 0, 0, time.UTC) // 00:30 local on the 29th, DST night
	if got := model.DayOf(instant, zurich); got != day("2026-03-29") {
		t.Fatalf("local day = %v", got)
	}
	if got := model.DayOf(instant, time.UTC); got != day("2026-03-28") {
		t.Fatalf("utc day = %v", got)
	}
}

func TestXP(t *testing.T) {
	cases := []struct {
		diff   string
		streak int
		xp     int
	}{
		{model.Easy, 0, 10}, {model.Medium, 6, 15}, {model.Hard, 1, 25},
		{model.Easy, 7, 15}, {model.Medium, 7, 23}, {model.Medium, 29, 23},
		{model.Medium, 30, 30}, {model.Hard, 30, 50},
	}
	for _, c := range cases {
		h := model.Habit{Kind: model.KindBuild, Difficulty: c.diff}
		if got := AwardXP(h, c.streak); got != c.xp {
			t.Errorf("%s/%d: got %d want %d", c.diff, c.streak, got, c.xp)
		}
	}
	if got := AwardXP(model.Habit{Kind: model.KindAvoid}, 0); got != 20 {
		t.Errorf("avoid: %d", got)
	}
}

func TestCoins(t *testing.T) {
	for xp, want := range map[int]int{10: 3, 15: 5, 20: 7, 23: 8, 25: 8, 50: 17} {
		if got := CoinsFor(xp); got != want {
			t.Errorf("CoinsFor(%d) = %d, want %d", xp, got, want)
		}
	}
}

func TestLevelCurveMatchesMockup(t *testing.T) {
	p := ProgressFor(8940, 0)
	if p.Level != 12 || p.Into != 1440 || p.Cost != 2000 || p.Missing != 560 {
		t.Fatalf("got %+v", p)
	}
	if Title(12) != "Gewohnheitsheld" || Title(13) != "Routine-Meister" || Title(1) != "Einsteiger" {
		t.Fatal("titles")
	}
}

func TestLevelCurveIsProgressive(t *testing.T) {
	for l := 1; l < 100; l++ {
		if LevelCost(l+1) <= LevelCost(l) {
			t.Fatalf("level %d not progressive", l)
		}
		if got := LevelFor(LevelXP(l)); got != l {
			t.Fatalf("LevelFor(LevelXP(%d)) = %d", l, got)
		}
		if got := LevelFor(LevelXP(l+1) - 1); got != l {
			t.Fatalf("just below level %d: %d", l+1, got)
		}
	}
}

func TestLevelsAreNeverLost(t *testing.T) {
	p := ProgressFor(LevelXP(5)-50, 5)
	if p.Level != 5 || p.Into != 0 {
		t.Fatalf("got %+v", p)
	}
}

func TestDailyStreak(t *testing.T) {
	h := daily("2026-10-01")
	markRange(h, day("2026-10-04"), day("2026-10-30"))
	mark(h, day("2026-10-01"))
	mark(h, day("2026-10-02"))
	s := h.StreakAt(day("2026-10-30"))
	if s.Current != 27 || s.Longest != 27 || len(s.Runs) != 2 {
		t.Fatalf("got %+v", s)
	}
}

func TestOpenTodayKeepsStreak(t *testing.T) {
	h := daily("2026-10-01", "2026-10-01", "2026-10-02")
	if s := h.StreakAt(day("2026-10-03")); s.Current != 2 {
		t.Fatalf("open today: %+v", s)
	}
	if s := h.StreakAt(day("2026-10-04")); s.Current != 0 || s.Longest != 2 {
		t.Fatalf("missed yesterday: %+v", s)
	}
}

func TestStreakAcrossMonthAndLeapDay(t *testing.T) {
	h := daily("2024-02-27")
	markRange(h, day("2024-02-27"), day("2024-03-02"))
	if s := h.StreakAt(day("2024-03-02")); s.Current != 5 {
		t.Fatalf("leap year: %+v", s)
	}
	h = daily("2025-02-27")
	markRange(h, day("2025-02-27"), day("2025-03-02"))
	if s := h.StreakAt(day("2025-03-02")); s.Current != 4 {
		t.Fatalf("common year: %+v", s)
	}
}

func TestStreakIgnoresLaterData(t *testing.T) {
	h := daily("2026-10-01", "2026-10-01", "2026-10-02", "2026-10-03")
	if s := h.StreakAt(day("2026-10-02")); s.Current != 2 {
		t.Fatalf("got %+v", s)
	}
}

func TestJokerBridgesWithoutCounting(t *testing.T) {
	h := daily("2026-10-01", "2026-10-01", "2026-10-02", "2026-10-04")
	h.Joker[day("2026-10-03")] = true
	if s := h.StreakAt(day("2026-10-04")); s.Current != 3 || len(s.Runs) != 1 {
		t.Fatalf("got %+v", s)
	}
}

func TestBackfillBeforeStartCounts(t *testing.T) {
	h := daily("2026-10-05", "2026-10-03", "2026-10-04", "2026-10-05")
	if s := h.StreakAt(day("2026-10-05")); s.Current != 3 {
		t.Fatalf("got %+v", s)
	}
}

func weekly(target int, start string) *HabitData {
	return NewHabitData(model.Habit{Kind: model.KindBuild, Frequency: model.FreqWeekly, WeeklyTarget: target}, day(start))
}

func TestWeeklyStreak(t *testing.T) {
	h := weekly(2, "2026-10-05") // a Monday
	for _, d := range []string{"2026-10-05", "2026-10-09", "2026-10-12", "2026-10-13"} {
		mark(h, day(d))
	}
	s := h.StreakAt(day("2026-10-14")) // third week open
	if !s.Weekly || s.Current != 2 || s.Days != 14 {
		t.Fatalf("got %+v", s)
	}
	if s := h.StreakAt(day("2026-10-26")); s.Current != 0 {
		t.Fatalf("missed week: %+v", s)
	}
}

func TestWeeklyStreakAcrossYearBoundary(t *testing.T) {
	h := weekly(1, "2025-12-22")
	mark(h, day("2025-12-24"))
	mark(h, day("2025-12-31"))
	mark(h, day("2026-01-05"))
	if s := h.StreakAt(day("2026-01-06")); s.Current != 3 {
		t.Fatalf("got %+v", s)
	}
}

func TestCustomDaysStreak(t *testing.T) {
	h := NewHabitData(model.Habit{Kind: model.KindBuild, Frequency: model.FreqCustom, DaysMask: 1<<4 | 1<<0}, day("2026-10-05"))
	mark(h, day("2026-10-05"))
	mark(h, day("2026-10-06")) // Tuesday is not planned and must not count
	if s := h.StreakAt(day("2026-10-12")); s.Current != 0 {
		t.Fatalf("got %+v", s)
	}
	mark(h, day("2026-10-09"))
	if s := h.StreakAt(day("2026-10-12")); s.Current != 1 {
		t.Fatalf("got %+v", s)
	}
	if !h.Scheduled(day("2026-10-09")) || h.Scheduled(day("2026-10-08")) {
		t.Fatal("scheduled")
	}
}

func TestDueWeekly(t *testing.T) {
	h := weekly(1, "2026-10-05")
	if !h.DueOn(day("2026-10-06")) {
		t.Fatal("should be due")
	}
	mark(h, day("2026-10-05"))
	if h.DueOn(day("2026-10-06")) {
		t.Fatal("target met, not due anymore")
	}
}

func avoid(start string, relapses ...string) *HabitData {
	h := NewHabitData(model.Habit{Kind: model.KindAvoid}, day(start))
	for _, r := range relapses {
		h.Relapses = append(h.Relapses, day(r))
	}
	return h
}

func TestAvoidStreak(t *testing.T) {
	h := avoid("2026-10-01")
	if s := h.StreakAt(day("2026-10-01")); s.Current != 0 {
		t.Fatalf("first day: %+v", s)
	}
	if s := h.StreakAt(day("2026-10-11")); s.Current != 10 {
		t.Fatalf("ten clean days: %+v", s)
	}
	h = avoid("2026-10-01", "2026-10-06")
	if s := h.StreakAt(day("2026-10-06")); s.Current != 0 || s.Longest != 5 {
		t.Fatalf("relapse today: %+v", s)
	}
	if s := h.StreakAt(day("2026-10-09")); s.Current != 2 || s.Longest != 5 {
		t.Fatalf("after relapse: %+v", s)
	}
}

func facts(w *World) *Facts {
	if w.Today == 0 {
		w.Today = day("2026-10-30")
	}
	if w.Installed == 0 {
		w.Installed = w.Today - 100
	}
	if w.Holiday == nil {
		w.Holiday = IsGermanHoliday
	}
	return BuildFacts(w)
}

func TestThereAre95BadgesIn16Categories(t *testing.T) {
	if len(Badges) != 95 {
		t.Fatalf("got %d badges", len(Badges))
	}
	if len(Categories) != 16 {
		t.Fatalf("got %d categories", len(Categories))
	}
	want := map[string]int{"serien": 10, "meilensteine": 11, "vermeiden": 5, "comeback": 4, "zeit": 6, "bereiche": 8,
		"gute-taten": 5, "familie": 5, "freunde": 5, "arbeit": 7, "digital": 5, "geld": 5, "umwelt": 5, "goenn-dir": 5, "level": 4, "geheim": 5}
	got := map[string]int{}
	seen := map[string]bool{}
	for _, b := range Badges {
		got[b.Category]++
		if seen[b.Key] {
			t.Errorf("duplicate key %s", b.Key)
		}
		seen[b.Key] = true
		if b.Metric.Rule == "" || b.Target <= 0 || b.Icon == "" {
			t.Errorf("incomplete badge %s", b.Key)
		}
		if _, ok := CategoryByKey(b.Category); !ok {
			t.Errorf("%s has unknown category", b.Key)
		}
	}
	for k, n := range want {
		if got[k] != n {
			t.Errorf("%s: got %d want %d", k, got[k], n)
		}
	}
}

// TestBadgeBoundaries checks every plain threshold rule one below and exactly
// at its target by setting the fact behind it directly.
func TestBadgeBoundaries(t *testing.T) {
	empty := func() *Facts {
		return &Facts{AreaCount: map[string]int{}, AreaStreak: map[string]int{}, AreaSpan: map[string]int{},
			TagCount: map[string]int{}, TagStreak: map[string]int{}, TagDays: map[string]int{}, AreaDays: map[string]int{}, AvoidAreaStreak: map[string]int{}}
	}
	set := map[string]func(*Facts, int){
		"funke": func(f *Facts, n int) { f.Habits = []habitFact{{kind: model.KindBuild, active: n}} }, "legende": func(f *Facts, n int) { f.Habits = []habitFact{{kind: model.KindBuild, active: n}} },
		"erster-schritt": func(f *Facts, n int) { f.Habits = []habitFact{{kind: model.KindBuild, done: n}} }, "tausend": func(f *Facts, n int) { f.Habits = []habitFact{{kind: model.KindBuild, done: n}} },
		"jongleur":       func(f *Facts, n int) { f.ActiveHabits = n },
		"perfekte-woche": func(f *Facts, n int) { f.PerfectRun = n }, "perfekter-monat": func(f *Facts, n int) { f.PerfectRun = n },
		"standhaft": func(f *Facts, n int) { f.Habits = []habitFact{{kind: model.KindAvoid, days: n}} }, "befreit": func(f *Facts, n int) { f.Habits = []habitFact{{kind: model.KindAvoid, days: n}} },
		"doppelschutz":   func(f *Facts, n int) { f.AvoidsAt30 = n },
		"comeback":       func(f *Facts, n int) { f.ComebackDays = n },
		"ohne-netz":      func(f *Facts, n int) { f.DaysNoJoker = n },
		"joker-sparer":   func(f *Facts, n int) { f.Jokers = n },
		"fruehaufsteher": func(f *Facts, n int) { f.EarlyCheckins = n }, "nachteule": func(f *Facts, n int) { f.LateCheckins = n },
		"wochenendheld": func(f *Facts, n int) { f.Weekends = n }, "feiertagsheld": func(f *Facts, n int) { f.HolidayCheckins = n },
		"ganzes-jahr": func(f *Facts, n int) { f.MonthsActive = n }, "jahreszeiten": func(f *Facts, n int) { f.Seasons = n },
		"buecherwurm": func(f *Facts, n int) { f.Habits = []habitFact{{area: "lernen", done: n}} }, "leseratte": func(f *Facts, n int) { f.Habits = []habitFact{{area: "lernen", done: n}} },
		"eiserner-wille": func(f *Facts, n int) { f.Habits = []habitFact{{tag: "sport", days: n}} }, "marathon": func(f *Facts, n int) { f.Habits = []habitFact{{tag: "sport", done: n}} },
		"ruhepol": func(f *Facts, n int) { f.Habits = []habitFact{{area: "achtsamkeit", done: n}} }, "zen-meister": func(f *Facts, n int) { f.Habits = []habitFact{{area: "achtsamkeit", done: n}} },
		"hydriert": func(f *Facts, n int) { f.Habits = []habitFact{{tag: "wasser", days: n}} }, "gesund-leben": func(f *Facts, n int) { f.HealthAt30 = n },
		"laecheln": func(f *Facts, n int) { f.Habits = []habitFact{{area: "gute-taten", done: n}} }, "helfende-hand": func(f *Facts, n int) { f.Habits = []habitFact{{area: "gute-taten", done: n}} },
		"weltverbesserer": func(f *Facts, n int) { f.Habits = []habitFact{{area: "gute-taten", done: n}} }, "ehrenamt": func(f *Facts, n int) { f.Habits = []habitFact{{area: "gute-taten", done: n}} },
		"herzensmensch": func(f *Facts, n int) { f.Habits = []habitFact{{area: "gute-taten", days: n}} },
		"angerufen":     func(f *Facts, n int) { f.Habits = []habitFact{{area: "familie", done: n}} }, "familientisch": func(f *Facts, n int) { f.Habits = []habitFact{{area: "familie", done: n}} },
		"daheim": func(f *Facts, n int) { f.Habits = []habitFact{{area: "familie", days: n}} }, "nicht-vergessen": func(f *Facts, n int) { f.Habits = []habitFact{{area: "familie", done: n}} },
		"familienmensch": func(f *Facts, n int) { f.AreaSpan["familie"] = n },
		"gemeldet":       func(f *Facts, n int) { f.Habits = []habitFact{{area: "freunde", done: n}} }, "treffpunkt": func(f *Facts, n int) { f.Habits = []habitFact{{area: "freunde", done: n}} },
		"verlaesslich": func(f *Facts, n int) { f.Habits = []habitFact{{area: "freunde", done: n}} }, "ueberraschung": func(f *Facts, n int) { f.Habits = []habitFact{{area: "freunde", done: n}} },
		"beste-freunde": func(f *Facts, n int) { f.AreaSpan["freunde"] = n },
		"ideengeber":    func(f *Facts, n int) { f.Habits = []habitFact{{area: "arbeit", done: n}} }, "initiative": func(f *Facts, n int) { f.Habits = []habitFact{{area: "arbeit", done: n}} },
		"vorangehen": func(f *Facts, n int) { f.Habits = []habitFact{{area: "arbeit", done: n}} }, "mentor": func(f *Facts, n int) { f.Habits = []habitFact{{area: "arbeit", done: n}} },
		"weiterbildung": func(f *Facts, n int) { f.Habits = []habitFact{{area: "arbeit", done: n}} }, "teamplayer": func(f *Facts, n int) { f.Habits = []habitFact{{area: "arbeit", done: n}} },
		"fokus":               func(f *Facts, n int) { f.Habits = []habitFact{{area: "arbeit", days: n}} },
		"offline-stunde":      func(f *Facts, n int) { f.AreaDays["digital"] = n },
		"handyfreier-morgen":  func(f *Facts, n int) { f.Habits = []habitFact{{tag: "handy", days: n}} },
		"scroll-stopp":        func(f *Facts, n int) { f.Habits = []habitFact{{tag: "doomscroll", days: n}} },
		"detox-tag":           func(f *Facts, n int) { f.DigitalClean = n },
		"bildschirm-zen":      func(f *Facts, n int) { f.AvoidAreaStreak["digital"] = n },
		"widerstanden":        func(f *Facts, n int) { f.Habits = []habitFact{{tag: "impulskauf", days: n}} },
		"budget-profi":        func(f *Facts, n int) { f.Habits = []habitFact{{tag: "budget", days: n}} },
		"selbstgemacht":       func(f *Facts, n int) { f.Habits = []habitFact{{tag: "lieferdienst", days: n}} },
		"sparschwein":         func(f *Facts, n int) { f.AvoidAreaStreak["geld"] = n },
		"finanzfreiheit":      func(f *Facts, n int) { f.AvoidAreaStreak["geld"] = n },
		"mehrweg":             func(f *Facts, n int) { f.TagDays["einweg"] = n },
		"velo-statt-auto":     func(f *Facts, n int) { f.TagDays["velo"] = n },
		"veggie-woche":        func(f *Facts, n int) { f.Habits = []habitFact{{tag: "fleisch", days: n}} },
		"nichts-verschwendet": func(f *Facts, n int) { f.Habits = []habitFact{{tag: "verschwendung", days: n}} },
		"gruener-fussabdruck": func(f *Facts, n int) { f.Habits = []habitFact{{area: "umwelt", days: n}} },
		"sparfuchs":           func(f *Facts, n int) { f.Coins = n },
		"erster-taler":        func(f *Facts, n int) { f.Redeemed = n }, "erste-belohnung": func(f *Facts, n int) { f.Redeemed = n }, "geniesser": func(f *Facts, n int) { f.Redeemed = n },
		"grosser-wunsch": func(f *Facts, n int) { f.MaxRedeemPrice = n },
		"schatzmeister":  func(f *Facts, n int) { f.CoinsEarned = n },
		"aufsteiger":     func(f *Facts, n int) { f.Level = n }, "meister": func(f *Facts, n int) { f.Level = n },
		"ein-jahr-dabei": func(f *Facts, n int) { f.DaysInstalled = n },
	}
	for key, apply := range set {
		b, ok := BadgeByKey(key)
		if !ok {
			t.Errorf("unknown badge %s", key)
			continue
		}
		f := empty()
		apply(f, b.Target-1)
		if b.Reached(f) {
			t.Errorf("%s reached below target", key)
		}
		apply(f, b.Target)
		if !b.Reached(f) {
			t.Errorf("%s not reached at target", key)
		}
	}
}

func TestStreakBadgesCountBuildHabitsOnly(t *testing.T) {
	h := avoid("2026-09-01")
	f := facts(&World{Habits: []*HabitData{h}})
	if f.BestStreak != 0 || f.AvoidBest != 59 {
		t.Fatalf("got best=%d avoid=%d", f.BestStreak, f.AvoidBest)
	}
}

func TestComebackAndPhoenix(t *testing.T) {
	h := avoid("2026-09-01", "2026-09-11") // 10 clean days, then relapse
	f := facts(&World{Today: day("2026-09-25"), Habits: []*HabitData{h}})
	if f.ComebackDays != 13 || !f.Phoenix {
		t.Fatalf("comeback=%d phoenix=%v", f.ComebackDays, f.Phoenix)
	}
	f = facts(&World{Today: day("2026-09-16"), Habits: []*HabitData{h}})
	if f.Phoenix {
		t.Fatal("no record yet")
	}
}

func TestPerfectRunAndWeekends(t *testing.T) {
	h := daily("2026-10-05")
	markRange(h, day("2026-10-05"), day("2026-10-18")) // two full weeks, Mon to Sun
	other := daily("2026-10-05")
	markRange(other, day("2026-10-05"), day("2026-10-18"))
	w := &World{Today: day("2026-10-18"), Habits: []*HabitData{h, other}}
	f := facts(w)
	if f.PerfectRun != 14 || f.Weekends != 2 {
		t.Fatalf("perfect=%d weekends=%d", f.PerfectRun, f.Weekends)
	}
	// A relapse of any avoid habit spoils the day.
	a := avoid("2026-10-01", "2026-10-11")
	w.Habits = append(w.Habits, a)
	f = facts(w)
	if f.PerfectRun != 7 || f.Weekends != 1 {
		t.Fatalf("with relapse perfect=%d weekends=%d", f.PerfectRun, f.Weekends)
	}
}

func TestPerfectRunRequiresWeeklyTargets(t *testing.T) {
	h := daily("2026-10-05")
	markRange(h, day("2026-10-05"), day("2026-10-11"))
	f := facts(&World{Today: day("2026-10-11"), Habits: []*HabitData{h, weekly(3, "2026-10-05")}})
	if f.PerfectRun != 0 {
		t.Fatalf("got %d", f.PerfectRun)
	}
}

func at(d string, hour, minute int) time.Time {
	x := day(d)
	return time.Date(x.Year(), x.Month(), x.Dom(), hour, minute, 0, 0, time.UTC)
}

func TestTimeOfDayFacts(t *testing.T) {
	h := daily("2026-03-01")
	h.Done[day("2026-03-14")] = at("2026-03-14", 6, 59)
	h.Done[day("2026-03-15")] = at("2026-03-15", 7, 0)
	h.Done[day("2026-03-16")] = at("2026-03-16", 22, 0)
	h.Done[day("2026-03-17")] = at("2026-03-17", 21, 59)
	h.Done[day("2026-03-18")] = at("2026-03-18", 0, 5)
	// A backfill entered the next morning says nothing about the time of day.
	h.Done[day("2026-03-19")] = at("2026-03-20", 5, 0)
	f := facts(&World{Today: day("2026-03-20"), Habits: []*HabitData{h}})
	if f.EarlyCheckins != 2 || f.LateCheckins != 1 {
		t.Fatalf("early=%d late=%d", f.EarlyCheckins, f.LateCheckins)
	}
	if !f.Midnight || !f.PiDay {
		t.Fatalf("midnight=%v pi=%v", f.Midnight, f.PiDay)
	}
	h2 := daily("2026-03-01")
	h2.Done[day("2026-03-13")] = at("2026-03-13", 0, 6)
	if f := facts(&World{Today: day("2026-03-20"), Habits: []*HabitData{h2}}); f.Midnight || f.PiDay {
		t.Fatalf("got %+v", f)
	}
}

func TestBlitzStart(t *testing.T) {
	h := daily("2026-10-30")
	h.Done[day("2026-10-30")] = at("2026-10-30", 7, 59)
	if f := facts(&World{Habits: []*HabitData{h}}); !f.Blitz {
		t.Fatal("expected blitz")
	}
	other := daily("2026-10-30")
	if f := facts(&World{Habits: []*HabitData{h, other}}); f.Blitz {
		t.Fatal("second habit open")
	}
}

func TestSchnapszahl(t *testing.T) {
	h := daily("2026-01-01")
	end := day("2026-01-01") + 110
	markRange(h, day("2026-01-01"), end)
	if f := facts(&World{Today: end, Installed: end - 200, Habits: []*HabitData{h}}); !f.Palindrome {
		t.Fatal("expected at 111")
	}
	if f := facts(&World{Today: end - 1, Installed: end - 200, Habits: []*HabitData{h}}); f.Palindrome {
		t.Fatal("not at 110")
	}
}

func TestMonthsActiveConsecutive(t *testing.T) {
	h := daily("2025-11-01")
	for _, d := range []string{"2025-11-03", "2025-12-03", "2026-01-03", "2026-03-03"} {
		mark(h, day(d))
	}
	if f := facts(&World{Today: day("2026-03-30"), Habits: []*HabitData{h}}); f.MonthsActive != 3 {
		t.Fatalf("got %d", f.MonthsActive)
	}
}

func TestHolidays(t *testing.T) {
	for _, d := range []string{"2026-01-01", "2026-04-03", "2026-04-06", "2026-05-14", "2026-05-25", "2026-10-03", "2026-12-26"} {
		if !IsGermanHoliday(day(d)) {
			t.Errorf("%s should be a holiday", d)
		}
	}
	for _, d := range []string{"2026-04-05", "2026-10-30"} {
		if IsGermanHoliday(day(d)) {
			t.Errorf("%s is no holiday", d)
		}
	}
}

func TestSeasons(t *testing.T) {
	h := daily("2025-12-01")
	markRange(h, day("2025-12-01"), day("2026-10-30"))
	if f := facts(&World{Habits: []*HabitData{h}}); f.Seasons != 4 {
		t.Fatalf("got %d", f.Seasons)
	}
	h = daily("2026-06-01")
	markRange(h, day("2026-06-01"), day("2026-07-30"))
	if f := facts(&World{Today: day("2026-08-30"), Habits: []*HabitData{h}}); f.Seasons != 0 {
		t.Fatalf("below 80 percent: %d", f.Seasons)
	}
}

func TestNewlyReachedSkipsUnlocked(t *testing.T) {
	h := daily("2026-10-28")
	markRange(h, day("2026-10-28"), day("2026-10-30"))
	f := facts(&World{Installed: day("2026-10-28"), Habits: []*HabitData{h}})
	got := NewlyReached(f, map[string]bool{"funke": true})
	for _, b := range got {
		if b.Key == "funke" {
			t.Fatal("funke already unlocked")
		}
	}
	if len(got) != 1 || got[0].Key != "erster-schritt" {
		t.Fatalf("got %+v", got)
	}
}

func TestWeeklyStreakCountsActiveDaysForSerienBadges(t *testing.T) {
	h := weekly(3, "2026-09-28")
	for w := day("2026-09-28"); w < day("2026-10-26"); w += 7 {
		mark(h, w)
		mark(h, w+2)
		mark(h, w+4)
	}
	s := h.StreakAt(day("2026-10-26"))
	if s.Days != 28 || s.Active != 12 {
		t.Fatalf("got %+v", s)
	}
	if f := facts(&World{Today: day("2026-10-26"), Habits: []*HabitData{h}}); f.BestStreak != 12 {
		t.Fatalf("best %d", f.BestStreak)
	}
}

func TestPerfectNeedsTwoHabits(t *testing.T) {
	h := daily("2026-10-05")
	markRange(h, day("2026-10-05"), day("2026-10-18"))
	if f := facts(&World{Today: day("2026-10-18"), Habits: []*HabitData{h}}); f.PerfectRun != 0 {
		t.Fatalf("got %d", f.PerfectRun)
	}
}

func TestMonthlyStreak(t *testing.T) {
	h := NewHabitData(model.Habit{Kind: model.KindBuild, Frequency: model.FreqMonthly, WeeklyTarget: 1}, day("2026-01-10"))
	mark(h, day("2026-01-31"))
	mark(h, day("2026-02-28"))
	mark(h, day("2026-03-01"))
	s := h.StreakAt(day("2026-03-15"))
	if !s.Monthly || s.Current != 3 || s.Days != 90 || s.Active != 3 {
		t.Fatalf("got %+v", s)
	}
	if h.DueOn(day("2026-03-15")) {
		t.Fatal("month target met")
	}
	if s := h.StreakAt(day("2026-05-01")); s.Current != 0 {
		t.Fatalf("missed april: %+v", s)
	}
	if !h.DueOn(day("2026-05-01")) {
		t.Fatal("new month is due")
	}
}

func TestLeapFebruaryIsOneMonth(t *testing.T) {
	if nextMonth(day("2024-02-29")) != day("2024-03-01") || nextMonth(day("2024-01-31")) != day("2024-02-01") || nextMonth(day("2026-12-05")) != day("2027-01-01") {
		t.Fatal("nextMonth")
	}
}

func TestCleanDaysCountForTagBadges(t *testing.T) {
	h := avoid("2026-10-01", "2026-10-06")
	h.Tag, h.AreaKey = "einweg", "umwelt"
	f := facts(&World{Today: day("2026-10-11"), Habits: []*HabitData{h}})
	if f.TagDays["einweg"] != 9 || f.AreaDays["umwelt"] != 9 || f.AvoidAreaStreak["umwelt"] != 4 {
		t.Fatalf("days %d area %d streak %d", f.TagDays["einweg"], f.AreaDays["umwelt"], f.AvoidAreaStreak["umwelt"])
	}
}

func TestDetoxNeedsThreeCleanDigitalHabits(t *testing.T) {
	var habits []*HabitData
	for range 3 {
		h := avoid("2026-10-28")
		h.AreaKey = "digital"
		habits = append(habits, h)
	}
	if f := facts(&World{Habits: habits}); f.DigitalClean != 3 {
		t.Fatalf("got %d", f.DigitalClean)
	}
	habits[0].Relapses = []model.Day{day("2026-10-29")}
	if f := facts(&World{Today: day("2026-10-29"), Habits: habits}); f.DigitalClean != 2 {
		t.Fatalf("got %d", f.DigitalClean)
	}
}

func TestEveryBadgeHasARule(t *testing.T) {
	for _, b := range Badges {
		if b.Rule() == "" {
			t.Errorf("%s has no rule", b.Key)
		}
	}
	if len(badgeRules) != len(Badges) {
		t.Errorf("%d rules for %d badges", len(badgeRules), len(Badges))
	}
}

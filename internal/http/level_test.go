package server

import (
	"fmt"
	"html"
	"strings"
	"testing"
	"time"

	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

func dailyHabit(start model.Day) *game.HabitData {
	return game.NewHabitData(model.Habit{ID: 1, Name: "Lesen", Kind: model.KindBuild, Frequency: model.FreqDaily}, start)
}

func streakBadges() []game.Badge { return dayGoals(game.DefaultRules()) }

func TestDayGoalsFollowBadges(t *testing.T) {
	r := game.DefaultRules()
	if got := dayGoals(r); got[0].Name != "Funke" || got[2].Name != "Zwei Wochen" {
		t.Errorf("goals = %v", got[:3])
	}
	r.BadgesOff = true
	if got := dayGoals(r); got[0].Name != "" || got[0].Target != 3 {
		t.Errorf("badges off must keep plain spans, got %+v", got[0])
	}
}

func TestStreakRowGroups(t *testing.T) {
	today := model.NewDay(2026, time.November, 4)
	h := dailyHabit(today - 20)
	for d := today - 12; d < today; d++ {
		h.Done[d] = time.Time{}
	}
	row, group := newStreakRow(h, today, 9, streakBadges())
	if group != streakRisk || row.DetailLine() != "heute noch offen · 9 Std. übrig" {
		t.Errorf("open = %s %q", group, row.DetailLine())
	}
	h.Done[today] = time.Time{}
	row, group = newStreakRow(h, today, 9, streakBadges())
	if group != streakSafe || !row.Record || row.DetailLine() != "noch 1 Tag bis „Zwei Wochen“" {
		t.Errorf("safe = %s record=%v %q", group, row.Record, row.DetailLine())
	}
	if row.Ring != 92 {
		t.Errorf("ring = %d", row.Ring)
	}
	delete(h.Done, today)
	delete(h.Done, today-1)
	row, group = newStreakRow(h, today, 9, streakBadges())
	if group != streakNew || row.DetailLine() != "Rekord 11 T – heute wieder anfangen" {
		t.Errorf("broken = %s %q", group, row.DetailLine())
	}
}

func TestStreakRowMeasured(t *testing.T) {
	today := model.NewDay(2026, time.November, 4)
	h := dailyHabit(today - 5)
	h.Measure, h.Target, h.Step, h.Unit = model.MeasureAmount, 3, 0.5, "l"
	h.Done[today-1] = time.Time{}
	h.Entries[today] = 1.5
	row, group := newStreakRow(h, today, 2, streakBadges())
	if group != streakRisk || !strings.Contains(row.DetailLine(), "noch 1,5 L bis zum Ziel") {
		t.Errorf("%s %q", group, row.DetailLine())
	}
}

func TestStreakRowWeekly(t *testing.T) {
	sunday := model.NewDay(2026, time.November, 1)
	h := game.NewHabitData(model.Habit{ID: 2, Kind: model.KindBuild, Frequency: model.FreqWeekly, WeeklyTarget: 2}, sunday-30)
	for _, d := range []model.Day{sunday - 7 - 1, sunday - 7 - 2} { // the week before
		h.Done[d] = time.Time{}
	}
	h.Done[sunday-1] = time.Time{}
	row, group := newStreakRow(h, sunday-1, 9, streakBadges())
	if group != streakSafe || row.Unit != "Wo" || !strings.Contains(row.DetailLine(), "1 von 2 diese Woche") {
		t.Errorf("saturday: %s %q", group, row.DetailLine())
	}
	row, group = newStreakRow(h, sunday, 9, streakBadges())
	if group != streakRisk {
		t.Errorf("one day left for one check-in: %s %q", group, row.DetailLine())
	}
	h.Done[sunday] = time.Time{}
	row, group = newStreakRow(h, sunday, 9, streakBadges())
	if group != streakSafe || !strings.Contains(row.DetailLine(), "diese Woche schon erreicht") {
		t.Errorf("met: %s %q", group, row.DetailLine())
	}
}

func TestStreakRowAvoid(t *testing.T) {
	today := model.NewDay(2026, time.November, 4)
	h := game.NewHabitData(model.Habit{ID: 3, Kind: model.KindAvoid}, today-5)
	if _, group := newStreakRow(h, today, 9, streakBadges()); group != streakSafe {
		t.Errorf("clean days are safe, got %s", group)
	}
	h.Relapses = []model.Day{today}
	row, group := newStreakRow(h, today, 9, streakBadges())
	if group != streakNew || !strings.Contains(row.DetailLine(), "morgen wieder anfangen") {
		t.Errorf("relapse: %s %q", group, row.DetailLine())
	}
}

func TestStatRange(t *testing.T) {
	sunday := model.NewDay(2026, time.November, 1)
	r := newStatRange(rangeWeek, sunday, sunday-90)
	if r.From != sunday-6 || r.Before[0] != sunday-13 || r.Before[1] != sunday-7 {
		t.Errorf("week = %+v", r)
	}
	r = newStatRange("Unsinn", sunday, sunday-90)
	if r.Key != rangeWeek {
		t.Errorf("unknown range falls back to the week, got %q", r.Key)
	}
	r = newStatRange(rangeMonth, model.NewDay(2026, time.March, 30), sunday-90)
	if want := model.NewDay(2026, time.February, 28); r.Before[1] != want {
		t.Errorf("the month before ends earlier: %s", r.Before[1])
	}
}

func TestLevelMilestones(t *testing.T) {
	r := game.DefaultRules()
	steps := pathSteps(r, game.Progress{Level: 7, Into: 10, Cost: 100, Missing: 90, Percent: 10}, 30)
	var levels []string
	for _, s := range steps {
		levels = append(levels, fmt.Sprintf("%d:%s", s.Level, s.State))
	}
	got := strings.Join(levels[:6], " ")
	if got != "1:done 3:done 5:done 6:done 7:now 8:next" {
		t.Errorf("steps = %s", got)
	}
	if steps[4].ETA != "noch ca. 3 Tage" {
		t.Errorf("eta = %q", steps[4].ETA)
	}
}

func TestLevelPages(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.habit("Lesen", model.KindBuild)
	c.post(fmt.Sprintf("/habits/%d/check", id), true)

	_, body := c.get("/")
	for _, want := range []string{"Als Nächstes:", "/level?tab=stats", "/level?tab=path", "/level?tab=history"} {
		if !strings.Contains(body, want) {
			t.Errorf("level card misses %q", want)
		}
	}
	resp, body := c.get("/level")
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	for _, want := range []string{"Erfüllungsquote", "längste Serien", "XP pro Tag", "XP nach Bereich", "Deine Serien", "Lesen"} {
		if !strings.Contains(body, want) {
			t.Errorf("stats miss %q", want)
		}
	}
	for _, rng := range []string{rangeToday, rangeMonth, rangeAll} {
		if resp, _ := c.get("/level?tab=stats&range=" + rng); resp.StatusCode != 200 {
			t.Errorf("range %s: %d", rng, resp.StatusCode)
		}
	}
	_, body = c.get("/level?tab=path")
	if !strings.Contains(body, "XP gesamt") || !strings.Contains(body, "way-step now") {
		t.Errorf("path page is incomplete")
	}
	body = c.page("/level?tab=history")
	if !strings.Contains(body, "class=\"event") || !strings.Contains(body, "Ziel erreicht") || !strings.Contains(body, "+15 XP") {
		t.Errorf("history misses the check-in")
	}
	if resp, _ := c.get("/level?tab=history&day=1999-01-01"); resp.StatusCode != 200 {
		t.Errorf("an old day falls back to today, got %d", resp.StatusCode)
	}
}

func TestHistoryCounter(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.meterHabit("Wasser", model.MeasureAmount, "1", "0.5", "l")
	path := fmt.Sprintf("/habits/%d/add", id)
	c.post(path, true)
	body := c.page("/level?tab=history")
	if !strings.Contains(body, "+0,5 L · 0,5 / 1 L") {
		t.Errorf("first click is missing")
	}
	c.post(path, true)
	body = c.page("/level?tab=history")
	if !strings.Contains(body, "Ziel erreicht · 1 L") {
		t.Errorf("goal line is missing")
	}
	if strings.Count(body, "+0,5 L") != 1 {
		t.Errorf("the click that reaches the goal stays out: %d", strings.Count(body, "+0,5 L"))
	}
}

func TestEtaDays(t *testing.T) {
	if got := etaDays(65, 33); got != 2 {
		t.Errorf("eta = %d", got)
	}
	if got := etaDays(65, 0); got != 0 {
		t.Errorf("no pace = %d", got)
	}
}

// page reads a page as the browser shows its text.
func (c *client) page(path string) string {
	c.t.Helper()
	_, body := c.get(path)
	return html.UnescapeString(body)
}

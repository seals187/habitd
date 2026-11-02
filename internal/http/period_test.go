package server

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/seals187/habitd/internal/model"
)

func (c *client) today(t *testing.T) model.Day {
	t.Helper()
	w, err := c.svc.World(c.q())
	if err != nil {
		t.Fatal(err)
	}
	return w.Today
}

func TestWeekPeriods(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.habit("Lesen", "build")
	today := c.today(t)
	week := today.WeekStart()
	path := func(w model.Day) string { return fmt.Sprintf("/habits/%d?view=week&w=%s", id, w) }

	running := c.page(fmt.Sprintf("/habits/%d?view=week", id))
	if !strings.Contains(running, fmt.Sprintf(`w=%s"`, week-7)) || strings.Contains(running, `data-swipe="next"`) {
		t.Fatal("the running week goes back but not on")
	}
	before := c.page(path(week - 7))
	if !strings.Contains(before, fmt.Sprintf(`w=%s"`, week-14)) || !strings.Contains(before, fmt.Sprintf(`w=%s"`, week)) {
		t.Fatal("a past week goes both ways")
	}
	if !strings.Contains(before, rangeLabel(week-7, week-1)) {
		t.Fatal("a past week names all of its days")
	}
	if strings.Contains(before, "läuft") {
		t.Fatal("a past week has no running day")
	}
	if got := c.page(path(today + 21)); strings.Contains(got, `data-swipe="next"`) {
		t.Fatal("a week in the future falls back to the running one")
	}
	if resp, _ := c.get(fmt.Sprintf("/habits/%d?view=week&w=garbage", id)); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad week: %s", resp.Status)
	}

	check := fmt.Sprintf("/habits/%d/check?date=%s&from=detail&view=week&w=%s", id, today, week-7)
	c.post(check, true)
	for { // the first tick earns a badge, and its celebration would take over the answer
		celebration, _, _, err := c.q().NextCelebration()
		if err != nil {
			break
		}
		if err := c.q().DeleteCelebration(celebration); err != nil {
			t.Fatal(err)
		}
	}
	_, html := c.post(check, true)
	if !strings.Contains(html, rangeLabel(week-7, week-1)) {
		t.Fatal("a tick while looking at a past week stays in that week")
	}
}

func TestMonthStopsAtTheRunningOne(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.habit("Lesen", "build")
	if html := c.page(fmt.Sprintf("/habits/%d?view=month", id)); strings.Contains(html, `data-swipe="next"`) {
		t.Fatal("the running month does not go on")
	}
	today := c.today(t)
	first := model.NewDay(today.Year(), today.Month()-1, 1)
	if html := c.page(fmt.Sprintf("/habits/%d?view=month&m=%s", id, first.String()[:7])); !strings.Contains(html, `data-swipe="next"`) {
		t.Fatal("a past month goes on")
	}
}

func TestYearSpans(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	plain := c.habit("Lesen", "build")
	amount := c.meterHabit("Wasser", model.MeasureAmount, "3", "1", "L")
	today := c.today(t)
	bars := func(id int64, query string) (plain, amounts int) {
		html := c.page(fmt.Sprintf("/habits/%d?view=year%s", id, query))
		return strings.Count(html, `class="month-track"`), strings.Count(html, `class="amount-track"`)
	}

	if got, _ := bars(plain, ""); got != spanShort {
		t.Fatalf("the year starts with %d months, got %d", spanShort, got)
	}
	if _, got := bars(amount, ""); got != spanShort {
		t.Fatalf("a new habit with a unit still shows %d months, got %d", spanShort, got)
	}
	if got, _ := bars(plain, "&span=12"); got != spanLong {
		t.Fatalf("12 months: got %d", got)
	}
	if _, got := bars(amount, "&span=12"); got != spanLong {
		t.Fatalf("12 months with a unit: got %d", got)
	}
	if got, _ := bars(plain, "&span=99"); got != spanShort {
		t.Fatalf("an unknown length falls back: got %d", got)
	}

	path := fmt.Sprintf("/habits/%d?view=year", plain)
	_, html := c.do("GET", path, nil, map[string]string{"Cookie": "year_span=12"})
	if strings.Count(html, `class="month-track"`) != spanLong {
		t.Fatal("the device keeps its length")
	}

	running := c.page(path)
	end := model.NewDay(today.Year(), today.Month(), 1)
	back := model.NewDay(end.Year(), end.Month()-6, 1).String()[:7]
	if !strings.Contains(running, "y="+back+`"`) || strings.Contains(running, `data-swipe="next"`) {
		t.Fatal("the running window goes back by its length and not on")
	}
	past := c.page(path + "&y=" + back)
	if !strings.Contains(past, "y="+end.String()[:7]+`"`) {
		t.Fatal("a past window goes on")
	}
	if strings.Contains(past, "Letzte 6 Monate") {
		t.Fatal("a past window names its months")
	}
	if got := c.page(path + "&y=2999-01"); strings.Contains(got, `data-swipe="next"`) {
		t.Fatal("a window in the future falls back to the running one")
	}
	if resp, _ := c.get(path + "&y=garbage"); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad window: %s", resp.Status)
	}
}

func TestMonthsLabel(t *testing.T) {
	cases := map[string][2]model.Day{
		"Mai–Okt 2026":  {model.NewDay(2026, 5, 1), model.NewDay(2026, 10, 31)},
		"Dez 25–Mai 26": {model.NewDay(2025, 12, 1), model.NewDay(2026, 5, 31)},
	}
	for want, days := range cases {
		if got := monthsLabel(days[0], days[1]); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}

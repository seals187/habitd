package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/seals187/habitd/internal/model"
)

// later moves the clock on, so a new habit is older than the usual days of backfill.
func (c *client) later(days int) {
	now := c.svc.Now
	c.svc.Now = func() time.Time { return now().AddDate(0, 0, days) }
}

func (c *client) editDay(id int64, d model.Day, form url.Values) (*http.Response, string) {
	c.t.Helper()
	form.Set("csrf", c.csrf)
	path := fmt.Sprintf("/habits/%d/day?date=%s&from=detail&view=month&m=%s", id, d, d.String()[:7])
	return c.do("POST", path, form, map[string]string{"X-CSRF-Token": c.csrf, "HX-Request": "true"})
}

func (c *client) dropCelebrations() {
	for {
		id, _, _, err := c.q().NextCelebration()
		if err != nil {
			return
		}
		if err := c.q().DeleteCelebration(id); err != nil {
			c.t.Fatal(err)
		}
	}
}

func TestEditDayOfAnAmount(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.meterHabit("Wasser", model.MeasureAmount, "3", "0.5", "L")
	c.later(25)
	today := c.today(t)
	day := today - 20

	month := c.page(fmt.Sprintf("/habits/%d?view=month&m=%s", id, day.String()[:7]))
	if !strings.Contains(month, fmt.Sprintf(`/day?date=%s&`, day)) {
		t.Fatal("a day long ago opens its sheet")
	}
	_, sheet := c.get(fmt.Sprintf("/habits/%d/day?date=%s", id, day))
	for _, want := range []string{`value="3"`, "Liter", "Tagesziel 3 L", `name="time"`} {
		if !strings.Contains(sheet, want) {
			t.Fatalf("sheet misses %q: %s", want, sheet)
		}
	}

	before := c.xp()
	if resp, _ := c.editDay(id, day, url.Values{"amount": {"2,5"}, "time": {"07:30"}}); resp.StatusCode >= 400 {
		t.Fatalf("below the goal: %s", resp.Status)
	}
	if got := c.xp(); got != before {
		t.Fatalf("a day below the goal earns nothing: %d", got-before)
	}
	c.editDay(id, day, url.Values{"amount": {"3"}, "time": {"12:00"}})
	if c.xp() <= before {
		t.Fatal("reaching the goal pays")
	}
	c.dropCelebrations()
	_, sheet = c.get(fmt.Sprintf("/habits/%d/day?date=%s", id, day))
	if !strings.Contains(sheet, `value="12:00"`) || !strings.Contains(sheet, `name="amount" min="0" step="any" inputmode="decimal" value="3"`) {
		t.Fatalf("the sheet shows what was saved, noon included: %s", sheet)
	}

	c.editDay(id, day, url.Values{"amount": {"0"}})
	if got := c.xp(); got != before {
		t.Fatalf("clearing the day takes the reward back: %d", got-before)
	}
}

func TestEditDayOfATick(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.habit("Lesen", "build")
	c.later(15)
	day := c.today(t) - 10

	c.editDay(id, day, url.Values{"done": {"on"}, "time": {"06:45"}})
	c.dropCelebrations()
	_, sheet := c.get(fmt.Sprintf("/habits/%d/day?date=%s", id, day))
	if !strings.Contains(sheet, `value="06:45"`) {
		t.Fatalf("the time stays: %s", sheet)
	}
	c.editDay(id, day, url.Values{"done": {"on"}, "time": {"08:15"}})
	_, sheet = c.get(fmt.Sprintf("/habits/%d/day?date=%s", id, day))
	if !strings.Contains(sheet, `value="08:15"`) {
		t.Fatal("the time of a reached day can change")
	}
	c.editDay(id, day, url.Values{})
	_, sheet = c.get(fmt.Sprintf("/habits/%d/day?date=%s", id, day))
	if strings.Contains(sheet, `value="08:15"`) {
		t.Fatal("switching a day off removes it")
	}
}

func TestADayNeverChangesWithOneClick(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.habit("Lesen", "build")
	avoid := c.habit("Rauchen", "avoid")
	for _, view := range []string{"week", "month"} {
		html := c.page(fmt.Sprintf("/habits/%d?view=%s", id, view))
		if strings.Contains(html, "/check?date=") {
			t.Fatalf("%s: a day of a habit may only open its sheet", view)
		}
		if !strings.Contains(html, "/day?date=") {
			t.Fatalf("%s: a day opens its sheet", view)
		}
		if html := c.page(fmt.Sprintf("/habits/%d?view=%s", avoid, view)); !strings.Contains(html, "Rückfall eintragen?") {
			t.Fatalf("%s: a relapse is confirmed first", view)
		}
	}
	today := c.today(t)
	week := today.WeekStart() - 7
	_, sheet := c.get(fmt.Sprintf("/habits/%d/day?date=%s&view=week&w=%s", id, today, week))
	if !strings.Contains(sheet, "view=week") || !strings.Contains(sheet, "w="+week.String()) {
		t.Fatalf("saving returns to the view it came from: %s", sheet)
	}
}

func TestDayBeforeTheStartCounts(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.meterHabit("Wasser", model.MeasureAmount, "3", "1", "L")
	today := c.today(t)
	day := today - 1
	c.editDay(id, day, url.Values{"amount": {"3"}, "time": {"20:03"}})
	c.dropCelebrations()

	week := c.page(fmt.Sprintf("/habits/%d?view=week&w=%s", id, day.WeekStart()))
	if !strings.Contains(week, "✓20:03") {
		t.Fatalf("the week shows the day: %s", week)
	}
	month := c.page(fmt.Sprintf("/habits/%d?view=month&m=%s", id, day.String()[:7]))
	if !strings.Contains(month, "seit "+sinceLabel(day, today)) {
		t.Fatal("the habit counts from that day on")
	}
	if year := c.page(fmt.Sprintf("/habits/%d?view=year", id)); !strings.Contains(year, "<b>3</b> <span>Liter</span>") {
		t.Fatal("the year adds the day up")
	}
}

func TestIconsAreOnThePage(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	_, html := c.get("/profile")
	if !strings.Contains(html, `<symbol id="house"`) || !strings.Contains(html, `<use href="#house"`) || strings.Contains(html, "sprite.svg?v=") {
		t.Fatal("icons must not wait for a file of their own, they blink on every load")
	}
}

func TestAuthPagesShowTheLogo(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	_, html := c.fresh().get("/login")
	if !strings.Contains(html, "/static/icons/icon-192.png\" width") {
		t.Fatalf("login: %s", html)
	}
}

func TestEditDayLimits(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.habit("Lesen", "build")
	avoid := c.habit("Rauchen", "avoid")
	today := c.today(t)

	if resp, _ := c.editDay(id, today+1, url.Values{"done": {"on"}}); resp.StatusCode < 400 {
		t.Fatal("a day in the future stays closed")
	}
	if resp, _ := c.editDay(id, today-30, url.Values{"done": {"on"}}); resp.StatusCode < 400 {
		t.Fatal("a day long before the start stays closed")
	}
	if resp, _ := c.editDay(avoid, today-1, url.Values{"done": {"on"}}); resp.StatusCode < 400 {
		t.Fatal("an avoid habit keeps its own way")
	}
	if resp, _ := c.editDay(id, today-1, url.Values{"done": {"on"}, "time": {"25:00"}}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad time: %s", resp.Status)
	}
	if resp, _ := c.editDay(c.meterHabit("W", model.MeasureAmount, "3", "1", "L"), today-1, url.Values{"amount": {"-1"}}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("negative amount: %s", resp.Status)
	}
	later := time.Now().In(c.svc.Loc).Add(2 * time.Minute)
	if model.DayOf(later, c.svc.Loc) == today {
		resp, body := c.editDay(id, today, url.Values{"done": {"on"}, "time": {later.Format("15:04")}})
		if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "Zukunft") {
			t.Fatalf("a time still to come: %s %s", resp.Status, body)
		}
	}
}

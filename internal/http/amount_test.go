package server

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

func TestAmountFormats(t *testing.T) {
	cases := []struct {
		unit         string
		v            float64
		label, bare  string
		number, word string
	}{
		{"L", 2.53, "2,5 L", "2,5", "2,5", "Liter"},
		{"Liter", 3, "3 L", "3,0", "3", "Liter"},
		{"l", 389.4, "389 L", "389", "389", "Liter"},
		{"Min.", 45, "45 Min.", "45′", "45", "Minuten"},
		{"Min.", 60, "1 Std.", "1h", "1", "Stunde"},
		{"Minuten", 150, "2,5 Std.", "2,5h", "2,5", "Stunden"},
		{"Schritte", 800, "800", "800", "800", "Schritte"},
		{"Schritte", 6400, "6,4k", "6,4k", "6,4k", "Schritte"},
		{"Schritte", 186000, "186k", "186k", "186k", "Schritte"},
		{"Schritte", 1257000, "1,3 Mio.", "1,3 Mio.", "1,3 Mio.", "Schritte"},
		{"Min.", 25.2, "25 Min.", "25′", "25", "Minuten"},
		{"km", 12.34, "12,3 km", "12,3", "12,3", "Kilometer"},
		{"Gläser", 8, "8 Gläser", "8", "8", "Gläser"},
		{"", 0.25, "0,3", "0,3", "0,3", ""},
	}
	for _, c := range cases {
		u := amountUnitOf(c.unit)
		number, word := u.parts(c.v)
		if got := u.label(c.v); got != c.label {
			t.Errorf("label %v %s: %q", c.v, c.unit, got)
		}
		if got := u.bare(c.v); got != c.bare {
			t.Errorf("bare %v %s: %q", c.v, c.unit, got)
		}
		if number != c.number || word != c.word {
			t.Errorf("parts %v %s: %q %q", c.v, c.unit, number, word)
		}
	}
}

func TestAmountDifference(t *testing.T) {
	u := amountUnitOf("L")
	for _, c := range []struct {
		diff        float64
		text, class string
	}{{1.8, "+1,8 L", "green"}, {-1.8, "−1,8 L", "orange"}, {0.01, "±0 L", ""}, {0, "±0 L", ""}} {
		if text, class := u.signed(c.diff); text != c.text || class != c.class {
			t.Errorf("%v: %q %q", c.diff, text, class)
		}
	}
	if text, _ := amountUnitOf("Schritte").signed(0); text != "±0" {
		t.Errorf("steps: %q", text)
	}
}

func TestAmountNotes(t *testing.T) {
	text := func(unit, tag string, total float64) string {
		h := &game.HabitData{Habit: model.Habit{Name: "3 Liter Wasser trinken", Tag: tag}}
		var b strings.Builder
		for _, s := range amountNote(h, amountUnitOf(unit), total) {
			b.WriteString(s.Text)
		}
		return b.String()
	}
	for _, c := range []struct {
		unit, tag string
		total     float64
		want      string
	}{
		{"L", "", 389, "Das sind etwa 2 volle Badewannen oder 778 Gläser Wasser."},
		{"L", "", 20, "Das sind etwa 40 Gläser Wasser."},
		{"km", "", 90, "Das sind etwa 2 Marathons."},
		{"Schritte", "", 186000, "Das sind etwa 130 km zu Fuß."},
		{"Min.", "lesen", 720, "Das sind etwa 2 Bücher."},
		{"Min.", "", 360, "Das sind etwa 3 Spielfilme."},
		{"Min.", "", 30, ""},
		{"Gläser", "", 500, ""},
	} {
		if got := text(c.unit, c.tag, c.total); got != c.want {
			t.Errorf("%s %v: %q", c.unit, c.total, got)
		}
	}
}

func TestAmountSumCountsOnlyWhatHappened(t *testing.T) {
	today := model.NewDay(2026, time.October, 31)
	start := today - 3
	h := &game.HabitData{Habit: model.Habit{Measure: model.MeasureAmount, Target: 3, Frequency: model.FreqDaily}, Start: start,
		Done: map[model.Day]time.Time{start: {}}}
	totals := map[model.Day]float64{start - 1: 99, start: 3.5, start + 1: 1, today: 2, today + 1: 50}
	sum := sumAmounts(h, start-5, today+5, today, totals)
	if sum.Total != 6.5 || sum.Days != 4 || sum.Reached != 1 || sum.Best != 3.5 || sum.BestDay != start {
		t.Fatalf("%+v", sum)
	}
	if got := periodGoal(h, today.WeekStart(), today.WeekStart()+6); got != float64(int(today.WeekStart()+6-start)+1)*3 {
		t.Fatalf("goal counts from the start: %v", got)
	}
}

func TestDetailShowsAmounts(t *testing.T) {
	c := newApp(t, Config{})
	c.signIn()
	id := c.meterHabit("Wasser", "amount", "3", "0,5", "L")
	for range 3 {
		c.post(fmt.Sprintf("/habits/%d/add", id), true)
	}
	month := view2(c, id, "month")
	for _, want := range []string{"täglich 3 L", "amount-hero", "1,5", "Liter", "Füllstand = Anteil vom Tagesziel", "Rekordtag", "Tage ≥ 3 L", "<b>1,5</b>"} {
		if !strings.Contains(month, want) {
			t.Fatalf("month misses %q: %s", want, month)
		}
	}
	if strings.Contains(month, "Quote 30 T.") {
		t.Fatal("the streak hero should give way to the amount")
	}
	week := view2(c, id, "week")
	for _, want := range []string{"Liter pro Tag", "goal-line", "Ø pro Tag", "bester Tag", "Ziel erreicht", "vs. Vorwoche"} {
		if !strings.Contains(week, want) {
			t.Fatalf("week misses %q: %s", want, week)
		}
	}
	year := view2(c, id, "year")
	for _, want := range []string{"Liter pro Monat", "seit Start · 1 Tag", "Ziel ≈ 90 L", "bester Monat", "Tage Serie max.", "amount-bars"} {
		if !strings.Contains(year, want) {
			t.Fatalf("year misses %q: %s", want, year)
		}
	}
	plain := c.habit("Lesen", "build")
	if html := view2(c, plain, "week"); strings.Contains(html, "amount-hero") || !strings.Contains(html, "Quote 30 T.") {
		t.Fatal("a plain habit keeps its streak view")
	}
}

func view2(c *client, id int64, name string) string {
	_, html := c.get(fmt.Sprintf("/habits/%d?view=%s", id, name))
	return html
}

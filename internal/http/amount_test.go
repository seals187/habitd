package server

import (
	"fmt"
	"strings"
	"testing"

	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/view"
)

func TestAmountNotes(t *testing.T) {
	text := func(unit, tag string, total float64) string {
		h := &game.HabitData{Habit: model.Habit{Name: "3 Liter Wasser trinken", Tag: tag}}
		var b strings.Builder
		for _, s := range amountNote(h, view.UnitOf(unit), total) {
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

package server

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/view"
)

// Rough sizes behind the comparisons in the year view.
const (
	litersBathtub   = 150
	litersGlass     = 0.5
	kmMarathon      = 42.195
	kmPerStep       = 0.0007
	minutesFilm     = 120
	minutesBook     = 360
	maxLabelledBars = 6 // more bars only have room for the bare number
	maxNamedBars    = 8 // more bars only have room for one letter
	barSpan         = 80
)

// amountBar is the sum of one month in the year view.
type amountBar struct {
	Name    string
	Label   string
	Height  int
	Current bool
	Stub    bool // the month before the habit started
}

// noteSpan is a piece of the comparison text; Bold pieces carry the numbers.
type noteSpan struct {
	Text string
	Bold bool
}

// amountData is what a habit with a unit shows instead of its streak.
type amountData struct {
	Value   string
	Unit    string
	Sub     string
	Percent int
	Side    statBox // comparison with the period before
	Hint    string
	Title   string
	Goal    string
	Bars    []amountBar
	Note    []noteSpan
}

// periodGoal is what the goals of a period add up to.
func periodGoal(h *game.HabitData, from, to, today model.Day) float64 {
	_, plan := h.Goals(from, to, today)
	return float64(plan) * h.Target
}

// newAmount fills the big figure of a period.
func newAmount(u view.Unit, total, goal float64, sub string) *amountData {
	a := &amountData{}
	a.Value, a.Unit = u.Parts(total)
	if goal > 0 {
		a.Percent = int(math.Round(total * 100 / goal))
		sub += " · Ziel " + u.Label(goal)
	}
	a.Sub = sub
	return a
}

func versus(u view.Unit, now, before float64, label string) statBox {
	box := statBox{Value: "–", Label: label}
	if before > 0 {
		box.Value, box.Class = u.Signed(now - before)
	}
	return box
}

func bestLabel(u view.Unit, sum game.AmountSum) string {
	if sum.Best == 0 {
		return "–"
	}
	return u.Label(sum.Best)
}

func averageLabel(u view.Unit, sum game.AmountSum) string {
	if sum.Days == 0 {
		return "–"
	}
	return u.Label(sum.Average())
}

func buildWeekAmount(d *detailData, h *game.HabitData, u view.Unit, start, today model.Day) {
	sum := h.Sum(start, start+6, today)
	before := h.Sum(start-7, min(start+6, today)-7, today)
	label := "diese Woche"
	if start != today.WeekStart() {
		_, week := start.Time().ISOWeek()
		label = fmt.Sprintf("KW %d", week)
	}
	d.Amount = newAmount(u, sum.Total, periodGoal(h, start, start+6, today), label)
	d.Amount.Side = statBox{Value: goalsLabel(h, start, today), Label: "Ziel erreicht", Class: "green"}
	d.Chart.Summary = u.Heading("Tag")
	bestName := "bester Tag"
	if sum.Best > 0 {
		bestName += " · " + view.Weekdays[sum.BestDay.WeekdayIndex()]
	}
	d.Stats = []statBox{
		{Value: averageLabel(u, sum), Label: "Ø pro Tag", Class: "accent"},
		{Value: bestLabel(u, sum), Label: bestName},
		versus(u, sum.Total, before.Total, "vs. Vorwoche"),
	}
}

func buildMonthAmount(d *detailData, h *game.HabitData, u view.Unit, first, last, today model.Day) {
	sum := h.Sum(first, last, today)
	beforeFirst := model.NewDay(first.Year(), first.Month()-1, 1)
	beforeLast := first - 1
	if today <= last { // a running month against the same days of the one before
		beforeLast = min(beforeFirst+(today-first), beforeLast)
	}
	before := h.Sum(beforeFirst, beforeLast, today)
	d.Amount = newAmount(u, sum.Total, periodGoal(h, first, last, today), "im "+view.Months[first.Month()-1])
	d.Amount.Side = versus(u, sum.Total, before.Total, "vs. "+view.ShortMonths[beforeFirst.Month()-1])
	name := u.Name
	if name == "" {
		name = "Menge"
	}
	d.Amount.Hint = "Zahl = " + name + " · Füllstand = Anteil vom " + goalWord(h)
	reached := "Tage ≥ " + u.Label(h.Target)
	if h.GoalWeek {
		reached = view.Plural(sum.Reached, "Woche erreicht", "Wochen erreicht")
	}
	d.Stats = []statBox{
		{Value: averageLabel(u, sum), Label: "Ø pro Tag", Class: "accent"},
		{Value: view.Number(sum.Reached), Label: reached, Class: "green"},
		{Value: bestLabel(u, sum), Label: "Rekordtag"},
	}
}

func buildYearAmount(d *detailData, h *game.HabitData, u view.Unit, first, last, today model.Day) {
	sum := h.Sum(first, last, today)
	beforeFirst := model.NewDay(first.Year(), first.Month()-time.Month(d.Span), 1)
	beforeLast := first - 1
	if today <= last { // a running window against the same stretch of the one before
		beforeLast = min(beforeFirst+(today-first), beforeLast)
	}
	before := h.Sum(beforeFirst, beforeLast, today)
	sub := d.HeatTitle
	if h.Start >= first && h.Start <= last {
		sub = fmt.Sprintf("seit Start · %d %s", sum.Days, view.Plural(sum.Days, "Tag", "Tage"))
	}
	goal := periodGoal(h, first, min(last, today), today)
	d.Amount = newAmount(u, sum.Total, goal, sub)
	d.Amount.Side = versus(u, sum.Total, before.Total, "vs. davor")
	d.Amount.Title = u.Heading("Monat")
	if sum.Days > 0 {
		d.Amount.Goal = "Ziel ≈ " + u.Label(goal/float64(sum.Days)*30)
	}
	bestMonth := buildAmountBars(d.Amount, h, u, first, d.Span, today)
	d.Amount.Note = amountNote(h, u, sum.Total)
	streakLabel := "Rekord-Serie"
	if h.Frequency == model.FreqDaily {
		streakLabel = "Tage Serie max."
	}
	d.Stats = []statBox{
		{Value: averageLabel(u, sum), Label: "Ø pro Tag", Class: "accent"},
		{Value: bestMonth, Label: "bester Monat"},
		{Value: view.Number(d.Streak.Longest), Label: streakLabel, Class: "orange"},
	}
}

func goalWord(h *game.HabitData) string {
	if h.GoalWeek {
		return "Wochenziel"
	}
	return "Tagesziel"
}

// buildAmountBars adds up the months of the window and returns the best one.
// Months before the start of the habit stay as stubs, so the window keeps its length.
func buildAmountBars(a *amountData, h *game.HabitData, u view.Unit, from model.Day, count int, today model.Day) string {
	month := func(i int) model.Day { return model.NewDay(from.Year(), from.Month()+time.Month(i), 1) }
	top, best := h.Target, "–"
	sums := make([]float64, count)
	for i := range count {
		sums[i] = h.Sum(month(i), month(i+1)-1, today).Total
		if sums[i] > top || (sums[i] == top && sums[i] > 0) {
			top, best = sums[i], view.ShortMonths[month(i).Month()-1]
		}
	}
	for i, sum := range sums {
		bar := amountBar{Name: monthName(month(i).Month(), count), Current: month(i) <= today && today < month(i+1)}
		if month(i+1) <= h.Start {
			bar.Stub = true
			a.Bars = append(a.Bars, bar)
			continue
		}
		bar.Height = max(int(sum*barSpan/top), 3)
		bar.Label = u.Bare(sum)
		if count <= maxLabelledBars && u.Kind != view.UnitTime {
			bar.Label = u.Label(sum)
		}
		a.Bars = append(a.Bars, bar)
	}
	return best
}

// amountNote puts a total into everyday terms. Too small a total stays silent.
func amountNote(h *game.HabitData, u view.Unit, total float64) []noteSpan {
	lead := noteSpan{Text: "Das sind etwa "}
	end := noteSpan{Text: "."}
	switch u.Kind {
	case view.UnitVolume:
		glasses := int(total / litersGlass)
		if glasses < 1 {
			return nil
		}
		if strings.Contains(strings.ToLower(h.Name), "wasser") {
			end.Text = " Wasser."
		}
		count := noteSpan{Text: view.Number(glasses) + view.Plural(glasses, " Glas", " Gläser"), Bold: true}
		if tubs := int(total / litersBathtub); tubs >= 1 {
			return []noteSpan{lead, {Text: view.Number(tubs) + view.Plural(tubs, " volle Badewanne", " volle Badewannen"), Bold: true}, {Text: " oder "}, count, end}
		}
		return []noteSpan{lead, count, end}
	case view.UnitDistance:
		if n := int(total / kmMarathon); n >= 1 {
			return []noteSpan{lead, {Text: view.Number(n) + view.Plural(n, " Marathon", " Marathons"), Bold: true}, end}
		}
	case view.UnitSteps:
		if km := int(total * kmPerStep); km >= 1 {
			return []noteSpan{lead, {Text: view.Number(km) + " km", Bold: true}, {Text: " zu Fuß."}}
		}
	case view.UnitTime:
		if h.Tag == "lesen" {
			if n := int(total / minutesBook); n >= 1 {
				return []noteSpan{lead, {Text: view.Number(n) + view.Plural(n, " Buch", " Bücher"), Bold: true}, end}
			}
		}
		if n := int(total / minutesFilm); n >= 1 {
			return []noteSpan{lead, {Text: view.Number(n) + view.Plural(n, " Spielfilm", " Spielfilme"), Bold: true}, end}
		}
	}
	return nil
}

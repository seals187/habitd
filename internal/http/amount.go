package server

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
	"github.com/seals187/habitd/internal/view"
)

// Kinds of units the amounts are formatted for.
const (
	unitPlain    = ""
	unitTime     = "time"
	unitSteps    = "steps"
	unitVolume   = "volume"
	unitDistance = "distance"
)

// Rough sizes behind the comparisons in the year view.
const (
	litersBathtub   = 150
	litersGlass     = 0.5
	kmMarathon      = 42.195
	kmPerStep       = 0.0007
	minutesFilm     = 120
	minutesBook     = 360
	stepsPerKilo    = 1000
	stepsPerMillion = 1000000
	minutesPerHour  = 60
	maxLabelledBars = 6 // more bars only have room for the bare number
	maxNamedBars    = 8 // more bars only have room for one letter
	barSpan         = 80
)

// amountUnit formats the amounts of one unit.
type amountUnit struct {
	kind  string
	name  string // long form, e.g. Liter
	short string // behind a number, e.g. L
}

func amountUnitOf(unit string) amountUnit {
	unit = strings.TrimSpace(unit)
	key := strings.ToLower(strings.TrimSuffix(unit, "."))
	switch {
	case view.IsMinutes(unit):
		return amountUnit{kind: unitTime, name: "Minuten", short: "Min."}
	case key == "l" || key == "liter" || key == "litre":
		return amountUnit{kind: unitVolume, name: "Liter", short: "L"}
	case key == "km" || key == "kilometer":
		return amountUnit{kind: unitDistance, name: "Kilometer", short: "km"}
	case key == "schritte" || key == "schritt" || key == "steps":
		return amountUnit{kind: unitSteps, name: "Schritte", short: "Schritte"}
	}
	return amountUnit{name: unit, short: unit}
}

// compact rounds to what is worth reading: one decimal below 100, none above.
func compact(v float64) string {
	if math.Abs(v) >= 100 {
		return view.Number(int(math.Round(v)))
	}
	return view.Decimal(math.Round(v*10) / 10)
}

func oneDecimal(v float64) string {
	return strings.ReplaceAll(strconv.FormatFloat(v, 'f', 1, 64), ".", ",")
}

// parts splits an amount into the number and the word of the big figure.
func (u amountUnit) parts(v float64) (string, string) {
	switch {
	case u.kind == unitTime && v >= minutesPerHour:
		hours := compact(v / minutesPerHour)
		if hours == "1" {
			return hours, "Stunde"
		}
		return hours, "Stunden"
	case u.kind == unitSteps:
		return u.bare(v), u.name
	case u.kind == unitTime:
		return view.Number(int(math.Round(v))), u.name
	}
	return compact(v), u.name
}

// label is an amount with its short unit: 2,5 L or 1,5 Std.
func (u amountUnit) label(v float64) string {
	switch {
	case u.kind == unitTime && v >= minutesPerHour:
		return compact(v/minutesPerHour) + " Std."
	case u.kind == unitSteps:
		return u.bare(v)
	case u.kind == unitTime:
		return view.Number(int(math.Round(v))) + " " + u.short
	}
	return strings.TrimSpace(compact(v) + " " + u.short)
}

// bare is an amount for tight spots like a bar or a calendar day.
func (u amountUnit) bare(v float64) string {
	switch u.kind {
	case unitTime:
		if v >= minutesPerHour {
			return compact(v/minutesPerHour) + "h"
		}
		return view.Number(int(math.Round(v))) + "′"
	case unitSteps:
		switch {
		case v >= stepsPerMillion:
			return compact(v/stepsPerMillion) + " Mio."
		case v >= stepsPerKilo:
			return compact(v/stepsPerKilo) + "k"
		}
	case unitVolume, unitDistance:
		if v < 100 {
			return oneDecimal(v)
		}
	}
	return compact(v)
}

// heading names the amounts of a chart.
func (u amountUnit) heading(per string) string {
	switch {
	case u.name == "":
		return "Menge pro " + per
	case u.kind == unitTime && per == "Monat":
		return "Stunden pro Monat"
	}
	return u.name + " pro " + per
}

// signed is the difference between two periods. Rounding decides what counts as
// no change, so a label never reads +0.
func (u amountUnit) signed(diff float64) (string, string) {
	text := u.label(math.Abs(diff))
	if strings.Trim(text, "0,. ") == strings.TrimSpace(u.short) || !strings.ContainsAny(text, "123456789") {
		return "±" + text, ""
	}
	if diff < 0 {
		return "−" + text, "orange"
	}
	return "+" + text, "green"
}

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

// amountSum adds up the days of a period, as far as they have happened.
type amountSum struct {
	Total   float64
	Days    int
	Reached int
	Best    float64
	BestDay model.Day
}

func (a amountSum) average() float64 { return a.Total / float64(max(a.Days, 1)) }

func sumAmounts(h *game.HabitData, from, to, today model.Day, totals map[model.Day]float64) amountSum {
	var sum amountSum
	for x := max(from, h.Start); x <= min(to, today); x++ {
		v := valueOn(h, x, today, totals)
		sum.Total += v
		sum.Days++
		if hasDone(h, x) {
			sum.Reached++
		}
		if v > sum.Best {
			sum.Best, sum.BestDay = v, x
		}
	}
	return sum
}

// periodGoal is what the planned days of a period add up to.
func periodGoal(h *game.HabitData, from, to model.Day) float64 {
	_, plan := planned(h, from, to)
	return float64(plan) * h.Target
}

// newAmount fills the big figure of a period.
func newAmount(u amountUnit, total, goal float64, sub string) *amountData {
	a := &amountData{}
	a.Value, a.Unit = u.parts(total)
	if goal > 0 {
		a.Percent = int(math.Round(total * 100 / goal))
		sub += " · Ziel " + u.label(goal)
	}
	a.Sub = sub
	return a
}

func versus(u amountUnit, now, before float64, label string) statBox {
	box := statBox{Value: "–", Label: label}
	if before > 0 {
		box.Value, box.Class = u.signed(now - before)
	}
	return box
}

func bestLabel(u amountUnit, sum amountSum) string {
	if sum.Best == 0 {
		return "–"
	}
	return u.label(sum.Best)
}

func averageLabel(u amountUnit, sum amountSum) string {
	if sum.Days == 0 {
		return "–"
	}
	return u.label(sum.average())
}

func buildWeekAmount(d *detailData, h *game.HabitData, u amountUnit, start, today model.Day, totals map[model.Day]float64) {
	sum := sumAmounts(h, start, start+6, today, totals)
	before := sumAmounts(h, start-7, min(start+6, today)-7, today, totals)
	label := "diese Woche"
	if start != today.WeekStart() {
		_, week := start.Time().ISOWeek()
		label = fmt.Sprintf("KW %d", week)
	}
	d.Amount = newAmount(u, sum.Total, periodGoal(h, start, start+6), label)
	done, plan := planned(h, start, start+6)
	d.Amount.Side = statBox{Value: fmt.Sprintf("%d/%d", done, plan), Label: "Ziel erreicht", Class: "green"}
	d.Chart.Summary = u.heading("Tag")
	bestName := "bester Tag"
	if sum.Best > 0 {
		bestName += " · " + dayNames[sum.BestDay.WeekdayIndex()]
	}
	d.Stats = []statBox{
		{Value: averageLabel(u, sum), Label: "Ø pro Tag", Class: "accent"},
		{Value: bestLabel(u, sum), Label: bestName},
		versus(u, sum.Total, before.Total, "vs. Vorwoche"),
	}
}

func buildMonthAmount(d *detailData, h *game.HabitData, u amountUnit, first, last, today model.Day, totals map[model.Day]float64) {
	sum := sumAmounts(h, first, last, today, totals)
	beforeFirst := model.NewDay(first.Year(), first.Month()-1, 1)
	beforeLast := first - 1
	if today <= last { // a running month against the same days of the one before
		beforeLast = min(beforeFirst+(today-first), beforeLast)
	}
	before := sumAmounts(h, beforeFirst, beforeLast, today, totals)
	d.Amount = newAmount(u, sum.Total, periodGoal(h, first, last), "im "+view.Months[first.Month()-1])
	d.Amount.Side = versus(u, sum.Total, before.Total, "vs. "+view.ShortMonths[beforeFirst.Month()-1])
	name := u.name
	if name == "" {
		name = "Menge"
	}
	d.Amount.Hint = "Zahl = " + name + " · Füllstand = Anteil vom " + goalWord(h)
	reached := "Tage ≥ " + u.label(h.Target)
	if h.GoalWeek {
		reached = plural(sum.Reached, "Woche erreicht", "Wochen erreicht")
	}
	d.Stats = []statBox{
		{Value: averageLabel(u, sum), Label: "Ø pro Tag", Class: "accent"},
		{Value: view.Number(sum.Reached), Label: reached, Class: "green"},
		{Value: bestLabel(u, sum), Label: "Rekordtag"},
	}
}

func buildYearAmount(d *detailData, h *game.HabitData, u amountUnit, first, last, today model.Day, totals map[model.Day]float64) {
	sum := sumAmounts(h, first, last, today, totals)
	beforeFirst := model.NewDay(first.Year(), first.Month()-time.Month(d.Span), 1)
	beforeLast := first - 1
	if today <= last { // a running window against the same stretch of the one before
		beforeLast = min(beforeFirst+(today-first), beforeLast)
	}
	before := sumAmounts(h, beforeFirst, beforeLast, today, totals)
	sub := d.HeatTitle
	if h.Start >= first && h.Start <= last {
		sub = fmt.Sprintf("seit Start · %d %s", sum.Days, dayWord(sum.Days))
	}
	goal := periodGoal(h, first, min(last, today))
	d.Amount = newAmount(u, sum.Total, goal, sub)
	d.Amount.Side = versus(u, sum.Total, before.Total, "vs. davor")
	d.Amount.Title = u.heading("Monat")
	if sum.Days > 0 {
		d.Amount.Goal = "Ziel ≈ " + u.label(goal/float64(sum.Days)*30)
	}
	bestMonth := buildAmountBars(d.Amount, h, u, first, d.Span, today, totals)
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

func dayWord(n int) string {
	if n == 1 {
		return "Tag"
	}
	return "Tage"
}

// buildAmountBars adds up the months of the window and returns the best one.
// Months before the start of the habit stay as stubs, so the window keeps its length.
func buildAmountBars(a *amountData, h *game.HabitData, u amountUnit, from model.Day, count int, today model.Day, totals map[model.Day]float64) string {
	month := func(i int) model.Day { return model.NewDay(from.Year(), from.Month()+time.Month(i), 1) }
	top, best := h.Target, "–"
	sums := make([]float64, count)
	for i := range count {
		sums[i] = sumAmounts(h, month(i), month(i+1)-1, today, totals).Total
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
		bar.Label = u.bare(sum)
		if count <= maxLabelledBars && u.kind != unitTime {
			bar.Label = u.label(sum)
		}
		a.Bars = append(a.Bars, bar)
	}
	return best
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// amountNote puts a total into everyday terms. Too small a total stays silent.
func amountNote(h *game.HabitData, u amountUnit, total float64) []noteSpan {
	lead := noteSpan{Text: "Das sind etwa "}
	end := noteSpan{Text: "."}
	switch u.kind {
	case unitVolume:
		glasses := int(total / litersGlass)
		if glasses < 1 {
			return nil
		}
		if strings.Contains(strings.ToLower(h.Name), "wasser") {
			end.Text = " Wasser."
		}
		count := noteSpan{Text: view.Number(glasses) + plural(glasses, " Glas", " Gläser"), Bold: true}
		if tubs := int(total / litersBathtub); tubs >= 1 {
			return []noteSpan{lead, {Text: view.Number(tubs) + plural(tubs, " volle Badewanne", " volle Badewannen"), Bold: true}, {Text: " oder "}, count, end}
		}
		return []noteSpan{lead, count, end}
	case unitDistance:
		if n := int(total / kmMarathon); n >= 1 {
			return []noteSpan{lead, {Text: view.Number(n) + plural(n, " Marathon", " Marathons"), Bold: true}, end}
		}
	case unitSteps:
		if km := int(total * kmPerStep); km >= 1 {
			return []noteSpan{lead, {Text: view.Number(km) + " km", Bold: true}, {Text: " zu Fuß."}}
		}
	case unitTime:
		if h.Tag == "lesen" {
			if n := int(total / minutesBook); n >= 1 {
				return []noteSpan{lead, {Text: view.Number(n) + plural(n, " Buch", " Bücher"), Bold: true}, end}
			}
		}
		if n := int(total / minutesFilm); n >= 1 {
			return []noteSpan{lead, {Text: view.Number(n) + plural(n, " Spielfilm", " Spielfilme"), Bold: true}, end}
		}
	}
	return nil
}

// step is an amount with its unit on a line of text: 0,5 L or 5′.
func (u amountUnit) step(v float64) string {
	if u.kind == unitTime {
		return u.bare(v)
	}
	return u.label(v)
}

// value is the running amount in front of its goal: 2,5 in 2,5 / 3 L.
func (u amountUnit) value(v float64) string {
	if u.kind == unitTime {
		return view.Number(int(math.Round(v)))
	}
	return compact(v)
}

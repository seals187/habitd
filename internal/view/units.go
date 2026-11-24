package view

import (
	"math"
	"strconv"
	"strings"

	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

// Weekdays are the short German weekday names, Monday first.
var Weekdays = [7]string{"Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"}

// Kinds of units the amounts are formatted for.
const (
	UnitPlain    = ""
	UnitTime     = "time"
	UnitSteps    = "steps"
	UnitVolume   = "volume"
	UnitDistance = "distance"
)

const (
	minutesPerHour  = 60
	stepsPerKilo    = 1000
	stepsPerMillion = 1000000
	maxTagUnit      = 3 // the longest unit of its own that still fits beside a number on a day
)

// Unit formats the amounts of one unit of a habit.
type Unit struct {
	Kind  string
	Name  string // long form, e.g. Liter
	Short string // behind a number, e.g. L
}

// UnitOf recognises the unit a habit was given.
func UnitOf(unit string) Unit {
	unit = strings.TrimSpace(unit)
	key := strings.ToLower(strings.TrimSuffix(unit, "."))
	switch {
	case IsMinutes(unit):
		return Unit{Kind: UnitTime, Name: "Minuten", Short: "Min."}
	case key == "l" || key == "liter" || key == "litre":
		return Unit{Kind: UnitVolume, Name: "Liter", Short: "L"}
	case key == "km" || key == "kilometer":
		return Unit{Kind: UnitDistance, Name: "Kilometer", Short: "km"}
	case key == "schritte" || key == "schritt" || key == "steps":
		return Unit{Kind: UnitSteps, Name: "Schritte", Short: "Schritte"}
	}
	return Unit{Name: unit, Short: unit}
}

// Compact rounds to what is worth reading: one decimal below 100, none above.
func Compact(v float64) string {
	if math.Abs(v) >= 100 {
		return Number(int(math.Round(v)))
	}
	return Decimal(math.Round(v*10) / 10)
}

func oneDecimal(v float64) string {
	return strings.ReplaceAll(strconv.FormatFloat(v, 'f', 1, 64), ".", ",")
}

// Parts splits an amount into the number and the word of a big figure.
func (u Unit) Parts(v float64) (string, string) {
	switch {
	case u.Kind == UnitTime && v >= minutesPerHour:
		hours := Compact(v / minutesPerHour)
		if hours == "1" {
			return hours, "Stunde"
		}
		return hours, "Stunden"
	case u.Kind == UnitSteps:
		return u.Bare(v), u.Name
	case u.Kind == UnitTime:
		return Number(int(math.Round(v))), u.Name
	}
	return Compact(v), u.Name
}

// Label is an amount with its short unit: 2,5 L or 1,5 Std.
func (u Unit) Label(v float64) string {
	switch {
	case u.Kind == UnitTime && v >= minutesPerHour:
		return Compact(v/minutesPerHour) + " Std."
	case u.Kind == UnitSteps:
		return u.Bare(v)
	case u.Kind == UnitTime:
		return Number(int(math.Round(v))) + " " + u.Short
	}
	return strings.TrimSpace(Compact(v) + " " + u.Short)
}

// Bare is an amount for tight spots like a bar or a calendar day.
func (u Unit) Bare(v float64) string {
	switch u.Kind {
	case UnitTime:
		if v >= minutesPerHour {
			return Compact(v/minutesPerHour) + "h"
		}
		return Number(int(math.Round(v))) + "′"
	case UnitSteps:
		switch {
		case v >= stepsPerMillion:
			return Compact(v/stepsPerMillion) + " Mio."
		case v >= stepsPerKilo:
			return Compact(v/stepsPerKilo) + "k"
		}
	case UnitVolume, UnitDistance:
		if v < 100 {
			return oneDecimal(v)
		}
	}
	return Compact(v)
}

// Tag is the amount of a single day in the week and the month, with its unit
// written on: 2,8l or 20min. A long unit of its own leaves the bare number.
func (u Unit) Tag(v float64) string {
	switch {
	case u.Kind == UnitTime && v < minutesPerHour:
		return Number(int(math.Round(v))) + "min"
	case u.Kind == UnitVolume:
		return u.Bare(v) + "l"
	case u.Kind == UnitDistance:
		return u.Bare(v) + "km"
	case u.Kind == UnitPlain && u.Short != "" && len([]rune(u.Short)) <= maxTagUnit:
		return u.Bare(v) + u.Short
	}
	return u.Bare(v)
}

// Over shows a goal that was passed in a short form: 3,5/3 l or 40/20 min.
func (u Unit) Over(value, goal float64) string {
	switch u.Kind {
	case UnitTime:
		return Compact(value) + "/" + Compact(goal) + " min"
	case UnitVolume:
		return Compact(value) + "/" + Compact(goal) + " l"
	case UnitSteps:
		return u.Bare(value) + "/" + u.Bare(goal) + " Schr."
	}
	return strings.TrimSpace(Compact(value) + "/" + Compact(goal) + " " + u.Short)
}

// Heading names the amounts of a chart.
func (u Unit) Heading(per string) string {
	switch {
	case u.Name == "":
		return "Menge pro " + per
	case u.Kind == UnitTime && per == "Monat":
		return "Stunden pro Monat"
	}
	return u.Name + " pro " + per
}

// Signed is the difference between two periods with the CSS class of its
// direction. Rounding decides what counts as no change, so a label never reads +0.
func (u Unit) Signed(diff float64) (string, string) {
	text := u.Label(math.Abs(diff))
	if strings.Trim(text, "0,. ") == strings.TrimSpace(u.Short) || !strings.ContainsAny(text, "123456789") {
		return "±" + text, ""
	}
	if diff < 0 {
		return "−" + text, "orange"
	}
	return "+" + text, "green"
}

// Step is an amount with its unit on a line of text: 0,5 L or 5′.
func (u Unit) Step(v float64) string {
	if u.Kind == UnitTime {
		return u.Bare(v)
	}
	return u.Label(v)
}

// Value is the running amount in front of its goal: 2,5 in 2,5 / 3 L.
func (u Unit) Value(v float64) string {
	if u.Kind == UnitTime {
		return Number(int(math.Round(v)))
	}
	return Compact(v)
}

// Plural picks the word for a count.
func Plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Streak units as they stand beside a number on a tile.
const (
	StreakDays   = "T"
	StreakWeeks  = "Wo"
	StreakMonths = "Mo"
)

// StreakShort is the short unit of a streak: T, Wo or Mo.
func StreakShort(s game.Streak) string {
	switch {
	case s.Monthly:
		return StreakMonths
	case s.Weekly:
		return StreakWeeks
	}
	return StreakDays
}

// Span writes a count of streak units out: 12 Tage, 1 Woche.
func Span(n int, short string) string {
	words := map[string][2]string{StreakDays: {"Tag", "Tage"}, StreakWeeks: {"Woche", "Wochen"}, StreakMonths: {"Monat", "Monate"}}[short]
	return strconv.Itoa(n) + " " + Plural(n, words[0], words[1])
}

// StreakTag is a streak on a card: a bare number of days, else 5 Wo or 2 Mo.
func StreakTag(s game.Streak) string {
	if short := StreakShort(s); short != StreakDays {
		return strconv.Itoa(s.Current) + " " + short
	}
	return Number(s.Current)
}

// RhythmLabel is the rhythm of a habit with the goal of a measured one: täglich 3 L.
func RhythmLabel(h model.Habit) string {
	if h.Measured() && !h.GoalWeek {
		return FrequencyLabel(h) + " " + UnitOf(h.Unit).Label(h.Target)
	}
	return FrequencyLabel(h)
}

package game

import (
	"time"

	"github.com/seals187/habitd/internal/model"
)

// easter returns Easter Sunday of a year (anonymous Gregorian algorithm).
func easter(y int) model.Day {
	a := y % 19
	b, c := y/100, y%100
	d, e := b/4, b%4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i, k := c/4, c%4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1
	return model.NewDay(y, time.Month(month), day)
}

// IsGermanHoliday reports the nationwide German public holidays.
func IsGermanHoliday(d model.Day) bool {
	switch {
	case d.Month() == time.January && d.Dom() == 1,
		d.Month() == time.May && d.Dom() == 1,
		d.Month() == time.October && d.Dom() == 3,
		d.Month() == time.December && (d.Dom() == 25 || d.Dom() == 26):
		return true
	}
	e := easter(d.Year())
	for _, offset := range []model.Day{-2, 1, 39, 50} { // Karfreitag, Ostermontag, Himmelfahrt, Pfingstmontag
		if d == e+offset {
			return true
		}
	}
	return false
}

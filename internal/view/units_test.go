package view

import "testing"

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
		u := UnitOf(c.unit)
		number, word := u.Parts(c.v)
		if got := u.Label(c.v); got != c.label {
			t.Errorf("label %v %s: %q", c.v, c.unit, got)
		}
		if got := u.Bare(c.v); got != c.bare {
			t.Errorf("bare %v %s: %q", c.v, c.unit, got)
		}
		if number != c.number || word != c.word {
			t.Errorf("parts %v %s: %q %q", c.v, c.unit, number, word)
		}
	}
}

func TestAmountDifference(t *testing.T) {
	u := UnitOf("L")
	for _, c := range []struct {
		diff        float64
		text, class string
	}{{1.8, "+1,8 L", "green"}, {-1.8, "−1,8 L", "orange"}, {0.01, "±0 L", ""}, {0, "±0 L", ""}} {
		if text, class := u.Signed(c.diff); text != c.text || class != c.class {
			t.Errorf("%v: %q %q", c.diff, text, class)
		}
	}
	if text, _ := UnitOf("Schritte").Signed(0); text != "±0" {
		t.Errorf("steps: %q", text)
	}
}

func TestDayTag(t *testing.T) {
	cases := []struct {
		unit string
		v    float64
		want string
	}{
		{"l", 3, "3,0l"},
		{"L", 2.8, "2,8l"},
		{"Min.", 20, "20min"},
		{"Min.", 90, "1,5h"},
		{"km", 5.2, "5,2km"},
		{"Schritte", 6400, "6,4k"},
		{"x", 4, "4x"},
		{"Seiten", 12, "12"},
		{"", 2, "2"},
	}
	for _, tc := range cases {
		if got := UnitOf(tc.unit).Tag(tc.v); got != tc.want {
			t.Fatalf("%q %v: got %q, want %q", tc.unit, tc.v, got, tc.want)
		}
	}
}

func TestStreakWords(t *testing.T) {
	for _, c := range []struct {
		n     int
		short string
		want  string
	}{{1, StreakDays, "1 Tag"}, {12, StreakDays, "12 Tage"}, {1, StreakWeeks, "1 Woche"}, {3, StreakMonths, "3 Monate"}} {
		if got := Span(c.n, c.short); got != c.want {
			t.Errorf("%d %s: %q", c.n, c.short, got)
		}
	}
}

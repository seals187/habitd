// Package tips turns the history of a habit into short hints. Everything is
// calculated by fixed rules, nothing leaves the server and no AI is involved.
//
// A rule is a small function that looks at an Input and may return a Tip. To add
// one, write the function and list it in general or in the rules of its kind.
package tips

import (
	"cmp"
	"slices"
	"time"

	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

const (
	// MinDays is how long a habit runs before rules about days have a say.
	MinDays = 7
	// MinEntries is how many moments rules about the time of day want to see.
	MinEntries = 5
	// MaxTips is how many hints a habit shows at most.
	MaxTips = 3
	// Window is how far back the stamps of a habit are worth looking at.
	Window = 28

	noon = 12 * 60

	firstHour = 6 // the strip of hours starts here, earlier moments count for it
	maxLevel  = 4
)

// Tip is one hint. Score sorts the tips of a habit, the highest comes first.
type Tip struct {
	Icon   string
	Tone   string // CSS class of the icon
	Title  string
	Detail string
	Score  int
}

// Stamp is something that really happened at a time of day.
type Stamp struct {
	Day    model.Day
	At     time.Time
	Amount float64
}

// minutes is the time of day of a stamp.
func (s Stamp) minutes() int { return s.At.Hour()*60 + s.At.Minute() }

// Input is what the rules get to see. Stamps and Done only hold real moments of
// the last Window days, days filled in afterwards have no time of day.
type Input struct {
	Habit  *game.HabitData
	Streak game.Streak
	Today  model.Day
	Now    time.Time
	Stamps []Stamp                // every click or check-in
	Done   []Stamp                // the moments a goal was reached
	Format func(v float64) string // amount with its unit, such as 0,4 L
	Drink  bool                   // the habit is about drinking
}

func (in Input) enoughDays() bool {
	return int(in.Today-in.Habit.Start)+1 >= MinDays
}

func (in Input) amount(v float64) string {
	if in.Format == nil {
		return ""
	}
	return in.Format(v)
}

func (in Input) avoid() bool { return in.Habit.Kind == model.KindAvoid }

// weekly reports habits that count per week, not per day.
func (in Input) weekly() bool {
	return in.Habit.GoalWeek || in.Habit.Frequency == model.FreqWeekly
}

// daily reports build habits with days of their own.
func (in Input) daily() bool {
	h := in.Habit
	return !in.avoid() && !h.GoalWeek && (h.Frequency == model.FreqDaily || h.Frequency == model.FreqCustom)
}

func (in Input) done(d model.Day) bool {
	_, ok := in.Habit.Done[d]
	return ok
}

// rule looks at a habit and has something to say or not.
type rule func(Input) (Tip, bool)

// general holds the rules for all habits, each checks on its own whom it fits.
var general = []rule{typicalTime, bestWeekday, weakWeekday, trend, streakAtRisk, recordInReach}

// rulesFor adds the rules of the kind of the habit.
func rulesFor(in Input) []rule {
	switch {
	case in.avoid():
		return append(slices.Clone(general), avoidRun, avoidRiskyDay)
	case in.weekly():
		return append(slices.Clone(general), weekPace, weekFavorites)
	case in.Habit.Measured():
		return append(slices.Clone(general), morningShare, almostThere, aboveGoal)
	}
	return append(slices.Clone(general), checkEarly, checkSteady)
}

// For returns the most relevant tips of a habit.
func For(in Input) []Tip {
	var out []Tip
	for _, r := range rulesFor(in) {
		if tip, ok := r(in); ok {
			out = append(out, tip)
		}
	}
	slices.SortStableFunc(out, func(a, b Tip) int { return cmp.Compare(b.Score, a.Score) })
	return out[:min(len(out), MaxTips)]
}

// Hours spreads the stamps over the hours from 6 to 24. Each value is a level
// from 0 to 4 for the strip, the result is nil while there is too little to show.
func Hours(in Input) []int {
	if in.avoid() || len(in.Stamps) < MinEntries {
		return nil
	}
	sums := make([]float64, 24-firstHour)
	top := 0.0
	for _, s := range in.Stamps {
		i := max(s.At.Hour()-firstHour, 0)
		sums[i] += max(s.Amount, 1e-9)
		top = max(top, sums[i])
	}
	levels := make([]int, len(sums))
	for i, v := range sums {
		if v > 0 {
			levels[i] = max(int(v/top*maxLevel+0.5), 1)
		}
	}
	return levels
}

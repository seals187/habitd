// Package game contains the pure gamification rules: XP, Taler, levels,
// streaks, jokers and badges. Nothing in here touches the database or the
// clock, which keeps every rule directly testable.
package game

import (
	"math"

	"github.com/seals187/habitd/internal/model"
)

// Fixed amounts from the game rules.
const (
	AvoidXP       = 20  // XP per clean day of an avoid habit
	QuestCoins    = 50  // daily quest bonus
	LevelUpCoins  = 100 // bonus per level up
	BadgeCoins    = 50  // bonus per unlocked badge
	JokerPrice    = 120 // shop price of a joker
	MaxJokers     = 3
	JokerInterval = 7 // one joker per this many streak days
	BackfillDays  = 7 // how far back check-ins can be edited
)

// Level curve parameters. The curve is progressive like 100*level^1.5, but
// calibrated to the design mockups, see docs/DECISIONS.md.
const (
	levelBase = 21.57
	levelExp  = 1.822
)

// BaseXP is the XP of one check-in before the streak bonus.
func BaseXP(h model.Habit) int {
	if h.Kind == model.KindAvoid {
		return AvoidXP
	}
	switch h.Difficulty {
	case model.Medium:
		return 15
	case model.Hard:
		return 25
	default:
		return 10
	}
}

// Multiplier returns the streak bonus for a streak measured in days.
func Multiplier(streakDays int) float64 {
	switch {
	case streakDays >= 30:
		return 2
	case streakDays >= 7:
		return 1.5
	default:
		return 1
	}
}

// AwardXP is the XP for one check-in at the given streak length.
func AwardXP(h model.Habit, streakDays int) int {
	return int(math.Round(float64(BaseXP(h)) * Multiplier(streakDays)))
}

// CoinsFor converts earned XP to Taler: one third, rounded.
func CoinsFor(xp int) int { return int(math.Round(float64(xp) / 3)) }

// LevelCost is the XP needed to get from the given level to the next one.
func LevelCost(level int) int {
	return int(math.Round(levelBase*math.Pow(float64(level), levelExp)/10)) * 10
}

// LevelXP returns the total XP at which a level starts. Level 1 starts at 0.
func LevelXP(level int) int {
	total := 0
	for l := 1; l < level; l++ {
		total += LevelCost(l)
	}
	return total
}

// LevelFor returns the level reached with the given total XP.
func LevelFor(xp int) int {
	level := 1
	for remaining := xp; remaining >= LevelCost(level); level++ {
		remaining -= LevelCost(level)
	}
	return level
}

// Progress describes the position inside a level.
type Progress struct {
	Level   int
	Into    int // XP earned inside the level
	Cost    int // XP the level costs
	Missing int // XP left until the next level
	Percent int
}

// ProgressFor computes level progress. minLevel is the highest level ever
// reached: levels are never lost, even when undoing check-ins lowers the XP.
func ProgressFor(xp, minLevel int) Progress {
	level := max(LevelFor(xp), minLevel, 1)
	cost := LevelCost(level)
	into := max(xp-LevelXP(level), 0)
	return Progress{Level: level, Into: into, Cost: cost, Missing: cost - into, Percent: min(into*100/cost, 100)}
}

var titles = []struct {
	level int
	title string
}{
	{1, "Einsteiger"},
	{3, "Entdecker"},
	{6, "Durchstarter"},
	{9, "Dranbleiber"},
	{12, "Gewohnheitsheld"},
	{13, "Routine-Meister"},
	{20, "Disziplin-Profi"},
	{30, "Gewohnheits-Virtuose"},
	{40, "Großmeister"},
	{50, "Lebenskünstler"},
}

// Title returns the title of a level.
func Title(level int) string {
	t := titles[0].title
	for _, e := range titles {
		if level >= e.level {
			t = e.title
		}
	}
	return t
}

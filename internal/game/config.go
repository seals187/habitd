package game

import (
	"math"
	"slices"
)

// Tempo presets scale the formula of the level curve.
const (
	TempoRelaxed   = "gemuetlich"
	TempoNormal    = "normal"
	TempoAmbitious = "ehrgeizig"
)

// TempoFactor returns the scale of a tempo preset.
func TempoFactor(tempo string) float64 {
	switch tempo {
	case TempoRelaxed:
		return 0.7
	case TempoAmbitious:
		return 1.4
	}
	return 1
}

// LevelCount is the number of levels the table holds.
const LevelCount = 50

// Level is one row of the level table.
type Level struct {
	Level      int
	Title      string
	XP         int // total XP at which the level starts
	Coins      int
	Jokers     int
	BadgeKey   string
	Overridden bool // XP set by hand instead of by the formula
}

// LevelSettings configure the curve and its rules.
type LevelSettings struct {
	Base      float64
	Exponent  float64
	Tempo     string
	MaxLevel  int
	Limit     bool // stop at MaxLevel
	Celebrate bool
	NoDrop    bool // levels are never lost
	ShowTitle bool
}

// Cost is the XP from a level to the next one under these settings.
func (s LevelSettings) Cost(level int) int {
	return int(math.Round(s.Base*TempoFactor(s.Tempo)*math.Pow(float64(level), s.Exponent)/10)) * 10
}

// XP returns the total XP of a level under these settings.
func (s LevelSettings) XP(level int) int {
	total := 0
	for l := 1; l < level; l++ {
		total += s.Cost(l)
	}
	return total
}

// Rules are all global game rules: the admin area changes them at runtime, so a
// value is never modified after it was built.
type Rules struct {
	Categories []Category
	Badges     []Badge
	Levels     []Level
	Settings   LevelSettings
}

var levelBadges = map[int]string{5: "aufsteiger", 10: "erfahren", 25: "profi", 50: "meister"}

// DefaultLevelSettings are the start values of the curve.
func DefaultLevelSettings() LevelSettings {
	return LevelSettings{Base: levelBase, Exponent: levelExp, Tempo: TempoNormal, MaxLevel: LevelCount,
		Celebrate: true, NoDrop: true, ShowTitle: true}
}

// DefaultLevels are the start values of the level table.
func DefaultLevels() []Level {
	s := DefaultLevelSettings()
	out := make([]Level, 0, LevelCount)
	for l := 1; l <= LevelCount; l++ {
		row := Level{Level: l, Title: startTitle(l), XP: s.XP(l), BadgeKey: levelBadges[l]}
		if l > 1 {
			row.Coins, row.Jokers = LevelUpCoins, 1
		}
		out = append(out, row)
	}
	return out
}

// DefaultRules are the rules before anything was configured.
func DefaultRules() *Rules {
	badges := make([]Badge, len(Badges))
	for i, b := range Badges {
		b.Description, b.Coins, b.Sort, b.Active, b.Builtin = badgeRules[b.Key], BadgeCoins, i+1, true, true
		badges[i] = b
	}
	categories := slices.Clone(Categories)
	for i := range categories {
		categories[i].Sort = i + 1
	}
	return &Rules{Categories: categories, Badges: badges, Levels: DefaultLevels(), Settings: DefaultLevelSettings()}
}

var defaults = DefaultRules()

// Special returns the code behind a built-in special badge.
func Special(key string) func(*Facts) int {
	for _, b := range Badges {
		if b.Key == key {
			return b.Metric.special
		}
	}
	return nil
}

// WithSpecial attaches the code of a special badge loaded from storage.
func (m Metric) WithSpecial(key string) Metric {
	if m.Rule == RuleSpecial {
		m.special = Special(key)
	}
	return m
}

// LevelXP returns the total XP at which a level starts. Levels beyond the table
// continue with the formula.
func (r *Rules) LevelXP(level int) int {
	n := len(r.Levels)
	if level <= n {
		return r.Levels[max(level, 1)-1].XP
	}
	total := r.Levels[n-1].XP
	for l := n; l < level; l++ {
		total += r.Settings.Cost(l)
	}
	return total
}

// LevelFor returns the level reached with the given total XP.
func (r *Rules) LevelFor(xp int) int {
	level := 1
	for xp >= r.LevelXP(level+1) {
		level++
		if r.Settings.Limit && level >= r.Settings.MaxLevel {
			return r.Settings.MaxLevel
		}
	}
	return level
}

// Level returns the table row of a level. Levels beyond the table repeat the
// title and rewards of its last row.
func (r *Rules) Level(level int) Level {
	if level >= 1 && level <= len(r.Levels) {
		return r.Levels[level-1]
	}
	row := r.Levels[len(r.Levels)-1]
	row.Level, row.BadgeKey = level, ""
	return row
}

// Title returns the title of a level.
func (r *Rules) Title(level int) string { return r.Level(level).Title }

// ProgressFor computes level progress. minLevel is the highest level reached
// so far; while levels cannot drop it wins over a lower XP total.
func (r *Rules) ProgressFor(xp, minLevel int) Progress {
	level := max(r.LevelFor(xp), minLevel, 1)
	start := r.LevelXP(level)
	if r.Settings.Limit && level >= r.Settings.MaxLevel {
		return Progress{Level: level, Into: max(xp-start, 0), Percent: 100}
	}
	cost := r.LevelXP(level+1) - start
	into := max(xp-start, 0)
	return Progress{Level: level, Into: into, Cost: cost, Missing: max(cost-into, 0), Percent: min(into*100/max(cost, 1), 100)}
}

// BadgeByKey finds a badge.
func (r *Rules) BadgeByKey(key string) (Badge, bool) {
	for _, b := range r.Badges {
		if b.Key == key {
			return b, true
		}
	}
	return Badge{}, false
}

// CategoryByKey finds a category.
func (r *Rules) CategoryByKey(key string) (Category, bool) {
	for _, c := range r.Categories {
		if c.Key == key {
			return c, true
		}
	}
	return Category{}, false
}

// ActiveBadges counts the badges that can still be earned.
func (r *Rules) ActiveBadges() int {
	n := 0
	for _, b := range r.Badges {
		if b.Active {
			n++
		}
	}
	return n
}

// NewlyReached returns the active rules that are fulfilled but not unlocked yet.
func (r *Rules) NewlyReached(f *Facts, unlocked map[string]bool) []Badge {
	var out []Badge
	for _, b := range r.Badges {
		if b.Active && !unlocked[b.Key] && b.Reached(f) {
			out = append(out, b)
		}
	}
	return out
}

// LevelFor returns the level reached with the given total XP under the default rules.
func LevelFor(xp int) int { return defaults.LevelFor(xp) }

// ProgressFor computes level progress under the default rules.
func ProgressFor(xp, minLevel int) Progress { return defaults.ProgressFor(xp, minLevel) }

// Title returns the title of a level under the default rules.
func Title(level int) string { return defaults.Title(level) }

// Package model holds the plain data types shared by all layers.
package model

import (
	"fmt"
	"time"
)

// Day is a civil calendar day, independent of time zone and daylight saving.
// It counts days since 1970-01-01, so plain integer arithmetic is safe.
type Day int

// NewDay builds a Day from a calendar date.
func NewDay(y int, m time.Month, d int) Day {
	return Day(time.Date(y, m, d, 12, 0, 0, 0, time.UTC).Unix() / 86400)
}

// DayOf returns the calendar day of t in the given location.
func DayOf(t time.Time, loc *time.Location) Day {
	y, m, d := t.In(loc).Date()
	return NewDay(y, m, d)
}

// ParseDay parses a YYYY-MM-DD string.
func ParseDay(s string) (Day, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return 0, fmt.Errorf("parse day %q: %w", s, err)
	}
	y, m, d := t.Date()
	return NewDay(y, m, d), nil
}

// Time returns noon UTC of the day, for calendar lookups only.
func (d Day) Time() time.Time { return time.Unix(int64(d)*86400+43200, 0).UTC() }

// String formats the day as YYYY-MM-DD.
func (d Day) String() string { return d.Time().Format("2006-01-02") }

// WeekdayIndex returns 0 for Monday up to 6 for Sunday.
func (d Day) WeekdayIndex() int { return (int(d.Time().Weekday()) + 6) % 7 }

// WeekStart returns the Monday of the day's week.
func (d Day) WeekStart() Day { return d - Day(d.WeekdayIndex()) }

// Year, Month and Dom expose the calendar date.
func (d Day) Year() int         { return d.Time().Year() }
func (d Day) Month() time.Month { return d.Time().Month() }
func (d Day) Dom() int          { return d.Time().Day() }

// Habit kinds.
const (
	KindBuild = "build"
	KindAvoid = "avoid"
)

// Habit frequencies.
const (
	FreqDaily   = "daily"
	FreqWeekly  = "weekly"
	FreqCustom  = "custom"
	FreqMonthly = "monthly"
)

// Difficulties.
const (
	Easy   = "easy"
	Medium = "medium"
	Hard   = "hard"
)

// How a habit is recorded: ticked once, counted in clicks or added up as an amount.
const (
	MeasureCheck  = "check"
	MeasureCount  = "count"
	MeasureAmount = "amount"
)

// Reward modes.
const (
	RewardCoins  = "coins"
	RewardStreak = "streak"
)

// Ledger kinds.
const (
	LedgerXP    = "xp"
	LedgerCoins = "coins"
	LedgerJoker = "joker"
)

// Area is a life area ("Bereich") a habit belongs to.
type Area struct {
	ID    int64  `json:"id"`
	Key   string `json:"key"` // stable slug of the built-in areas, used by badge rules
	Name  string `json:"name"`
	Color string `json:"color"`
	Icon  string `json:"icon"`
	Sort  int    `json:"sort"`
}

// Habit is a tracked behaviour.
type Habit struct {
	ID           int64     `json:"id"`
	TemplateID   string    `json:"template_id,omitempty"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"`
	Frequency    string    `json:"frequency"`
	WeeklyTarget int       `json:"weekly_target"` // check-ins per period, for FreqWeekly and FreqMonthly
	DaysMask     int       `json:"days_mask"`     // for FreqCustom, bit 0 = Monday
	Icon         string    `json:"icon"`
	Tag          string    `json:"tag"`
	AreaID       int64     `json:"area_id"`
	Difficulty   string    `json:"difficulty"`
	Archived     bool      `json:"archived"`
	CreatedAt    time.Time `json:"created_at"`
	Sort         int       `json:"sort"`
	Measure      string    `json:"measure"`
	Target       int       `json:"target"` // daily goal of counter and quantity habits
	Step         int       `json:"step"`   // amount of one click
	Unit         string    `json:"unit"`
}

// Measured reports whether the habit is filled up during the day.
func (h Habit) Measured() bool { return h.Measure == MeasureCount || h.Measure == MeasureAmount }

// Normalized fills in the plain tick-off defaults, also for older exports.
func (h Habit) Normalized() Habit {
	if !h.Measured() {
		h.Measure, h.Target, h.Step, h.Unit = MeasureCheck, 1, 1, ""
	}
	return h
}

// Entry is one click on a counter or quantity habit.
type Entry struct {
	ID        int64
	HabitID   int64
	Date      Day
	Amount    int
	CreatedAt time.Time
}

// Checkin marks a build habit as done on a day. UsedJoker rows only bridge a
// missed day: they keep the streak alive but do not count as done.
type Checkin struct {
	HabitID   int64     `json:"habit_id"`
	Date      Day       `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UsedJoker bool      `json:"used_joker"`
}

// Relapse marks a day an avoid habit was broken.
type Relapse struct {
	HabitID int64 `json:"habit_id"`
	Date    Day   `json:"-"`
}

// Reward is something to treat yourself with.
type Reward struct {
	ID             int64  `json:"id"`
	Title          string `json:"title"`
	Mode           string `json:"mode"`
	Price          int    `json:"price"`
	HabitID        int64  `json:"habit_id"`
	StreakTarget   int    `json:"streak_target"`
	ResetOnRelapse bool   `json:"reset_on_relapse"`
	Repeatable     bool   `json:"repeatable"`
	Archived       bool   `json:"archived"`
}

// Redemption records a redeemed reward.
type Redemption struct {
	RewardID   int64     `json:"reward_id"`
	Title      string    `json:"title"`
	RedeemedAt time.Time `json:"redeemed_at"`
	PricePaid  int       `json:"price_paid"`
}

// LedgerEntry is one credit or debit; balances are always sums over the ledger.
type LedgerEntry struct {
	ID     int64     `json:"id"`
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	Amount int       `json:"amount"`
	Reason string    `json:"reason"`
	Ref    string    `json:"ref"`
}

// Balances are the ledger totals.
type Balances struct {
	XP          int // net XP
	Coins       int // spendable Taler
	Jokers      int
	CoinsEarned int // Taler earned, spending excluded
	CoinsToday  int // Taler earned today
}

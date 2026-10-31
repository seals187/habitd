package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/seals187/habitd/internal/db"
	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

// Target numbers of the design mockups.
const (
	seedXP          = 8940
	seedCoins       = 340
	seedCoinsEarned = 2115
	seedDailyCoins  = 45
	seedJokers      = 2
	seedInstalled   = 212 // days since first start
	seedReadSpan    = 246 // days the reading habit exists: 212 done is 86 percent
)

// ErrNotEmpty is returned when seeding a database that has habits.
var ErrNotEmpty = errors.New("database already contains habits")

// Seed fills an empty database with demo data matching the design mockups.
func (s *Service) Seed(ctx context.Context) error {
	return s.Store.Tx(ctx, func(q *db.Q) error {
		existing, err := q.Habits()
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			return ErrNotEmpty
		}
		return s.seed(q)
	})
}

type seedHabit struct {
	habit model.Habit
	age   int
}

func (s *Service) seed(q *db.Q) error {
	today := s.Today()
	areas, err := q.Areas()
	if err != nil {
		return err
	}
	area := map[string]int64{}
	for _, a := range areas {
		area[a.Key] = a.ID
	}
	todayBit := 1 << today.WeekdayIndex()
	ids := map[string]int64{}
	for _, h := range []seedHabit{
		{model.Habit{Name: "20 Min. lesen", Kind: model.KindBuild, TemplateID: "20-min-lesen", Frequency: model.FreqDaily, Icon: "book", Tag: "lesen", AreaID: area["lernen"], Difficulty: model.Medium}, seedReadSpan - 1},
		{model.Habit{Name: "Sport", Kind: model.KindBuild, Frequency: model.FreqWeekly, WeeklyTarget: 3, Icon: "dumb", Tag: "sport", AreaID: area["gesundheit"], Difficulty: model.Hard}, 42},
		{model.Habit{Name: "Mama anrufen", TemplateID: "mama-anrufen", Kind: model.KindBuild, Frequency: model.FreqCustom, DaysMask: todayBit, Icon: "phone", Tag: "anrufen", AreaID: area["familie"], Difficulty: model.Easy}, 35},
		{model.Habit{Name: "Neue Idee einbringen", Kind: model.KindBuild, Frequency: model.FreqCustom, DaysMask: todayBit, Icon: "bulb", Tag: "idee", AreaID: area["arbeit"], Difficulty: model.Medium}, 21},
		{model.Habit{Name: "Kein Alkohol", Kind: model.KindAvoid, Frequency: model.FreqDaily, Icon: "ban", AreaID: area["gesundheit"], Difficulty: model.Easy}, 40},
	} {
		h.habit.CreatedAt = s.dayAt(today-model.Day(h.age), 9, 0)
		id, err := q.SaveHabit(h.habit)
		if err != nil {
			return err
		}
		ids[h.habit.Name] = id
	}

	missed, joker := readingGaps(today)
	for d := today - seedReadSpan + 1; d <= today; d++ {
		if missed[d] {
			continue
		}
		if err := s.seedCheckin(q, ids["20 Min. lesen"], d); err != nil {
			return err
		}
	}
	if err := q.Checkin(model.Checkin{HabitID: ids["20 Min. lesen"], Date: joker, CreatedAt: s.dayAt(joker+1, 8, 0), UsedJoker: true}); err != nil {
		return err
	}
	thisWeek := today.WeekStart()
	for w := thisWeek - 35; w < thisWeek; w += 7 {
		for _, offset := range []model.Day{0, 2, 4} {
			if err := s.seedCheckin(q, ids["Sport"], w+offset); err != nil {
				return err
			}
		}
	}
	for offset := model.Day(0); thisWeek+offset < today; offset += 2 {
		if err := s.seedCheckin(q, ids["Sport"], thisWeek+offset); err != nil {
			return err
		}
	}
	for w := model.Day(0); w <= 5; w++ {
		if err := s.seedCheckin(q, ids["Mama anrufen"], today-w*7); err != nil {
			return err
		}
	}
	for w := model.Day(1); w <= 3; w++ {
		if err := s.seedCheckin(q, ids["Neue Idee einbringen"], today-w*7); err != nil {
			return err
		}
	}

	if err := s.seedRewards(q, ids["20 Min. lesen"]); err != nil {
		return err
	}
	if err := s.seedLedger(q, today); err != nil {
		return err
	}
	for key, value := range map[string]string{
		KeyInstalledOn:  (today - seedInstalled).String(),
		KeyLastSettled:  (today - 1).String(),
		KeyLevel:        fmt.Sprint(s.Rules().LevelFor(seedXP)),
		KeyReminderTime: "20:00",
		KeyReminderOn:   "1",
		KeyOnboarded:    "1",
	} {
		if err := q.SetSetting(key, value); err != nil {
			return err
		}
	}
	return s.evaluate(q, nil, true)
}

func (s *Service) dayAt(d model.Day, hour, minute int) time.Time {
	return time.Date(d.Year(), d.Month(), d.Dom(), hour, minute, 0, 0, s.Loc)
}

func (s *Service) seedCheckin(q *db.Q, habitID int64, d model.Day) error {
	return q.Checkin(model.Checkin{HabitID: habitID, Date: d, CreatedAt: s.dayAt(d, 19, 30)})
}

// readingGaps builds the missed days of the reading habit: the streaks of the
// mockup (27 current, 41 record, 19 before) stay intact, and the remaining
// gaps fall on weekends, so Tuesday is the best and Saturday the worst day.
func readingGaps(today model.Day) (map[model.Day]bool, model.Day) {
	first := today - seedReadSpan + 1
	missed := map[model.Day]bool{}
	for _, i := range []model.Day{119, 139, 149, 191, 209, 218} {
		missed[first+i] = true
	}
	joker := first + 202
	missed[joker] = true
	regions := [][2]model.Day{{0, 118}, {140, 148}, {192, 208}}
	for _, q := range []struct{ weekday, count int }{{5, 11}, {6, 7}, {0, 3}, {2, 3}, {4, 2}, {3, 1}} {
		var days []model.Day
		for _, r := range regions {
			for i := r[0]; i <= r[1]; i++ {
				if (first+i).WeekdayIndex() == q.weekday && !missed[first+i] {
					days = append(days, first+i)
				}
			}
		}
		for n := range q.count {
			missed[days[n*len(days)/q.count]] = true
		}
	}
	return missed, joker
}

func (s *Service) seedRewards(q *db.Q, readingID int64) error {
	titles := []struct {
		title string
		icon  string
		price int
		count int
	}{
		{"Serien-Folge schauen", "eye", 50, 6},
		{"Lieblingskaffee", "cup", 80, 6},
		{"Essen bestellen", "meal", 200, 3},
		{"Ausschlafen am Sa", "moon", 150, 2},
		{"Neues Videospiel", "trophy", 900, 0},
	}
	today := s.Today()
	back := model.Day(5)
	for _, t := range titles {
		if err := q.SaveReward(model.Reward{Title: t.title, Icon: t.icon, Mode: model.RewardCoins, Price: t.price, Repeatable: true}); err != nil {
			return err
		}
		saved, err := q.Rewards()
		if err != nil {
			return err
		}
		for range t.count {
			if _, err := q.AddRedemption(saved[len(saved)-1].ID, s.dayAt(today-back, 18, 0), t.price, ""); err != nil {
				return err
			}
			back += 11
		}
	}
	return q.SaveReward(model.Reward{Title: "Neues Buch", Icon: "book", Mode: model.RewardStreak, HabitID: readingID, StreakTarget: 30, ResetOnRelapse: true, Repeatable: true})
}

// seedLedger books the balances of the mockup. Check-ins are not replayed one
// by one: per-habit totals carry the habit statistics, one opening entry each
// makes the sums exact, and today's check-ins stay undoable.
func (s *Service) seedLedger(q *db.Q, today model.Day) error {
	w, err := s.World(q)
	if err != nil {
		return err
	}
	old := s.dayAt(today-20, 12, 0)
	xpSum, coinSum := 0, 0
	book := func(prefix string, h *game.HabitData, xp int) error {
		ref := fmt.Sprintf("%s:%d:seed", prefix, h.ID)
		if err := q.AddLedger(old, model.LedgerXP, xp, "seed", ref); err != nil {
			return err
		}
		xpSum += xp
		coinSum += game.CoinsFor(xp)
		return q.AddLedger(old, model.LedgerCoins, game.CoinsFor(xp), "seed", ref)
	}
	for _, h := range w.Habits {
		if h.Kind == model.KindAvoid {
			xp := 0
			for d := h.Start; d < today; d++ {
				xp += game.AwardXP(h.Habit, h.StreakAt(d+1).Days)
			}
			if err := book("avoid", h, xp); err != nil {
				return err
			}
			continue
		}
		xp, fromToday := 0, 0
		for d := range h.Done {
			award := game.AwardXP(h.Habit, h.StreakAt(d).Days)
			if d == today {
				fromToday = award
				continue
			}
			xp += award
		}
		if err := book("checkin", h, xp); err != nil {
			return err
		}
		if fromToday > 0 {
			now := s.Now()
			ref := checkinRef(h.ID, today)
			if err := q.AddLedger(now, model.LedgerXP, fromToday, "checkin", ref); err != nil {
				return err
			}
			if err := q.AddLedger(now, model.LedgerCoins, game.CoinsFor(fromToday), "checkin", ref); err != nil {
				return err
			}
			xpSum += fromToday
			coinSum += game.CoinsFor(fromToday)
			// Real check-ins count towards the demo income of today.
			if err := q.AddLedger(now, model.LedgerCoins, -game.CoinsFor(fromToday), "seed", "seed:today"); err != nil {
				return err
			}
			coinSum -= game.CoinsFor(fromToday)
		}
	}
	if err := q.AddLedger(old, model.LedgerXP, seedXP-xpSum, "seed", "seed:opening"); err != nil {
		return err
	}
	for d := today - 13; d <= today; d++ {
		if err := q.AddLedger(s.dayAt(d, 6, 0), model.LedgerCoins, seedDailyCoins, "seed", "seed:income"); err != nil {
			return err
		}
		coinSum += seedDailyCoins
	}
	if err := q.AddLedger(old, model.LedgerCoins, seedCoinsEarned-coinSum, "seed", "seed:opening"); err != nil {
		return err
	}
	if err := q.AddLedger(old, model.LedgerCoins, -(seedCoinsEarned - seedCoins), "redeem", "seed:spent"); err != nil {
		return err
	}
	return q.AddLedger(old, model.LedgerJoker, seedJokers, "seed", "seed:opening")
}

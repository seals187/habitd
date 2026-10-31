package db

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("not found")

const timeLayout = time.RFC3339

func formatTime(t time.Time) string { return t.Format(timeLayout) }

// parseTime keeps the stored offset, so wall-clock hours stay meaningful.
func parseTime(s string) time.Time {
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Setting returns a setting of the current user or the fallback.
func (q *Q) Setting(key, fallback string) (string, error) {
	var v string
	err := q.r.QueryRowContext(q.ctx, `SELECT value FROM user_settings WHERE user_id = ? AND key = ?`, q.uid, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	if err != nil {
		return "", fmt.Errorf("get setting %s: %w", key, err)
	}
	return v, nil
}

// SettingInt returns an integer setting or the fallback.
func (q *Q) SettingInt(key string, fallback int) (int, error) {
	v, err := q.Setting(key, "")
	if err != nil || v == "" {
		return fallback, err
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback, nil //nolint:nilerr // a damaged value behaves like an unset one
	}
	return n, nil
}

// SetSetting stores a setting of the current user.
func (q *Q) SetSetting(key, value string) error {
	return q.exec(`INSERT INTO user_settings (user_id, key, value) VALUES (?, ?, ?)
		ON CONFLICT (user_id, key) DO UPDATE SET value = excluded.value`, q.uid, key, value)
}

// GlobalSetting returns an instance-wide setting or the fallback.
func (q *Q) GlobalSetting(key, fallback string) (string, error) {
	var v string
	err := q.r.QueryRowContext(q.ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	if err != nil {
		return "", fmt.Errorf("get global setting %s: %w", key, err)
	}
	return v, nil
}

// SetGlobalSetting stores an instance-wide setting.
func (q *Q) SetGlobalSetting(key, value string) error {
	return q.exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
}

// DeleteGlobalSetting removes an instance-wide setting.
func (q *Q) DeleteGlobalSetting(key string) error {
	return q.exec(`DELETE FROM settings WHERE key = ?`, key)
}

// Areas lists all areas in display order.
func (q *Q) Areas() ([]model.Area, error) {
	rows, err := q.r.QueryContext(q.ctx, `SELECT id, key, name, color, icon, sort FROM areas WHERE user_id = ? ORDER BY sort, id`, q.uid)
	if err != nil {
		return nil, fmt.Errorf("list areas: %w", err)
	}
	defer rows.Close()
	var out []model.Area
	for rows.Next() {
		var a model.Area
		if err := rows.Scan(&a.ID, &a.Key, &a.Name, &a.Color, &a.Icon, &a.Sort); err != nil {
			return nil, fmt.Errorf("scan area: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SaveArea creates or updates an area.
func (q *Q) SaveArea(a model.Area) error {
	if a.ID == 0 {
		return q.exec(`INSERT INTO areas (user_id, name, color, icon, sort)
			VALUES (?, ?, ?, ?, (SELECT coalesce(max(sort), 0) + 1 FROM areas WHERE user_id = ?))`, q.uid, a.Name, a.Color, a.Icon, q.uid)
	}
	return q.execOne(`UPDATE areas SET name = ?, color = ?, icon = ? WHERE id = ? AND user_id = ?`, a.Name, a.Color, a.Icon, a.ID, q.uid)
}

// DeleteArea removes an area without habits.
func (q *Q) DeleteArea(id int64) error {
	var n int
	if err := q.r.QueryRowContext(q.ctx, `SELECT count(*) FROM habits WHERE area_id = ? AND user_id = ?`, id, q.uid).Scan(&n); err != nil {
		return fmt.Errorf("count area habits: %w", err)
	}
	if n > 0 {
		return fmt.Errorf("area %d still has habits", id)
	}
	return q.execOne(`DELETE FROM areas WHERE id = ? AND user_id = ?`, id, q.uid)
}

const habitColumns = `id, template_id, name, kind, frequency, weekly_target, days_mask, icon, tag, area_id, difficulty, archived, created_at, sort, measure, target, step, unit`

func scanHabit(s interface{ Scan(...any) error }) (model.Habit, error) {
	var h model.Habit
	var created string
	var template sql.NullString
	err := s.Scan(&h.ID, &template, &h.Name, &h.Kind, &h.Frequency, &h.WeeklyTarget, &h.DaysMask, &h.Icon, &h.Tag,
		&h.AreaID, &h.Difficulty, &h.Archived, &created, &h.Sort, &h.Measure, &h.Target, &h.Step, &h.Unit)
	h.CreatedAt = parseTime(created)
	h.TemplateID = template.String
	return h, err
}

// Habit returns one habit.
func (q *Q) Habit(id int64) (model.Habit, error) {
	row := q.r.QueryRowContext(q.ctx, `SELECT `+habitColumns+` FROM habits WHERE id = ? AND user_id = ?`, id, q.uid)
	h, err := scanHabit(row)
	if errors.Is(err, sql.ErrNoRows) {
		return h, ErrNotFound
	}
	if err != nil {
		return h, fmt.Errorf("get habit: %w", err)
	}
	return h, nil
}

// Habits lists all habits in display order, archived ones included.
func (q *Q) Habits() ([]model.Habit, error) {
	rows, err := q.r.QueryContext(q.ctx, `SELECT `+habitColumns+` FROM habits WHERE user_id = ? ORDER BY sort, id`, q.uid)
	if err != nil {
		return nil, fmt.Errorf("list habits: %w", err)
	}
	defer rows.Close()
	var out []model.Habit
	for rows.Next() {
		h, err := scanHabit(rows)
		if err != nil {
			return nil, fmt.Errorf("scan habit: %w", err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// SaveHabit creates or updates a habit and returns its id.
func (q *Q) SaveHabit(h model.Habit) (int64, error) {
	h = h.Normalized()
	if h.ID != 0 {
		err := q.exec(`UPDATE habits SET template_id = ?, name = ?, kind = ?, frequency = ?, weekly_target = ?, days_mask = ?, icon = ?,
			tag = ?, area_id = ?, difficulty = ?, archived = ?, measure = ?, target = ?, step = ?, unit = ? WHERE id = ? AND user_id = ?`,
			nullableText(h.TemplateID), h.Name, h.Kind, h.Frequency, h.WeeklyTarget, h.DaysMask, h.Icon, h.Tag, h.AreaID, h.Difficulty, b2i(h.Archived),
			h.Measure, h.Target, h.Step, h.Unit, h.ID, q.uid)
		return h.ID, err
	}
	res, err := q.r.ExecContext(q.ctx, `INSERT INTO habits
		(user_id, template_id, name, kind, frequency, weekly_target, days_mask, icon, tag, area_id, difficulty, archived, created_at, measure, target, step, unit, sort)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, (SELECT coalesce(max(sort), 0) + 1 FROM habits WHERE user_id = ?))`,
		q.uid, nullableText(h.TemplateID), h.Name, h.Kind, h.Frequency, h.WeeklyTarget, h.DaysMask, h.Icon, h.Tag, h.AreaID, h.Difficulty, b2i(h.Archived), formatTime(h.CreatedAt),
		h.Measure, h.Target, h.Step, h.Unit, q.uid)
	if err != nil {
		return 0, fmt.Errorf("insert habit: %w", err)
	}
	return res.LastInsertId()
}

// SetHabitArchived archives or restores a habit.
func (q *Q) SetHabitArchived(id int64, archived bool) error {
	return q.execOne(`UPDATE habits SET archived = ? WHERE id = ? AND user_id = ?`, b2i(archived), id, q.uid)
}

// MoveHabit swaps the habit with its neighbour among the active habits.
func (q *Q) MoveHabit(id int64, up bool) error {
	if _, err := q.Habit(id); err != nil {
		return err
	}
	habits, err := q.Habits()
	if err != nil {
		return err
	}
	var active []model.Habit
	for _, h := range habits {
		if !h.Archived {
			active = append(active, h)
		}
	}
	for i, h := range active {
		if h.ID != id {
			continue
		}
		j := i + 1
		if up {
			j = i - 1
		}
		if j < 0 || j >= len(active) {
			return nil
		}
		active[i], active[j] = active[j], active[i]
		break
	}
	for i, h := range active {
		if err := q.exec(`UPDATE habits SET sort = ? WHERE id = ? AND user_id = ?`, i+1, h.ID, q.uid); err != nil {
			return err
		}
	}
	return nil
}

// Checkin inserts or replaces a check-in.
func (q *Q) Checkin(c model.Checkin) error {
	return q.exec(`INSERT INTO checkins (user_id, habit_id, date, created_at, used_joker) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (habit_id, date) DO UPDATE SET created_at = excluded.created_at, used_joker = excluded.used_joker`,
		q.uid, c.HabitID, c.Date.String(), formatTime(c.CreatedAt), b2i(c.UsedJoker))
}

// DeleteCheckin removes a check-in.
func (q *Q) DeleteCheckin(habitID int64, d model.Day) error {
	return q.exec(`DELETE FROM checkins WHERE habit_id = ? AND date = ? AND user_id = ?`, habitID, d.String(), q.uid)
}

// DeleteHabit removes a habit with its history. The ledger stays: earned XP and
// Taler are kept. Streak rewards of the habit could never unlock again.
func (q *Q) DeleteHabit(id int64) error {
	if err := q.exec(`UPDATE rewards SET archived = 1 WHERE habit_id = ? AND mode = ? AND user_id = ?`, id, model.RewardStreak, q.uid); err != nil {
		return err
	}
	return q.exec(`DELETE FROM habits WHERE id = ? AND user_id = ?`, id, q.uid)
}

// ResetProgress removes habits and everything earned. Unlike DeleteHabit it also
// clears the ledger. Areas, settings and coin rewards stay.
func (q *Q) ResetProgress() error {
	for _, stmt := range []string{
		`DELETE FROM celebrations WHERE user_id = ?`, `DELETE FROM badges_unlocked WHERE user_id = ?`,
		`DELETE FROM ledger WHERE user_id = ?`, `DELETE FROM redemptions WHERE user_id = ?`,
		`UPDATE rewards SET archived = 1 WHERE user_id = ? AND mode = '` + model.RewardStreak + `'`, `DELETE FROM habits WHERE user_id = ?`,
	} {
		if err := q.exec(stmt, q.uid); err != nil {
			return err
		}
	}
	return nil
}

// AddEntry records one click.
func (q *Q) AddEntry(e model.Entry) error {
	return q.exec(`INSERT INTO habit_entries (user_id, habit_id, date, amount, created_at) VALUES (?, ?, ?, ?, ?)`,
		q.uid, e.HabitID, e.Date.String(), e.Amount, formatTime(e.CreatedAt))
}

// DeleteLastEntry removes the newest entry of a day. It returns ErrNotFound if
// there is none.
func (q *Q) DeleteLastEntry(habitID int64, d model.Day) error {
	var id int64
	err := q.r.QueryRowContext(q.ctx, `SELECT id FROM habit_entries WHERE habit_id = ? AND date = ? AND user_id = ? ORDER BY id DESC LIMIT 1`,
		habitID, d.String(), q.uid).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("last entry: %w", err)
	}
	return q.exec(`DELETE FROM habit_entries WHERE id = ?`, id)
}

// DeleteDayEntries removes all entries of a day.
func (q *Q) DeleteDayEntries(habitID int64, d model.Day) error {
	return q.exec(`DELETE FROM habit_entries WHERE habit_id = ? AND date = ? AND user_id = ?`, habitID, d.String(), q.uid)
}

// DayTotals sums the entries of a day per habit.
func (q *Q) DayTotals(d model.Day) (map[int64]int, error) {
	rows, err := q.r.QueryContext(q.ctx, `SELECT habit_id, sum(amount) FROM habit_entries WHERE date = ? AND user_id = ? GROUP BY habit_id`, d.String(), q.uid)
	if err != nil {
		return nil, fmt.Errorf("day totals: %w", err)
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var total int
		if err := rows.Scan(&id, &total); err != nil {
			return nil, fmt.Errorf("scan day total: %w", err)
		}
		out[id] = total
	}
	return out, rows.Err()
}

// AddRelapse records a relapse.
func (q *Q) AddRelapse(habitID int64, d model.Day) error {
	return q.exec(`INSERT OR IGNORE INTO relapses (user_id, habit_id, date) VALUES (?, ?, ?)`, q.uid, habitID, d.String())
}

// DeleteRelapse removes a relapse.
func (q *Q) DeleteRelapse(habitID int64, d model.Day) error {
	return q.exec(`DELETE FROM relapses WHERE habit_id = ? AND date = ? AND user_id = ?`, habitID, d.String(), q.uid)
}

// LoadHabits loads all habits with their full history.
func (q *Q) LoadHabits(loc *time.Location) ([]*game.HabitData, error) {
	areas, err := q.Areas()
	if err != nil {
		return nil, err
	}
	keys := map[int64]string{}
	for _, a := range areas {
		keys[a.ID] = a.Key
	}
	habits, err := q.Habits()
	if err != nil {
		return nil, err
	}
	out := make([]*game.HabitData, 0, len(habits))
	byID := map[int64]*game.HabitData{}
	for _, h := range habits {
		d := game.NewHabitData(h, model.DayOf(h.CreatedAt, loc))
		d.AreaKey = keys[h.AreaID]
		out = append(out, d)
		byID[h.ID] = d
	}

	rows, err := q.r.QueryContext(q.ctx, `SELECT habit_id, date, created_at, used_joker FROM checkins WHERE user_id = ?`, q.uid)
	if err != nil {
		return nil, fmt.Errorf("list checkins: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var date, created string
		var joker bool
		if err := rows.Scan(&id, &date, &created, &joker); err != nil {
			return nil, fmt.Errorf("scan checkin: %w", err)
		}
		h, day := byID[id], model.Day(0)
		if day, err = model.ParseDay(date); err != nil {
			return nil, err
		}
		if h == nil {
			continue
		}
		if joker {
			h.Joker[day] = true
		} else {
			h.Done[day] = parseTime(created)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	amounts, err := q.r.QueryContext(q.ctx, `SELECT habit_id, sum(amount) FROM habit_entries WHERE user_id = ? GROUP BY habit_id`, q.uid)
	if err != nil {
		return nil, fmt.Errorf("sum entries: %w", err)
	}
	defer amounts.Close()
	for amounts.Next() {
		var id int64
		var sum int
		if err := amounts.Scan(&id, &sum); err != nil {
			return nil, fmt.Errorf("scan entry sum: %w", err)
		}
		if h := byID[id]; h != nil {
			h.Amount = sum
		}
	}
	if err := amounts.Err(); err != nil {
		return nil, err
	}

	relapses, err := q.r.QueryContext(q.ctx, `SELECT habit_id, date FROM relapses WHERE user_id = ? ORDER BY date`, q.uid)
	if err != nil {
		return nil, fmt.Errorf("list relapses: %w", err)
	}
	defer relapses.Close()
	for relapses.Next() {
		var id int64
		var date string
		if err := relapses.Scan(&id, &date); err != nil {
			return nil, fmt.Errorf("scan relapse: %w", err)
		}
		day, err := model.ParseDay(date)
		if err != nil {
			return nil, err
		}
		if h := byID[id]; h != nil {
			h.Relapses = append(h.Relapses, day)
		}
	}
	return out, relapses.Err()
}

// LastJokerDay returns the latest day bridged by a joker, or 0.
func (q *Q) LastJokerDay() (model.Day, error) {
	var date sql.NullString
	if err := q.r.QueryRowContext(q.ctx, `SELECT max(date) FROM checkins WHERE used_joker = 1 AND user_id = ?`, q.uid).Scan(&date); err != nil {
		return 0, fmt.Errorf("last joker: %w", err)
	}
	if !date.Valid {
		return 0, nil
	}
	return model.ParseDay(date.String)
}

// Reward returns one reward.
func (q *Q) Reward(id int64) (model.Reward, error) {
	var r model.Reward
	var habit sql.NullInt64
	err := q.r.QueryRowContext(q.ctx, `SELECT id, title, mode, price, habit_id, streak_target, reset_on_relapse, repeatable, archived
		FROM rewards WHERE id = ? AND user_id = ?`, id, q.uid).Scan(&r.ID, &r.Title, &r.Mode, &r.Price, &habit, &r.StreakTarget, &r.ResetOnRelapse, &r.Repeatable, &r.Archived)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, fmt.Errorf("get reward: %w", err)
	}
	r.HabitID = habit.Int64
	return r, nil
}

// Rewards lists all rewards, archived ones included.
func (q *Q) Rewards() ([]model.Reward, error) {
	rows, err := q.r.QueryContext(q.ctx, `SELECT id, title, mode, price, habit_id, streak_target, reset_on_relapse, repeatable, archived
		FROM rewards WHERE user_id = ? ORDER BY id`, q.uid)
	if err != nil {
		return nil, fmt.Errorf("list rewards: %w", err)
	}
	defer rows.Close()
	var out []model.Reward
	for rows.Next() {
		var r model.Reward
		var habit sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Title, &r.Mode, &r.Price, &habit, &r.StreakTarget, &r.ResetOnRelapse, &r.Repeatable, &r.Archived); err != nil {
			return nil, fmt.Errorf("scan reward: %w", err)
		}
		r.HabitID = habit.Int64
		out = append(out, r)
	}
	return out, rows.Err()
}

func nullableText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullable(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// SaveReward creates or updates a reward.
func (q *Q) SaveReward(r model.Reward) error {
	if r.ID != 0 {
		return q.execOne(`UPDATE rewards SET title = ?, mode = ?, price = ?, habit_id = ?, streak_target = ?,
			reset_on_relapse = ?, repeatable = ?, archived = ? WHERE id = ? AND user_id = ?`,
			r.Title, r.Mode, r.Price, nullable(r.HabitID), r.StreakTarget, b2i(r.ResetOnRelapse), b2i(r.Repeatable), b2i(r.Archived), r.ID, q.uid)
	}
	return q.exec(`INSERT INTO rewards (user_id, title, mode, price, habit_id, streak_target, reset_on_relapse, repeatable, archived)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		q.uid, r.Title, r.Mode, r.Price, nullable(r.HabitID), r.StreakTarget, b2i(r.ResetOnRelapse), b2i(r.Repeatable), b2i(r.Archived))
}

// AddRedemption records a redeemed reward.
func (q *Q) AddRedemption(rewardID int64, at time.Time, price int) error {
	return q.exec(`INSERT INTO redemptions (user_id, reward_id, redeemed_at, price_paid) VALUES (?, ?, ?, ?)`, q.uid, rewardID, formatTime(at), price)
}

// Redemptions lists redemptions, newest first.
func (q *Q) Redemptions() ([]model.Redemption, error) {
	rows, err := q.r.QueryContext(q.ctx, `SELECT d.reward_id, r.title, d.redeemed_at, d.price_paid
		FROM redemptions d JOIN rewards r ON r.id = d.reward_id WHERE d.user_id = ? ORDER BY d.redeemed_at DESC, d.id DESC`, q.uid)
	if err != nil {
		return nil, fmt.Errorf("list redemptions: %w", err)
	}
	defer rows.Close()
	var out []model.Redemption
	for rows.Next() {
		var r model.Redemption
		var at string
		if err := rows.Scan(&r.RewardID, &r.Title, &at, &r.PricePaid); err != nil {
			return nil, fmt.Errorf("scan redemption: %w", err)
		}
		r.RedeemedAt = parseTime(at)
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddLedger appends an entry. Time is stored with its local offset, so the
// first ten characters are the local calendar day.
func (q *Q) AddLedger(at time.Time, kind string, amount int, reason, ref string) error {
	return q.exec(`INSERT INTO ledger (user_id, at, kind, amount, reason, ref) VALUES (?, ?, ?, ?, ?, ?)`,
		q.uid, formatTime(at), kind, amount, reason, ref)
}

// LedgerSum sums the entries of a ref per kind.
func (q *Q) LedgerSum(ref string) (map[string]int, error) {
	rows, err := q.r.QueryContext(q.ctx, `SELECT kind, sum(amount) FROM ledger WHERE ref = ? AND user_id = ? GROUP BY kind`, ref, q.uid)
	if err != nil {
		return nil, fmt.Errorf("sum ledger: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var kind string
		var sum int
		if err := rows.Scan(&kind, &sum); err != nil {
			return nil, fmt.Errorf("scan ledger sum: %w", err)
		}
		out[kind] = sum
	}
	return out, rows.Err()
}

// LedgerHas reports whether any entry with this ref exists.
func (q *Q) LedgerHas(ref string) (bool, error) {
	var n int
	err := q.r.QueryRowContext(q.ctx, `SELECT count(*) FROM ledger WHERE ref = ? AND user_id = ?`, ref, q.uid).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check ledger: %w", err)
	}
	return n > 0, nil
}

// Spending reasons do not count as earned Taler.
const earnedFilter = `kind = 'coins' AND reason NOT IN ('redeem', 'joker_buy')`

// Balances computes all balances from the ledger.
func (q *Q) Balances(today model.Day) (model.Balances, error) {
	var b model.Balances
	err := q.r.QueryRowContext(q.ctx, `SELECT
		coalesce(sum(CASE WHEN kind = 'xp' THEN amount END), 0),
		coalesce(sum(CASE WHEN kind = 'coins' THEN amount END), 0),
		coalesce(sum(CASE WHEN kind = 'joker' THEN amount END), 0),
		coalesce(sum(CASE WHEN `+earnedFilter+` THEN amount END), 0),
		coalesce(sum(CASE WHEN `+earnedFilter+` AND substr(at, 1, 10) = ? THEN amount END), 0)
		FROM ledger WHERE user_id = ?`, today.String(), q.uid).Scan(&b.XP, &b.Coins, &b.Jokers, &b.CoinsEarned, &b.CoinsToday)
	if err != nil {
		return b, fmt.Errorf("balances: %w", err)
	}
	return b, nil
}

// AverageDailyCoins is the average of Taler earned per day in the range.
func (q *Q) AverageDailyCoins(from, to model.Day) (int, error) {
	var sum int
	err := q.r.QueryRowContext(q.ctx, `SELECT coalesce(sum(amount), 0) FROM ledger WHERE user_id = ? AND `+earnedFilter+`
		AND substr(at, 1, 10) >= ? AND substr(at, 1, 10) <= ?`, q.uid, from.String(), to.String()).Scan(&sum)
	if err != nil {
		return 0, fmt.Errorf("average coins: %w", err)
	}
	return sum / max(int(to-from)+1, 1), nil
}

// HabitEarnings sums the XP and Taler a habit has brought in.
func (q *Q) HabitEarnings(habitID int64) (xp, coins int, err error) {
	like := func(prefix string) string { return prefix + ":" + strconv.FormatInt(habitID, 10) + ":%" }
	err = q.r.QueryRowContext(q.ctx, `SELECT
		coalesce(sum(CASE WHEN kind = 'xp' THEN amount END), 0),
		coalesce(sum(CASE WHEN kind = 'coins' THEN amount END), 0)
		FROM ledger WHERE user_id = ? AND (ref LIKE ? OR ref LIKE ?)`, q.uid, like("checkin"), like("avoid")).Scan(&xp, &coins)
	if err != nil {
		return 0, 0, fmt.Errorf("habit earnings: %w", err)
	}
	return xp, coins, nil
}

// UnlockedBadges returns unlock times by badge key.
func (q *Q) UnlockedBadges() (map[string]time.Time, error) {
	rows, err := q.r.QueryContext(q.ctx, `SELECT badge_key, unlocked_at FROM badges_unlocked WHERE user_id = ?`, q.uid)
	if err != nil {
		return nil, fmt.Errorf("list badges: %w", err)
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var key, at string
		if err := rows.Scan(&key, &at); err != nil {
			return nil, fmt.Errorf("scan badge: %w", err)
		}
		out[key] = parseTime(at)
	}
	return out, rows.Err()
}

// UnlockBadge stores an unlocked badge.
func (q *Q) UnlockBadge(key string, at time.Time) error {
	return q.exec(`INSERT OR IGNORE INTO badges_unlocked (user_id, badge_key, unlocked_at) VALUES (?, ?, ?)`, q.uid, key, formatTime(at))
}

// AddCelebration queues a celebration.
func (q *Q) AddCelebration(kind, payload string) error {
	return q.exec(`INSERT INTO celebrations (user_id, kind, payload) VALUES (?, ?, ?)`, q.uid, kind, payload)
}

// NextCelebration returns the oldest queued celebration; levels go first.
func (q *Q) NextCelebration() (id int64, kind, payload string, err error) {
	err = q.r.QueryRowContext(q.ctx, `SELECT id, kind, payload FROM celebrations WHERE user_id = ?
		ORDER BY kind = 'level' DESC, id LIMIT 1`, q.uid).Scan(&id, &kind, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", "", ErrNotFound
	}
	if err != nil {
		return 0, "", "", fmt.Errorf("next celebration: %w", err)
	}
	return id, kind, payload, nil
}

// HasCelebration reports whether a celebration is waiting.
func (q *Q) HasCelebration() (bool, error) {
	var n int
	if err := q.r.QueryRowContext(q.ctx, `SELECT count(*) FROM celebrations WHERE user_id = ?`, q.uid).Scan(&n); err != nil {
		return false, fmt.Errorf("count celebrations: %w", err)
	}
	return n > 0, nil
}

// DeleteCelebration removes a shown celebration.
func (q *Q) DeleteCelebration(id int64) error {
	return q.exec(`DELETE FROM celebrations WHERE id = ? AND user_id = ?`, id, q.uid)
}

// PushSubscription is a browser endpoint for reminders.
type PushSubscription struct {
	Endpoint string
	P256dh   string
	Auth     string
}

// SavePushSubscription stores a subscription.
func (q *Q) SavePushSubscription(s PushSubscription) error {
	return q.exec(`INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth) VALUES (?, ?, ?, ?)
		ON CONFLICT (endpoint) DO UPDATE SET user_id = excluded.user_id, p256dh = excluded.p256dh, auth = excluded.auth`, q.uid, s.Endpoint, s.P256dh, s.Auth)
}

// DeletePushSubscription removes a subscription.
func (q *Q) DeletePushSubscription(endpoint string) error {
	return q.exec(`DELETE FROM push_subscriptions WHERE endpoint = ? AND user_id = ?`, endpoint, q.uid)
}

// PushSubscriptions lists the subscriptions of the current user.
func (q *Q) PushSubscriptions() ([]PushSubscription, error) {
	rows, err := q.r.QueryContext(q.ctx, `SELECT endpoint, p256dh, auth FROM push_subscriptions WHERE user_id = ?`, q.uid)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	defer rows.Close()
	var out []PushSubscription
	for rows.Next() {
		var s PushSubscription
		if err := rows.Scan(&s.Endpoint, &s.P256dh, &s.Auth); err != nil {
			return nil, fmt.Errorf("scan subscription: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Export holds the data of one user. Password hash, sessions and push keys stay out.
type Export struct {
	Version     int                 `json:"version"`
	Settings    map[string]string   `json:"settings"`
	Areas       []model.Area        `json:"areas"`
	Habits      []model.Habit       `json:"habits"`
	Checkins    []ExportCheckin     `json:"checkins"`
	Entries     []ExportEntry       `json:"entries"`
	Relapses    []ExportRelapse     `json:"relapses"`
	Rewards     []model.Reward      `json:"rewards"`
	Redemptions []model.Redemption  `json:"redemptions"`
	Ledger      []model.LedgerEntry `json:"ledger"`
	Badges      map[string]string   `json:"badges"`
}

// ExportCheckin is the JSON form of a check-in.
type ExportCheckin struct {
	HabitID   int64  `json:"habit_id"`
	Date      string `json:"date"`
	CreatedAt string `json:"created_at"`
	UsedJoker bool   `json:"used_joker"`
}

// ExportEntry is the JSON form of a click on a counter or quantity habit.
type ExportEntry struct {
	HabitID   int64  `json:"habit_id"`
	Date      string `json:"date"`
	Amount    int    `json:"amount"`
	CreatedAt string `json:"created_at"`
}

// ExportRelapse is the JSON form of a relapse.
type ExportRelapse struct {
	HabitID int64  `json:"habit_id"`
	Date    string `json:"date"`
}

// privateSettings never leave the instance; old exports may still carry them.
var privateSettings = []string{"password_hash", "admin_user", "vapid_private", "vapid_public"}

func isPrivateSetting(key string) bool { return slices.Contains(privateSettings, key) }

// DumpAll reads all exportable data of the current user.
func (q *Q) DumpAll() (*Export, error) {
	e := &Export{Version: 1, Settings: map[string]string{}, Badges: map[string]string{}}
	var err error
	if e.Areas, err = q.Areas(); err != nil {
		return nil, err
	}
	if e.Habits, err = q.Habits(); err != nil {
		return nil, err
	}
	if e.Rewards, err = q.Rewards(); err != nil {
		return nil, err
	}
	if e.Redemptions, err = q.Redemptions(); err != nil {
		return nil, err
	}
	err = q.each(`SELECT key, value FROM user_settings WHERE user_id = ?`, func(s scanner) error {
		var k, v string
		if err := s.Scan(&k, &v); err != nil {
			return err
		}
		if !isPrivateSetting(k) {
			e.Settings[k] = v
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = q.each(`SELECT habit_id, date, created_at, used_joker FROM checkins WHERE user_id = ? ORDER BY date`, func(s scanner) error {
		var c ExportCheckin
		if err := s.Scan(&c.HabitID, &c.Date, &c.CreatedAt, &c.UsedJoker); err != nil {
			return err
		}
		e.Checkins = append(e.Checkins, c)
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = q.each(`SELECT habit_id, date, amount, created_at FROM habit_entries WHERE user_id = ? ORDER BY id`, func(s scanner) error {
		var x ExportEntry
		if err := s.Scan(&x.HabitID, &x.Date, &x.Amount, &x.CreatedAt); err != nil {
			return err
		}
		e.Entries = append(e.Entries, x)
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = q.each(`SELECT habit_id, date FROM relapses WHERE user_id = ? ORDER BY date`, func(s scanner) error {
		var r ExportRelapse
		if err := s.Scan(&r.HabitID, &r.Date); err != nil {
			return err
		}
		e.Relapses = append(e.Relapses, r)
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = q.each(`SELECT id, at, kind, amount, reason, ref FROM ledger WHERE user_id = ? ORDER BY id`, func(s scanner) error {
		var l model.LedgerEntry
		var at string
		if err := s.Scan(&l.ID, &at, &l.Kind, &l.Amount, &l.Reason, &l.Ref); err != nil {
			return err
		}
		l.At = parseTime(at)
		e.Ledger = append(e.Ledger, l)
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = q.each(`SELECT badge_key, unlocked_at FROM badges_unlocked WHERE user_id = ?`, func(s scanner) error {
		var k, v string
		if err := s.Scan(&k, &v); err != nil {
			return err
		}
		e.Badges[k] = v
		return nil
	})
	return e, err
}

type scanner interface{ Scan(...any) error }

// each runs a query with the current user as its only argument.
func (q *Q) each(query string, fn func(scanner) error) error {
	rows, err := q.r.QueryContext(q.ctx, query, q.uid)
	if err != nil {
		return fmt.Errorf("query %q: %w", firstLine(query), err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return fmt.Errorf("scan %q: %w", firstLine(query), err)
		}
	}
	return rows.Err()
}

// RestoreAll replaces the data of the current user with the export. Run it in a
// transaction. Rows get new ids, because the export may stem from another user
// of this instance; ledger references follow them.
func (q *Q) RestoreAll(e *Export) error {
	for _, table := range []string{"celebrations", "badges_unlocked", "ledger", "redemptions", "rewards", "relapses",
		"habit_entries", "checkins", "habits", "areas", "user_settings"} {
		if err := q.exec(`DELETE FROM `+table+` WHERE user_id = ?`, q.uid); err != nil {
			return err
		}
	}
	for k, v := range e.Settings {
		if isPrivateSetting(k) {
			continue
		}
		if err := q.SetSetting(k, v); err != nil {
			return err
		}
	}
	areas, habits, rewards := map[int64]int64{}, map[int64]int64{}, map[int64]int64{}
	for _, a := range e.Areas {
		res, err := q.r.ExecContext(q.ctx, `INSERT INTO areas (user_id, key, name, color, icon, sort) VALUES (?, ?, ?, ?, ?, ?)`,
			q.uid, a.Key, a.Name, a.Color, a.Icon, a.Sort)
		if err != nil {
			return fmt.Errorf("restore area: %w", err)
		}
		if areas[a.ID], err = res.LastInsertId(); err != nil {
			return err
		}
	}
	for _, h := range e.Habits {
		h = h.Normalized()
		res, err := q.r.ExecContext(q.ctx, `INSERT INTO habits (user_id, template_id, name, kind, frequency, weekly_target, days_mask,
			icon, tag, area_id, difficulty, archived, created_at, sort, measure, target, step, unit)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			q.uid, nullableText(h.TemplateID), h.Name, h.Kind, h.Frequency, h.WeeklyTarget, h.DaysMask, h.Icon, h.Tag, areas[h.AreaID], h.Difficulty,
			b2i(h.Archived), formatTime(h.CreatedAt), h.Sort, h.Measure, h.Target, h.Step, h.Unit)
		if err != nil {
			return fmt.Errorf("restore habit: %w", err)
		}
		if habits[h.ID], err = res.LastInsertId(); err != nil {
			return err
		}
	}
	for _, x := range e.Entries {
		if err := q.exec(`INSERT INTO habit_entries (user_id, habit_id, date, amount, created_at) VALUES (?, ?, ?, ?, ?)`,
			q.uid, habits[x.HabitID], x.Date, x.Amount, x.CreatedAt); err != nil {
			return err
		}
	}
	for _, c := range e.Checkins {
		if err := q.exec(`INSERT INTO checkins (user_id, habit_id, date, created_at, used_joker) VALUES (?, ?, ?, ?, ?)`,
			q.uid, habits[c.HabitID], c.Date, c.CreatedAt, b2i(c.UsedJoker)); err != nil {
			return err
		}
	}
	for _, r := range e.Relapses {
		if err := q.exec(`INSERT INTO relapses (user_id, habit_id, date) VALUES (?, ?, ?)`, q.uid, habits[r.HabitID], r.Date); err != nil {
			return err
		}
	}
	for _, r := range e.Rewards {
		res, err := q.r.ExecContext(q.ctx, `INSERT INTO rewards (user_id, title, mode, price, habit_id, streak_target, reset_on_relapse, repeatable, archived)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, q.uid, r.Title, r.Mode, r.Price, nullable(habits[r.HabitID]), r.StreakTarget,
			b2i(r.ResetOnRelapse), b2i(r.Repeatable), b2i(r.Archived))
		if err != nil {
			return fmt.Errorf("restore reward: %w", err)
		}
		if rewards[r.ID], err = res.LastInsertId(); err != nil {
			return err
		}
	}
	for _, r := range e.Redemptions {
		if err := q.AddRedemption(rewards[r.RewardID], r.RedeemedAt, r.PricePaid); err != nil {
			return err
		}
	}
	for _, l := range e.Ledger {
		if err := q.AddLedger(l.At, l.Kind, l.Amount, l.Reason, remapRef(l.Ref, habits, rewards)); err != nil {
			return err
		}
	}
	for k, at := range e.Badges {
		if err := q.exec(`INSERT INTO badges_unlocked (user_id, badge_key, unlocked_at) VALUES (?, ?, ?)`, q.uid, k, at); err != nil {
			return err
		}
	}
	return nil
}

// remapRef moves a ledger reference like checkin:5:2026-10-31 to the new id.
func remapRef(ref string, habits, rewards map[int64]int64) string {
	parts := strings.SplitN(ref, ":", 3)
	if len(parts) < 3 {
		return ref
	}
	ids := habits
	switch parts[0] {
	case "checkin", "avoid", "streak", "bridge":
	case "redeem":
		ids = rewards
	default:
		return ref
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return ref
	}
	if fresh, ok := ids[id]; ok {
		parts[1] = strconv.FormatInt(fresh, 10)
	}
	return strings.Join(parts, ":")
}

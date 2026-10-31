package db

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/seals187/habitd/internal/game"
	"github.com/seals187/habitd/internal/model"
)

const questColumns = `id, key, name, text, icon, color, kind, count, until_time, area, percent, reward_kind, reward,
	weight, days, min_habits, min_level, only_area, active, sort`

// SeedQuests stores the start values once.
func (q *Q) SeedQuests(quests []game.QuestDef, c game.QuestConfig) error {
	var n int
	if err := q.r.QueryRowContext(q.ctx, `SELECT count(*) FROM quest_settings`).Scan(&n); err != nil {
		return fmt.Errorf("count quest settings: %w", err)
	}
	if n > 0 {
		return nil
	}
	if err := q.SaveQuestConfig(c); err != nil {
		return err
	}
	for _, d := range quests {
		if _, err := q.SaveQuest(d); err != nil {
			return err
		}
	}
	return nil
}

// LoadQuests reads the pool and the draw settings.
func (q *Q) LoadQuests() ([]game.QuestDef, game.QuestConfig, error) {
	var out []game.QuestDef
	err := q.global(`SELECT `+questColumns+` FROM quests ORDER BY sort, id`, func(s scanner) error {
		var d game.QuestDef
		if err := s.Scan(&d.ID, &d.Key, &d.Name, &d.Text, &d.Icon, &d.Color, &d.Kind, &d.Count, &d.Until, &d.Area, &d.Percent, &d.RewardKind, &d.Reward,
			&d.Weight, &d.Days, &d.MinHabits, &d.MinLevel, &d.OnlyArea, &d.Active, &d.Sort); err != nil {
			return err
		}
		out = append(out, d)
		return nil
	})
	if err != nil {
		return nil, game.QuestConfig{}, err
	}
	var c game.QuestConfig
	err = q.r.QueryRowContext(q.ctx, `SELECT per_day, rollover, weighted, no_repeat, matching, streak_bonus, weekend_rule, first_rule FROM quest_settings WHERE id = 1`).
		Scan(&c.PerDay, &c.Rollover, &c.Weighted, &c.NoRepeat, &c.Matching, &c.StreakBonus, &c.WeekendRule, &c.FirstRule)
	if err != nil {
		return nil, c, fmt.Errorf("quest settings: %w", err)
	}
	return out, c, nil
}

// SaveQuestConfig stores the draw settings.
func (q *Q) SaveQuestConfig(c game.QuestConfig) error {
	return q.exec(`INSERT INTO quest_settings (id, per_day, rollover, weighted, no_repeat, matching, streak_bonus, weekend_rule, first_rule)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET per_day = excluded.per_day, rollover = excluded.rollover, weighted = excluded.weighted,
		no_repeat = excluded.no_repeat, matching = excluded.matching, streak_bonus = excluded.streak_bonus,
		weekend_rule = excluded.weekend_rule, first_rule = excluded.first_rule`,
		c.PerDay, c.Rollover, b2i(c.Weighted), b2i(c.NoRepeat), b2i(c.Matching), b2i(c.StreakBonus), b2i(c.WeekendRule), b2i(c.FirstRule))
}

// SaveQuest creates or updates a quest and returns its id.
func (q *Q) SaveQuest(d game.QuestDef) (int64, error) {
	if d.ID != 0 {
		return d.ID, q.execOne(`UPDATE quests SET name = ?, text = ?, icon = ?, color = ?, kind = ?, count = ?, until_time = ?, area = ?, percent = ?,
			reward_kind = ?, reward = ?, weight = ?, days = ?, min_habits = ?, min_level = ?, only_area = ?, active = ?, sort = ? WHERE id = ?`,
			d.Name, d.Text, d.Icon, d.Color, d.Kind, d.Count, d.Until, d.Area, d.Percent, d.RewardKind, d.Reward, d.Weight, d.Days,
			d.MinHabits, d.MinLevel, d.OnlyArea, b2i(d.Active), d.Sort, d.ID)
	}
	res, err := q.r.ExecContext(q.ctx, `INSERT INTO quests (key, name, text, icon, color, kind, count, until_time, area, percent, reward_kind, reward,
		weight, days, min_habits, min_level, only_area, active, sort) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.Key, d.Name, d.Text, d.Icon, d.Color, d.Kind, d.Count, d.Until, d.Area, d.Percent, d.RewardKind, d.Reward, d.Weight, d.Days,
		d.MinHabits, d.MinLevel, d.OnlyArea, b2i(d.Active), d.Sort)
	if err != nil {
		return 0, fmt.Errorf("insert quest: %w", err)
	}
	return res.LastInsertId()
}

// SetQuestActive switches a quest on or off.
func (q *Q) SetQuestActive(id int64, active bool) error {
	return q.execOne(`UPDATE quests SET active = ? WHERE id = ?`, b2i(active), id)
}

// DeleteQuest removes a quest with its plan and history.
func (q *Q) DeleteQuest(id int64) error {
	return q.execOne(`DELETE FROM quests WHERE id = ?`, id)
}

// UserQuest is a quest drawn for the current user.
type UserQuest struct {
	QuestID int64
	Done    bool
}

// UserQuests lists the quests of the current user on a day.
func (q *Q) UserQuests(d model.Day) ([]UserQuest, error) {
	rows, err := q.r.QueryContext(q.ctx, `SELECT quest_id, done FROM user_quests WHERE user_id = ? AND date = ? ORDER BY quest_id`, q.uid, d.String())
	if err != nil {
		return nil, fmt.Errorf("list user quests: %w", err)
	}
	defer rows.Close()
	var out []UserQuest
	for rows.Next() {
		var u UserQuest
		if err := rows.Scan(&u.QuestID, &u.Done); err != nil {
			return nil, fmt.Errorf("scan user quest: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// AddUserQuest records a drawn quest.
func (q *Q) AddUserQuest(d model.Day, questID int64) error {
	return q.exec(`INSERT OR IGNORE INTO user_quests (user_id, date, quest_id) VALUES (?, ?, ?)`, q.uid, d.String(), questID)
}

// SetUserQuestDone stores the outcome of a drawn quest.
func (q *Q) SetUserQuestDone(d model.Day, questID int64, done bool) error {
	return q.exec(`UPDATE user_quests SET done = ? WHERE user_id = ? AND date = ? AND quest_id = ?`, b2i(done), q.uid, d.String(), questID)
}

// QuestDays returns for every day with a quest whether one was finished.
func (q *Q) QuestDays() (map[model.Day]bool, error) {
	rows, err := q.r.QueryContext(q.ctx, `SELECT date, max(done) FROM user_quests WHERE user_id = ? GROUP BY date`, q.uid)
	if err != nil {
		return nil, fmt.Errorf("quest days: %w", err)
	}
	defer rows.Close()
	out := map[model.Day]bool{}
	for rows.Next() {
		var date string
		var done bool
		if err := rows.Scan(&date, &done); err != nil {
			return nil, fmt.Errorf("scan quest day: %w", err)
		}
		d, err := model.ParseDay(date)
		if err != nil {
			return nil, err
		}
		out[d] = done
	}
	return out, rows.Err()
}

// QuestStat counts the draws of a quest.
type QuestStat struct {
	Drawn int
	Done  int
}

// QuestStats sums the draws of all users since a day.
func (q *Q) QuestStats(from model.Day) (map[int64]QuestStat, error) {
	rows, err := q.r.QueryContext(q.ctx, `SELECT quest_id, count(*), coalesce(sum(done), 0) FROM user_quests WHERE date >= ? GROUP BY quest_id`, from.String())
	if err != nil {
		return nil, fmt.Errorf("quest stats: %w", err)
	}
	defer rows.Close()
	out := map[int64]QuestStat{}
	for rows.Next() {
		var id int64
		var st QuestStat
		if err := rows.Scan(&id, &st.Drawn, &st.Done); err != nil {
			return nil, fmt.Errorf("scan quest stat: %w", err)
		}
		out[id] = st
	}
	return out, rows.Err()
}

// PlannedQuests returns the quests fixed for a day.
func (q *Q) PlannedQuests(d model.Day) ([]int64, error) {
	rows, err := q.r.QueryContext(q.ctx, `SELECT quest_id FROM quest_plan WHERE date = ? ORDER BY quest_id`, d.String())
	if err != nil {
		return nil, fmt.Errorf("planned quests: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan planned quest: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// PlanQuest fixes a quest for a day.
func (q *Q) PlanQuest(d model.Day, id int64) error {
	return q.exec(`INSERT OR IGNORE INTO quest_plan (date, quest_id) VALUES (?, ?)`, d.String(), id)
}

// UnplanQuest takes a quest off a day.
func (q *Q) UnplanQuest(d model.Day, id int64) error {
	return q.exec(`DELETE FROM quest_plan WHERE date = ? AND quest_id = ?`, d.String(), id)
}

// QuestEvent changes the quests of a period.
type QuestEvent struct {
	ID         int64
	Name       string
	Start      model.Day
	End        model.Day
	QuestID    int64 // an extra quest for every day, 0 for none
	Multiplier int
}

// QuestEvents lists all events by date.
func (q *Q) QuestEvents() ([]QuestEvent, error) {
	var out []QuestEvent
	err := q.global(`SELECT id, name, start_date, end_date, quest_id, multiplier FROM quest_events ORDER BY start_date, id`, func(s scanner) error {
		var e QuestEvent
		var start, end string
		var quest sql.NullInt64
		if err := s.Scan(&e.ID, &e.Name, &start, &end, &quest, &e.Multiplier); err != nil {
			return err
		}
		var err error
		if e.Start, err = model.ParseDay(start); err != nil {
			return err
		}
		if e.End, err = model.ParseDay(end); err != nil {
			return err
		}
		e.QuestID = quest.Int64
		out = append(out, e)
		return nil
	})
	return out, err
}

// SaveQuestEvent stores a new event.
func (q *Q) SaveQuestEvent(e QuestEvent) error {
	return q.exec(`INSERT INTO quest_events (name, start_date, end_date, quest_id, multiplier) VALUES (?, ?, ?, ?, ?)`,
		e.Name, e.Start.String(), e.End.String(), nullable(e.QuestID), e.Multiplier)
}

// DeleteQuestEvent removes an event.
func (q *Q) DeleteQuestEvent(id int64) error {
	err := q.execOne(`DELETE FROM quest_events WHERE id = ?`, id)
	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	return err
}

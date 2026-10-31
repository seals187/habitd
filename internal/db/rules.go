package db

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/seals187/habitd/internal/game"
)

// SeedRules stores the start values. New built-in badges of a later version are
// added, nothing that exists is touched.
func (q *Q) SeedRules(r *game.Rules) error {
	for _, c := range r.Categories {
		if err := q.exec(`INSERT OR IGNORE INTO badge_categories (key, name, color, sort) VALUES (?, ?, ?, ?)`,
			c.Key, c.Name, c.Color, c.Sort); err != nil {
			return err
		}
	}
	for _, b := range r.Badges {
		if err := q.insertBadge(`INSERT OR IGNORE`, b); err != nil {
			return err
		}
	}
	var levels int
	if err := q.r.QueryRowContext(q.ctx, `SELECT count(*) FROM levels`).Scan(&levels); err != nil {
		return fmt.Errorf("count levels: %w", err)
	}
	if levels == 0 {
		if err := q.SaveLevels(r.Levels); err != nil {
			return err
		}
	}
	var settings int
	if err := q.r.QueryRowContext(q.ctx, `SELECT count(*) FROM level_settings`).Scan(&settings); err != nil {
		return fmt.Errorf("count level settings: %w", err)
	}
	if settings == 0 {
		return q.SaveLevelSettings(r.Settings)
	}
	return nil
}

func (q *Q) insertBadge(verb string, b game.Badge) error {
	m := b.Metric
	return q.exec(verb+` INTO badges (key, name, hint, description, category_key, icon, color, rule_type, scope, scope_value,
		target, count_jokers, secret, reward_coins, reward_jokers, sort, active, builtin)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.Key, b.Name, b.Hint, b.Description, b.Category, b.Icon, b.Color, m.Rule, m.Scope, m.Value,
		b.Target, b2i(m.Jokers), b2i(b.Secret), b.Coins, b.Jokers, b.Sort, b2i(b.Active), b2i(b.Builtin))
}

// LoadRules reads all global rules.
func (q *Q) LoadRules() (*game.Rules, error) {
	r := &game.Rules{}
	err := q.global(`SELECT key, name, color, sort FROM badge_categories ORDER BY sort, key`, func(s scanner) error {
		var c game.Category
		if err := s.Scan(&c.Key, &c.Name, &c.Color, &c.Sort); err != nil {
			return err
		}
		r.Categories = append(r.Categories, c)
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = q.global(`SELECT key, name, hint, description, category_key, icon, color, rule_type, scope, scope_value,
		target, count_jokers, secret, reward_coins, reward_jokers, sort, active, builtin FROM badges ORDER BY sort, key`, func(s scanner) error {
		var b game.Badge
		var m game.Metric
		if err := s.Scan(&b.Key, &b.Name, &b.Hint, &b.Description, &b.Category, &b.Icon, &b.Color, &m.Rule, &m.Scope, &m.Value,
			&b.Target, &m.Jokers, &b.Secret, &b.Coins, &b.Jokers, &b.Sort, &b.Active, &b.Builtin); err != nil {
			return err
		}
		b.Metric = m.WithSpecial(b.Key)
		r.Badges = append(r.Badges, b)
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = q.global(`SELECT level, title, xp_required, reward_coins, reward_jokers, badge_key, overridden FROM levels ORDER BY level`, func(s scanner) error {
		var l game.Level
		if err := s.Scan(&l.Level, &l.Title, &l.XP, &l.Coins, &l.Jokers, &l.BadgeKey, &l.Overridden); err != nil {
			return err
		}
		r.Levels = append(r.Levels, l)
		return nil
	})
	if err != nil {
		return nil, err
	}
	st := &r.Settings
	err = q.r.QueryRowContext(q.ctx, `SELECT base, exponent, tempo, max_level, limit_level, celebrate, no_drop, show_title FROM level_settings WHERE id = 1`).
		Scan(&st.Base, &st.Exponent, &st.Tempo, &st.MaxLevel, &st.Limit, &st.Celebrate, &st.NoDrop, &st.ShowTitle)
	if err != nil {
		return nil, fmt.Errorf("level settings: %w", err)
	}
	return r, nil
}

func (q *Q) global(query string, fn func(scanner) error) error {
	rows, err := q.r.QueryContext(q.ctx, query)
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

// SaveLevels replaces the level table.
func (q *Q) SaveLevels(levels []game.Level) error {
	if err := q.exec(`DELETE FROM levels`); err != nil {
		return err
	}
	for _, l := range levels {
		if err := q.exec(`INSERT INTO levels (level, title, xp_required, reward_coins, reward_jokers, badge_key, overridden)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, l.Level, l.Title, l.XP, l.Coins, l.Jokers, l.BadgeKey, b2i(l.Overridden)); err != nil {
			return err
		}
	}
	return nil
}

// SaveLevelSettings stores curve and switches.
func (q *Q) SaveLevelSettings(s game.LevelSettings) error {
	return q.exec(`INSERT INTO level_settings (id, base, exponent, tempo, max_level, limit_level, celebrate, no_drop, show_title)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET base = excluded.base, exponent = excluded.exponent, tempo = excluded.tempo,
		max_level = excluded.max_level, limit_level = excluded.limit_level, celebrate = excluded.celebrate,
		no_drop = excluded.no_drop, show_title = excluded.show_title`,
		s.Base, s.Exponent, s.Tempo, s.MaxLevel, b2i(s.Limit), b2i(s.Celebrate), b2i(s.NoDrop), b2i(s.ShowTitle))
}

// SaveBadge creates or replaces a badge.
func (q *Q) SaveBadge(b game.Badge) error { return q.insertBadge(`INSERT OR REPLACE`, b) }

// SetBadgeActive switches a badge on or off.
func (q *Q) SetBadgeActive(key string, active bool) error {
	return q.execOne(`UPDATE badges SET active = ? WHERE key = ?`, b2i(active), key)
}

// DeleteBadge removes a custom badge. Holders keep nothing visible of it, the
// unlock row goes with it.
func (q *Q) DeleteBadge(key string) error {
	if err := q.execOne(`DELETE FROM badges WHERE key = ? AND builtin = 0`, key); err != nil {
		return err
	}
	return q.exec(`DELETE FROM badges_unlocked WHERE badge_key = ?`, key)
}

// BadgeHolders counts the users who have a badge.
func (q *Q) BadgeHolders(key string) (int, error) {
	var n int
	if err := q.r.QueryRowContext(q.ctx, `SELECT count(*) FROM badges_unlocked WHERE badge_key = ?`, key).Scan(&n); err != nil {
		return 0, fmt.Errorf("count holders: %w", err)
	}
	return n, nil
}

// SaveCategory creates or updates a category.
func (q *Q) SaveCategory(c game.Category) error {
	return q.exec(`INSERT INTO badge_categories (key, name, color, sort) VALUES (?, ?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET name = excluded.name, color = excluded.color, sort = excluded.sort`,
		c.Key, c.Name, c.Color, c.Sort)
}

// ErrCategoryInUse is returned for a category that still holds badges.
var ErrCategoryInUse = errors.New("category in use")

// DeleteCategory removes an empty category.
func (q *Q) DeleteCategory(key string) error {
	var n int
	err := q.r.QueryRowContext(q.ctx, `SELECT count(*) FROM badges WHERE category_key = ?`, key).Scan(&n)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("count category badges: %w", err)
	}
	if n > 0 {
		return ErrCategoryInUse
	}
	return q.execOne(`DELETE FROM badge_categories WHERE key = ?`, key)
}

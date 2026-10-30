-- Habits get a template reference and the monthly rhythm. SQLite cannot change
-- a CHECK constraint, so the table is rebuilt; the runner keeps foreign keys off.
CREATE TABLE habits_new (
    id            INTEGER PRIMARY KEY,
    template_id   TEXT,
    name          TEXT NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('build', 'avoid')),
    frequency     TEXT NOT NULL CHECK (frequency IN ('daily', 'weekly', 'custom', 'monthly')),
    weekly_target INTEGER NOT NULL DEFAULT 0,
    days_mask     INTEGER NOT NULL DEFAULT 0,
    icon          TEXT NOT NULL DEFAULT '',
    tag           TEXT NOT NULL DEFAULT '',
    area_id       INTEGER NOT NULL REFERENCES areas (id),
    difficulty    TEXT NOT NULL DEFAULT 'easy' CHECK (difficulty IN ('easy', 'medium', 'hard')),
    archived      INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL,
    sort          INTEGER NOT NULL DEFAULT 0
);

INSERT INTO habits_new (id, name, kind, frequency, weekly_target, days_mask, icon, tag, area_id, difficulty, archived, created_at, sort)
SELECT id, name, kind, frequency, weekly_target, days_mask,
       CASE icon
           WHEN 'droplet' THEN 'drop'
           WHEN 'dumbbell' THEN 'dumb'
           WHEN 'hand-heart' THEN 'handheart'
           WHEN 'lightbulb' THEN 'bulb'
           WHEN 'footprints' THEN 'run'
           WHEN 'briefcase' THEN 'brief'
           WHEN 'utensils' THEN 'meal'
           WHEN 'coffee' THEN 'cup'
           ELSE icon
       END,
       tag, area_id, difficulty, archived, created_at, sort
FROM habits;

DROP TABLE habits;
ALTER TABLE habits_new RENAME TO habits;

UPDATE areas SET color = '#F5A3A3' WHERE key = 'freunde';
UPDATE areas SET icon = 'handheart' WHERE key = 'gute-taten';
UPDATE areas SET icon = 'home' WHERE key = 'familie';
UPDATE areas SET icon = 'brief' WHERE key = 'arbeit';

INSERT INTO areas (key, name, color, icon, sort) VALUES
    ('digital', 'Digital', '#8C8CF5', 'phoneoff', 8),
    ('geld',    'Geld',    '#D9B26B', 'wallet',   9),
    ('umwelt',  'Umwelt',  '#8ACB7A', 'leaf',     10);

-- Installations from before the onboarding do not need it.
INSERT INTO settings (key, value)
SELECT 'onboarded', '1' WHERE EXISTS (SELECT 1 FROM habits);

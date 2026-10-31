-- Counter and quantity habits log every click in habit_entries. The rebuild also
-- adds AUTOINCREMENT: ledger refs carry the habit id, and now that habits can be
-- deleted a reused id would inherit the bookings of the old habit.
CREATE TABLE habits_new (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
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
    sort          INTEGER NOT NULL DEFAULT 0,
    measure       TEXT NOT NULL DEFAULT 'check' CHECK (measure IN ('check', 'count', 'amount')),
    target        INTEGER NOT NULL DEFAULT 1,
    step          INTEGER NOT NULL DEFAULT 1,
    unit          TEXT NOT NULL DEFAULT ''
);

INSERT INTO habits_new (id, template_id, name, kind, frequency, weekly_target, days_mask, icon, tag, area_id, difficulty, archived, created_at, sort)
SELECT id, template_id, name, kind, frequency, weekly_target, days_mask, icon, tag, area_id, difficulty, archived, created_at, sort
FROM habits;

DROP TABLE habits;
ALTER TABLE habits_new RENAME TO habits;

-- Habits taken from these templates become what the template now describes.
UPDATE habits SET measure = 'count', target = 8, step = 1, unit = 'Gläser' WHERE kind = 'build' AND name = '8 Gläser Wasser trinken';
UPDATE habits SET measure = 'amount', target = 10000, step = 1000, unit = 'Schritte' WHERE kind = 'build' AND name = '10.000 Schritte';
UPDATE habits SET measure = 'amount', target = 30, step = 5, unit = 'Min.' WHERE kind = 'build' AND name IN ('30 Min. Sport', '30 Min. Weiterbildung');
UPDATE habits SET measure = 'amount', target = 20, step = 5, unit = 'Min.' WHERE kind = 'build' AND name = '20 Min. lesen';
UPDATE habits SET measure = 'amount', target = 10, step = 5, unit = 'Min.' WHERE kind = 'build' AND name = '10 Min. aufräumen';
UPDATE habits SET measure = 'amount', target = 90, step = 15, unit = 'Min.' WHERE kind = 'build' AND name = '90 Min. konzentriert arbeiten';

CREATE TABLE habit_entries (
    id         INTEGER PRIMARY KEY,
    habit_id   INTEGER NOT NULL REFERENCES habits (id) ON DELETE CASCADE,
    date       TEXT NOT NULL,
    amount     INTEGER NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX habit_entries_day ON habit_entries (habit_id, date);

-- Daily quests: a global pool, a plan and one draw per user and day. The start
-- values come from the game package, see Service.LoadRules.
CREATE TABLE quests (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    key         TEXT NOT NULL DEFAULT '',
    name        TEXT NOT NULL,
    text        TEXT NOT NULL DEFAULT '',
    icon        TEXT NOT NULL,
    color       TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('all', 'until', 'area', 'counter', 'catchup', 'avoid')),
    count       INTEGER NOT NULL DEFAULT 0, -- habits to finish, 0 means all
    until_time  TEXT NOT NULL DEFAULT '',
    area        TEXT NOT NULL DEFAULT '',   -- empty means the weakest area of the week
    percent     INTEGER NOT NULL DEFAULT 100,
    reward_kind TEXT NOT NULL DEFAULT 'coins' CHECK (reward_kind IN ('coins', 'xp', 'joker')),
    reward      INTEGER NOT NULL DEFAULT 0,
    weight      INTEGER NOT NULL DEFAULT 2,
    days        INTEGER NOT NULL DEFAULT 127,
    min_habits  INTEGER NOT NULL DEFAULT 0,
    min_level   INTEGER NOT NULL DEFAULT 0,
    only_area   TEXT NOT NULL DEFAULT '',
    active      INTEGER NOT NULL DEFAULT 1,
    sort        INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE quest_settings (
    id           INTEGER PRIMARY KEY CHECK (id = 1),
    per_day      INTEGER NOT NULL DEFAULT 1,
    rollover     TEXT NOT NULL DEFAULT '04:00',
    weighted     INTEGER NOT NULL DEFAULT 1,
    no_repeat    INTEGER NOT NULL DEFAULT 1,
    matching     INTEGER NOT NULL DEFAULT 1,
    streak_bonus INTEGER NOT NULL DEFAULT 1,
    weekend_rule INTEGER NOT NULL DEFAULT 1,
    first_rule   INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE quest_plan (
    date     TEXT NOT NULL,
    quest_id INTEGER NOT NULL REFERENCES quests (id) ON DELETE CASCADE,
    PRIMARY KEY (date, quest_id)
) WITHOUT ROWID;

CREATE TABLE quest_events (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    start_date TEXT NOT NULL,
    end_date   TEXT NOT NULL,
    quest_id   INTEGER REFERENCES quests (id) ON DELETE SET NULL,
    multiplier INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE user_quests (
    user_id  INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    date     TEXT NOT NULL,
    quest_id INTEGER NOT NULL REFERENCES quests (id) ON DELETE CASCADE,
    done     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, date, quest_id)
) WITHOUT ROWID;

CREATE INDEX user_quests_quest ON user_quests (quest_id, date);

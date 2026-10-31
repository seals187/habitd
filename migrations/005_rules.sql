-- Levels and badges move from the code into the database. The start values come
-- from the game package on the first start, see Service.LoadRules.
CREATE TABLE levels (
    level         INTEGER PRIMARY KEY,
    title         TEXT NOT NULL,
    xp_required   INTEGER NOT NULL,
    reward_coins  INTEGER NOT NULL DEFAULT 0,
    reward_jokers INTEGER NOT NULL DEFAULT 0,
    badge_key     TEXT NOT NULL DEFAULT '',
    overridden    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE level_settings (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    base        REAL NOT NULL,
    exponent    REAL NOT NULL,
    tempo       TEXT NOT NULL DEFAULT 'normal' CHECK (tempo IN ('gemuetlich', 'normal', 'ehrgeizig')),
    max_level   INTEGER NOT NULL DEFAULT 50,
    limit_level INTEGER NOT NULL DEFAULT 0,
    celebrate   INTEGER NOT NULL DEFAULT 1,
    no_drop     INTEGER NOT NULL DEFAULT 1,
    show_title  INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE badge_categories (
    key   TEXT PRIMARY KEY,
    name  TEXT NOT NULL,
    color TEXT NOT NULL,
    sort  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE badges (
    key           TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    hint          TEXT NOT NULL DEFAULT '',
    description   TEXT NOT NULL DEFAULT '',
    category_key  TEXT NOT NULL REFERENCES badge_categories (key),
    icon          TEXT NOT NULL,
    color         TEXT NOT NULL DEFAULT '',
    rule_type     TEXT NOT NULL CHECK (rule_type IN ('streak', 'count', 'amount', 'special')),
    scope         TEXT NOT NULL DEFAULT 'global' CHECK (scope IN ('any_habit', 'habit_tag', 'area', 'kind_build', 'kind_avoid', 'global')),
    scope_value   TEXT NOT NULL DEFAULT '',
    target        INTEGER NOT NULL DEFAULT 1,
    count_jokers  INTEGER NOT NULL DEFAULT 0,
    secret        INTEGER NOT NULL DEFAULT 0,
    reward_coins  INTEGER NOT NULL DEFAULT 0,
    reward_jokers INTEGER NOT NULL DEFAULT 0,
    sort          INTEGER NOT NULL DEFAULT 0,
    active        INTEGER NOT NULL DEFAULT 1,
    builtin       INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX badges_category ON badges (category_key);

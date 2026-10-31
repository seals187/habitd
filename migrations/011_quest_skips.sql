-- Habits a user leaves out of a quest.
CREATE TABLE quest_skips (
    user_id  INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    quest_id INTEGER NOT NULL REFERENCES quests (id) ON DELETE CASCADE,
    habit_id INTEGER NOT NULL REFERENCES habits (id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, quest_id, habit_id)
) WITHOUT ROWID;

-- Habits on the Today screen can stay folded.
ALTER TABLE habits ADD COLUMN collapsed INTEGER NOT NULL DEFAULT 0;

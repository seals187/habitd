-- A counter or quantity goal can add up over a week instead of a day.
ALTER TABLE habits ADD COLUMN goal_week INTEGER NOT NULL DEFAULT 0;

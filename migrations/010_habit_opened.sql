-- The day a reached habit was opened again on the Today screen.
ALTER TABLE habits ADD COLUMN opened_on TEXT NOT NULL DEFAULT '';

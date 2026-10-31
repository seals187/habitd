-- Rewards get an icon, a weekly limit, a pause and a place on the Today screen.
-- redemptions.ledger_ref lets a redemption be taken back exactly.
ALTER TABLE rewards ADD COLUMN icon TEXT NOT NULL DEFAULT 'gift';
ALTER TABLE rewards ADD COLUMN weekly INTEGER NOT NULL DEFAULT 0;
ALTER TABLE rewards ADD COLUMN paused INTEGER NOT NULL DEFAULT 0;
ALTER TABLE rewards ADD COLUMN show_today INTEGER NOT NULL DEFAULT 0;
ALTER TABLE redemptions ADD COLUMN ledger_ref TEXT NOT NULL DEFAULT '';

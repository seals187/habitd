-- Folded and opened cards now live in cookies, one set per device.
ALTER TABLE habits DROP COLUMN collapsed;
ALTER TABLE habits DROP COLUMN opened_on;

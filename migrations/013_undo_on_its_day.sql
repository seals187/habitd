-- Taking a past day back booked the undo on the day it happened, so that day read
-- negative and the day it belongs to too high. It now moves to the first booking of
-- its ref. Balances stay the same, only the days change.
UPDATE ledger
SET at = (
    SELECT min(o.at) FROM ledger o
    WHERE o.user_id IS ledger.user_id AND o.ref = ledger.ref AND o.kind = ledger.kind AND o.reason <> 'undo'
)
WHERE reason = 'undo' AND ref <> '' AND kind IN ('xp', 'coins') AND EXISTS (
    SELECT 1 FROM ledger o
    WHERE o.user_id IS ledger.user_id AND o.ref = ledger.ref AND o.kind = ledger.kind AND o.reason <> 'undo'
);

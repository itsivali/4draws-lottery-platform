-- +goose Up
-- Owner, 2026-10-26: while the apps are tested with real M-Pesa money, a play costs KES 1 (top-ups go down to KES 1 with
-- TOP_UP_MIN_KES). The odds stay those in force; tickets keep the price they were sold at. Going live adds a version
-- with the launch price from the launch date (a new migration, never an edit of this one).
INSERT INTO game.odds_config (slot_count, lucky_slots, price_kes, effective_from, created_by)
SELECT slot_count, lucky_slots, 1, DATE '2026-10-26', 'owner:2026-10-26 testing price'
FROM game.odds_config WHERE effective_from <= DATE '2026-10-26' ORDER BY effective_from DESC LIMIT 1;

-- +goose Down
DELETE FROM game.odds_config WHERE effective_from = DATE '2026-10-26' AND created_by = 'owner:2026-10-26 testing price';

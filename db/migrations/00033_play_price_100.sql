-- +goose Up
-- Owner, 2026-10-28: a spin costs KES 100 again (the KES 1 testing price of migration 00028 is over); plays are paid
-- through the Daraja sandbox from the player's own M-Pesa number. The odds stay those in force; tickets keep the price
-- they were sold at.
INSERT INTO game.odds_config (slot_count, lucky_slots, price_kes, effective_from, created_by)
SELECT slot_count, lucky_slots, 100, DATE '2026-10-28', 'owner:2026-10-28 KES 100 a spin'
FROM game.odds_config WHERE effective_from <= DATE '2026-10-28' ORDER BY effective_from DESC LIMIT 1;

-- +goose Down
DELETE FROM game.odds_config WHERE effective_from = DATE '2026-10-28' AND created_by = 'owner:2026-10-28 KES 100 a spin';

-- +goose Up
-- Owner, 2026-10-29: a spin costs KES 1, so a top-up of a shilling (TOP_UP_MIN_KES=1) is enough to play. The odds stay
-- those in force; tickets keep the price they were sold at. A later price is a new version from its date, never an
-- edit of this one.
INSERT INTO game.odds_config (slot_count, lucky_slots, price_kes, effective_from, created_by)
SELECT slot_count, lucky_slots, 1, DATE '2026-10-29', 'owner:2026-10-29 KES 1 a spin'
FROM game.odds_config WHERE effective_from <= DATE '2026-10-29' ORDER BY effective_from DESC LIMIT 1;

-- +goose Down
DELETE FROM game.odds_config WHERE effective_from = DATE '2026-10-29' AND created_by = 'owner:2026-10-29 KES 1 a spin';

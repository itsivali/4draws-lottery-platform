-- +goose Up
-- Owner, 2026-11-13: staff sign-in addresses on the old domain move to 4draws.co.ke (the product was renamed on
-- 2026-11-11). The password, roles and sessions of each member stay as they are; they sign in with the new address
-- from now on. A member whose new address already belongs to another member is left alone.
UPDATE staff.members m
   SET email = split_part(m.email, '@', 1) || '@4draws.co.ke', updated_at = now()
 WHERE m.email LIKE '%@tajirika.co.ke'
   AND NOT EXISTS (SELECT 1 FROM staff.members o WHERE o.email = split_part(m.email, '@', 1) || '@4draws.co.ke');

-- +goose Down
UPDATE staff.members m
   SET email = split_part(m.email, '@', 1) || '@tajirika.co.ke', updated_at = now()
 WHERE m.email LIKE '%@4draws.co.ke'
   AND NOT EXISTS (SELECT 1 FROM staff.members o WHERE o.email = split_part(m.email, '@', 1) || '@tajirika.co.ke');

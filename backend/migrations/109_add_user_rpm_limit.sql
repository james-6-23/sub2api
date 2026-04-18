-- Add per-user Requests-Per-Minute cap.
-- rpm_limit: maximum cross-group requests per minute allowed for this user
-- (0 = unlimited). This works independently from groups.rpm_limit: the user-level
-- counter aggregates across ALL groups the user accesses, while the group-level
-- counter stays scoped to (user, group). Whichever limit is hit first triggers 429.
ALTER TABLE users ADD COLUMN IF NOT EXISTS rpm_limit integer NOT NULL DEFAULT 0;

COMMENT ON COLUMN users.rpm_limit IS 'Per-user cross-group requests-per-minute cap; 0 = unlimited. Enforced independently from groups.rpm_limit.';

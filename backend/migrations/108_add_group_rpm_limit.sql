-- Add per-group Requests-Per-Minute limit for downstream API Keys.
-- rpm_limit: maximum requests per minute allowed for EACH API Key that belongs
-- to this group (0 = unlimited). The counter is tracked per api_key in Redis;
-- the group only stores the policy value.
ALTER TABLE groups ADD COLUMN IF NOT EXISTS rpm_limit integer NOT NULL DEFAULT 0;

COMMENT ON COLUMN groups.rpm_limit IS 'Per-API-Key requests-per-minute cap enforced when the key is bound to this group; 0 = unlimited.';

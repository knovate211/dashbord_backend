-- Attempts are counted per paid registration, not per person for all time.
-- A certification invite row is reused when the same email buys a resit (and
-- touched again by an admin resend), so neither its id nor sent_at marks where
-- the current registration's attempts begin. attempts_since does: it is set
-- when a registration is paid, and attempts started before it do not count
-- against the paper's max_attempts. NULL keeps the old per-person count.
ALTER TABLE assessment_invites ADD COLUMN IF NOT EXISTS attempts_since timestamptz;

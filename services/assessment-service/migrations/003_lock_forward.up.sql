-- Sequential papers: the candidate may not move past a question until it is
-- answered.
--
-- This is distinct from allow_backtrack, which governs the other direction.
-- The two combine: a scholarship paper sets lock_forward so the questions are
-- revealed one at a time, while leaving backtracking on so a candidate can
-- revisit and change an answer they have already committed.
--
-- Default false — every existing practice and hiring paper keeps the free
-- navigation it was authored with.
ALTER TABLE assessments
    ADD COLUMN IF NOT EXISTS lock_forward BOOLEAN NOT NULL DEFAULT false;

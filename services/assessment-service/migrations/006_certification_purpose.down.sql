-- Certification papers become scholarship papers rather than practice tests:
-- both are invite-only, so a rollback cannot throw a paid exam open to every
-- logged-in student. The certification tables keep pointing at them, and the
-- rows are recoverable by re-applying this migration.
UPDATE assessments SET purpose = 'scholarship' WHERE purpose = 'certification';

ALTER TABLE assessments DROP CONSTRAINT IF EXISTS assessments_purpose_check;
ALTER TABLE assessments ADD  CONSTRAINT assessments_purpose_check
    CHECK (purpose IN ('practice', 'hiring', 'scholarship'));

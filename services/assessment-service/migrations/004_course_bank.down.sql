ALTER TABLE assessment_sections DROP COLUMN IF EXISTS pick_course;
DROP INDEX IF EXISTS idx_mcq_course;
ALTER TABLE mcq_questions DROP COLUMN IF EXISTS course_id;

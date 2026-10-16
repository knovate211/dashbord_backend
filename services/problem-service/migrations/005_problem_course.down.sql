DROP INDEX IF EXISTS idx_problems_course;
ALTER TABLE problems DROP COLUMN IF EXISTS course_id;

-- Course a problem belongs to, matching the catalog / user_courses course id.
-- '' = general, usable by any course. Mirrors mcq_questions.course_id.
ALTER TABLE problems ADD COLUMN IF NOT EXISTS course_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_problems_course ON problems(course_id);

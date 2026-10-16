-- Course-based question bank.
--
-- course_id matches the catalog / user_courses course id ('5', 'genai', …).
-- '' means a general question that belongs to no single course — the existing
-- aptitude bank, shared by every course's paper.
ALTER TABLE mcq_questions ADD COLUMN IF NOT EXISTS course_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_mcq_course ON mcq_questions(course_id) WHERE is_active;

-- A random-draw section can be limited to one course's questions.
-- '' = no course filter (topic/difficulty only), as before.
ALTER TABLE assessment_sections ADD COLUMN IF NOT EXISTS pick_course TEXT NOT NULL DEFAULT '';

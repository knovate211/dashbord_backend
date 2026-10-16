-- Who a practice test is for. Empty = every signed-in student (the behaviour
-- before this column existed); otherwise only students enrolled in at least
-- one of these courses (user_courses.course_id) see and can start it.
-- Hiring and scholarship papers are invite-only and ignore this.
ALTER TABLE assessments ADD COLUMN IF NOT EXISTS course_ids TEXT[] NOT NULL DEFAULT '{}';

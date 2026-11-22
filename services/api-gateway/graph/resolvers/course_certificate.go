package resolvers

import (
	"context"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

// Course completion certificates.
//
// A learner who reaches the last item of a course syllabus (courseTotals in
// college.go) gets one "course" certificate for it, issued once. It sits in
// the same certificates table as exam certificates, so it has a credential ID,
// a public /verify page and shows in the student app's Certificate tab.
//
// Progress is reported by the course viewer, so this certifies that the
// syllabus was worked through, not that it was examined — the certificate's
// wording says "completed", and only an exam certificate says "passed".

var courseCertIndexOnce sync.Once

// courseCertificateTitle is the title printed on a course certificate.
func courseCertificateTitle(courseKey string) (string, bool) {
	c, ok := courseTotals[courseKey]
	if !ok || c.Total == 0 {
		return "", false
	}
	return c.Title, true
}

// courseComplete reports whether a furthest-read index finishes the course.
func courseComplete(courseKey string, maxIndex int) bool {
	c, ok := courseTotals[courseKey]
	return ok && c.Total > 0 && maxIndex+1 >= c.Total
}

// issueCourseCertificate issues the learner's certificate for a finished
// course, or returns the one already issued. Safe to call on every save.
func (h *CourseProgressHandler) issueCourseCertificate(ctx context.Context, userID, courseKey string) (string, error) {
	title, ok := courseCertificateTitle(courseKey)
	if !ok {
		return "", nil
	}
	// One course certificate per learner and course; the index makes a race
	// between two tabs harmless. Created lazily because the certificates
	// table belongs to the certification handler.
	courseCertIndexOnce.Do(func() {
		if _, err := h.Pool.Exec(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS certificates_course_once
			ON certificates (user_id, title) WHERE source = 'course'`); err != nil {
			h.Log.Warn("course certificate index", zap.Error(err))
		}
	})

	var id string
	err := h.Pool.QueryRow(ctx, `SELECT credential_id FROM certificates
		WHERE user_id = $1::uuid AND title = $2 AND source = 'course'`, userID, title).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	for try := 0; try < 3; try++ {
		cred, err := newCredentialID()
		if err != nil {
			return "", err
		}
		err = h.Pool.QueryRow(ctx, `
			INSERT INTO certificates (credential_id, user_id, holder_name, title, source, revoke_reason)
			SELECT $1, u.id, COALESCE(NULLIF(TRIM(u.name), ''), split_part(u.email, '@', 1)), $3, 'course', ''
			FROM users u WHERE u.id = $2::uuid
			ON CONFLICT DO NOTHING
			RETURNING credential_id`, cred, userID, title).Scan(&id)
		if err == nil {
			h.Log.Info("course certificate issued", zap.String("credential", id), zap.String("course", title))
			return id, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		// Nothing inserted: either another request issued it (read it back)
		// or the credential ID collided (try a new one).
		if err := h.Pool.QueryRow(ctx, `SELECT credential_id FROM certificates
			WHERE user_id = $1::uuid AND title = $2 AND source = 'course'`, userID, title).Scan(&id); err == nil {
			return id, nil
		}
	}
	return "", errors.New("could not issue course certificate")
}

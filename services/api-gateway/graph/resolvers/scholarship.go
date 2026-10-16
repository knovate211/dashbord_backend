package resolvers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
	pkgauth "github.com/knovate211/pkg/auth"
)

// ScholarshipHandler runs the public scholarship funnel: a visitor picks a
// course on the marketing site, and lands inside the test portal holding a real
// session, without ever having created an account.
//
// It is the second unauthenticated write endpoint in the gateway, after
// /api/inquiries, and it deliberately mirrors that handler's shape — its own
// pool, ensureTable at boot, honeypot, per-IP rate limiting, every field
// length-capped before it reaches Postgres. What it adds over an enquiry is
// consequence: an apply call provisions a user, grants an assessment invite and
// mints a credential. Three rules follow from that, and each is load-bearing:
//
//   - An existing account is never re-credentialed. Applying with somebody
//     else's email must not touch their password or their role, or the funnel
//     becomes an account-takeover endpoint.
//   - The claim token is stored as a SHA-256 digest. A database read alone
//     cannot then impersonate an applicant.
//   - Eligibility is the assessment_invites row, enforced in assessment-service
//     by resolveInvite. This handler writes that row; it never grants access by
//     any other means.
//
// It talks to Postgres directly rather than through a service. Every table it
// touches already lives in the same database, and routing an insert through a
// new gRPC service would be plumbing for its own sake.
type ScholarshipHandler struct {
	Pool *pgxpool.Pool
	Log  *zap.Logger

	// jwtSecret signs the session minted on claim. Same secret, same claims and
	// same lifetime as /api/login — a claimed session is an ordinary session.
	jwtSecret string
	// appBase is the test portal's public origin, used to build the claim URL.
	appBase string

	ipLimiter    *rateLimiter
	emailLimiter *rateLimiter
	mailer       *scholarshipMailer
}

// claimTTL bounds how long a claim link works.
//
// The link is a bearer credential, so it wants to be short-lived; but a
// candidate who applies on a phone during a commute and sits the test that
// evening is the normal case, not the exception. Three days covers that without
// leaving the link useful for a week.
const claimTTL = 72 * time.Hour

// terminalStatuses are the application states in which the funnel is finished.
// A claim against one of these is refused rather than re-armed.
var terminalStatuses = map[string]bool{
	"submitted": true, "evaluated": true, "awarded": true, "rejected": true,
}

// spentAttempt are the attempt states that mean the candidate has had their go.
// assessment-service enforces max_attempts regardless, so this exists to fail
// early with a sentence a candidate can understand rather than late with
// "no attempts left" after they have filled in the form again.
var spentAttempt = map[string]bool{
	"submitted": true, "evaluating": true, "evaluated": true,
	"disqualified": true, "expired": true,
}

// NewScholarshipHandler wires the handler and ensures its tables exist.
func NewScholarshipHandler(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger, jwtSecret, appBase string) (*ScholarshipHandler, error) {
	h := &ScholarshipHandler{
		Pool:      pool,
		Log:       log,
		jwtSecret: jwtSecret,
		appBase:   strings.TrimRight(appBase, "/"),
		// A scholarship campaign is a burst by design: a lecturer says "open
		// this now" and a lab of forty applies inside a minute, all from one
		// NAT address. The enquiry form's 30-per-10-minutes was written for a
		// trickle of contact-form submissions and cannot survive that — a
		// 50-candidate dry run lost 20 of them to this limiter.
		//
		// So the per-IP cap is sized for a cohort and left tunable, because the
		// right number depends on the campaign. It is a flood stop, not the
		// abuse control.
		ipLimiter: newRateLimiter(envInt("SCHOLARSHIP_IP_LIMIT", 0), envMinutes("SCHOLARSHIP_IP_WINDOW_MIN", 10)),
		// This is the abuse control, and it stays tight. Nobody legitimately
		// applies to the same programme five times in an hour, and a per-address
		// limit is what stops one person farming accounts or probing which
		// emails already hold one — neither of which a bigger cohort changes.
		emailLimiter: newRateLimiter(envInt("SCHOLARSHIP_EMAIL_LIMIT", 5), time.Hour),
		mailer:       newScholarshipMailer(log),
	}
	if err := h.ensureTable(ctx); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *ScholarshipHandler) ensureTable(ctx context.Context) error {
	_, err := h.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS scholarship_programs (
			id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			-- Matches the marketing catalog's course id and user_courses.course_id,
			-- so "the course they applied for" and "the course they enrol on" are
			-- the same string everywhere.
			course_id     TEXT        NOT NULL UNIQUE,
			course_name   TEXT        NOT NULL,
			-- FK-by-convention into assessments, matching how section_questions
			-- already references problem-service.
			assessment_id UUID        NOT NULL,
			is_active     BOOLEAN     NOT NULL DEFAULT true,
			opens_at      TIMESTAMPTZ,
			closes_at     TIMESTAMPTZ,
			-- 0 = unlimited.
			seats         INT         NOT NULL DEFAULT 0,
			-- [{"minPercent":80,"awardPercent":100}, …], highest first.
			award_slabs   JSONB       NOT NULL DEFAULT '[]',
			created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
		);

		CREATE TABLE IF NOT EXISTS scholarship_applications (
			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			program_id       UUID NOT NULL REFERENCES scholarship_programs(id) ON DELETE RESTRICT,
			course_id        TEXT NOT NULL,

			name             TEXT NOT NULL,
			email            TEXT NOT NULL,
			phone            TEXT NOT NULL DEFAULT '',
			whatsapp         TEXT NOT NULL DEFAULT '',
			qualification    TEXT NOT NULL DEFAULT '',
			college          TEXT NOT NULL DEFAULT '',
			graduation_year  INT,
			city             TEXT NOT NULL DEFAULT '',

			source           TEXT  NOT NULL DEFAULT 'scholarship',
			page_url         TEXT  NOT NULL DEFAULT '',
			utm              JSONB NOT NULL DEFAULT '{}',

			user_id          UUID,
			assessment_id    UUID,
			invite_id        UUID,
			-- Reserved. The authoritative link to an attempt is
			-- (assessment_id, user_id), which is already known the moment the
			-- application is written — so nothing has to write this column back
			-- after the candidate starts, and no push path can fall behind.
			attempt_id       UUID,

			-- Only the digest is stored. The raw token exists in the redirect
			-- URL and the email, and nowhere else.
			claim_token_hash TEXT UNIQUE,
			claim_expires_at TIMESTAMPTZ,
			claimed_at       TIMESTAMPTZ,

			status           TEXT NOT NULL DEFAULT 'applied'
			                 CHECK (status IN ('applied', 'invited', 'started', 'submitted',
			                                   'evaluated', 'awarded', 'rejected', 'expired')),
			award_percent    INT,
			notes            TEXT NOT NULL DEFAULT '',
			created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
		);

		-- One application per person per programme; re-applying updates in place.
		CREATE UNIQUE INDEX IF NOT EXISTS uq_scholarship_app_email_program
			ON scholarship_applications (lower(email), program_id);
		CREATE INDEX IF NOT EXISTS idx_scholarship_apps_status  ON scholarship_applications(status);
		CREATE INDEX IF NOT EXISTS idx_scholarship_apps_created ON scholarship_applications(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_scholarship_apps_program ON scholarship_applications(program_id);
	`)
	if err != nil {
		return fmt.Errorf("create scholarship tables: %w", err)
	}
	return nil
}

// ─── Routing ──────────────────────────────────────────────────────────────────

// ServeHTTP handles /api/scholarship/*. All three routes are public.
func (h *ScholarshipHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	switch strings.TrimRight(strings.TrimPrefix(r.URL.Path, "/api/scholarship"), "/") {
	case "/config":
		if r.Method != http.MethodGet {
			h.fail(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.handleConfig(w, r)
	case "/apply":
		if r.Method != http.MethodPost {
			h.fail(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.handleApply(w, r)
	case "/claim":
		if r.Method != http.MethodPost {
			h.fail(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.handleClaim(w, r)
	case "/outcome":
		if r.Method != http.MethodGet {
			h.fail(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.handleOutcome(w, r)
	default:
		h.fail(w, http.StatusNotFound, "not found")
	}
}

// ─── GET /api/scholarship/config ──────────────────────────────────────────────

type programSummary struct {
	CourseID        string `json:"courseId"`
	CourseName      string `json:"courseName"`
	Title           string `json:"title"`
	DurationMinutes int32  `json:"durationMinutes"`
	TotalMarks      int32  `json:"totalMarks"`
	SectionSummary  string `json:"sectionSummary"`
	OpensAt         string `json:"opensAt,omitempty"`
	ClosesAt        string `json:"closesAt,omitempty"`
	SeatsLeft       *int   `json:"seatsLeft,omitempty"`
	AwardSlabs      any    `json:"awardSlabs"`
}

// handleConfig lists the programmes a visitor can actually apply to right now,
// so the marketing site's course picker cannot offer a course whose paper is
// unpublished, closed or full.
func (h *ScholarshipHandler) handleConfig(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(), `
		SELECT p.course_id, p.course_name, a.title, a.duration_minutes, a.total_marks,
		       p.opens_at, p.closes_at, p.seats, p.award_slabs,
		       (SELECT COUNT(*) FROM scholarship_applications s WHERE s.program_id = p.id),
		       COALESCE((
		         SELECT string_agg(x.label, ' · ' ORDER BY x.order_index)
		         FROM (
		           SELECT s.order_index,
		                  COALESCE(s.pick_count,
		                           (SELECT COUNT(*) FROM section_questions sq WHERE sq.section_id = s.id))
		                  || ' ' || CASE s.kind WHEN 'mcq' THEN 'MCQ' ELSE initcap(s.kind) END AS label
		           FROM assessment_sections s WHERE s.assessment_id = a.id
		         ) x
		       ), '')
		FROM   scholarship_programs p
		JOIN   assessments a ON a.id = p.assessment_id
		WHERE  p.is_active
		  AND  a.status = 'published'
		  AND  (p.opens_at  IS NULL OR p.opens_at  <= now())
		  AND  (p.closes_at IS NULL OR p.closes_at >  now())
		ORDER  BY p.course_name
	`)
	if err != nil {
		h.Log.Error("list scholarship programs failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load the scholarship programmes")
		return
	}
	defer rows.Close()

	out := []programSummary{}
	for rows.Next() {
		var p programSummary
		var opensAt, closesAt *time.Time
		var seats, used int
		var slabs []byte
		if err := rows.Scan(&p.CourseID, &p.CourseName, &p.Title, &p.DurationMinutes, &p.TotalMarks,
			&opensAt, &closesAt, &seats, &slabs, &used, &p.SectionSummary); err != nil {
			h.Log.Error("scan scholarship program failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not load the scholarship programmes")
			return
		}
		if opensAt != nil {
			p.OpensAt = opensAt.UTC().Format(time.RFC3339)
		}
		if closesAt != nil {
			p.ClosesAt = closesAt.UTC().Format(time.RFC3339)
		}
		if seats > 0 {
			left := seats - used
			if left < 0 {
				left = 0
			}
			// A full programme is not offered at all rather than shown at zero.
			if left == 0 {
				continue
			}
			p.SeatsLeft = &left
		}
		p.AwardSlabs = json.RawMessage(slabs)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		h.Log.Error("iterate scholarship programs failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load the scholarship programmes")
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"programs": out})
}

// ─── POST /api/scholarship/apply ──────────────────────────────────────────────

type scholarshipInput struct {
	Name           string          `json:"name"`
	Email          string          `json:"email"`
	Phone          string          `json:"phone"`
	WhatsApp       string          `json:"whatsapp"`
	CourseID       string          `json:"courseId"`
	Qualification  string          `json:"qualification"`
	College        string          `json:"college"`
	GraduationYear int             `json:"graduationYear"`
	City           string          `json:"city"`
	PageURL        string          `json:"page_url"`
	UTM            json.RawMessage `json:"utm"`
	// Honeypot, exactly as on the enquiry form: hidden from real users, filled
	// by bots, and answered with a success shape so the bot learns nothing.
	Website string `json:"website"`
}

func (h *ScholarshipHandler) handleApply(w http.ResponseWriter, r *http.Request) {
	var in scholarshipInput
	// Cap the body before decoding — an unauthenticated endpoint should never
	// read an unbounded request.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if in.Website != "" {
		h.Log.Info("scholarship honeypot triggered", zap.String("ip", clientIP(r)))
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		return
	}

	name := clip(in.Name, 120)
	email := strings.ToLower(clip(in.Email, 200))
	courseID := clip(in.CourseID, 80)
	if name == "" || !looksLikeEmail(email) {
		h.fail(w, http.StatusBadRequest, "please provide your name and a valid email address")
		return
	}
	if courseID == "" {
		h.fail(w, http.StatusBadRequest, "please choose the course you want to apply for")
		return
	}
	// Logged separately, and loudly enough to find: a limiter turning real
	// applicants away during a campaign is invisible from the server's side
	// otherwise — it looks like nobody applied.
	if !h.ipLimiter.allow(clientIP(r)) {
		h.Log.Warn("scholarship apply rate limited by IP — raise SCHOLARSHIP_IP_LIMIT if this is a campaign",
			zap.String("ip", clientIP(r)), zap.String("email", email))
		h.fail(w, http.StatusTooManyRequests, "too many applications from this network — please try again in a few minutes")
		return
	}
	if !h.emailLimiter.allow(email) {
		h.Log.Warn("scholarship apply rate limited by email", zap.String("email", email))
		h.fail(w, http.StatusTooManyRequests, "you have applied several times already — please try again later")
		return
	}

	ctx := r.Context()

	// ── The programme, and whether it is open ────────────────────────────────
	var programID, courseName, assessmentID, assessmentTitle, assessmentStatus string
	var durationMinutes, totalMarks int32
	var opensAt, closesAt *time.Time
	var seats int
	err := h.Pool.QueryRow(ctx, `
		SELECT p.id::text, p.course_name, p.assessment_id::text, p.opens_at, p.closes_at, p.seats,
		       a.title, a.status, a.duration_minutes, a.total_marks
		FROM   scholarship_programs p
		JOIN   assessments a ON a.id = p.assessment_id
		WHERE  p.course_id = $1 AND p.is_active
	`, courseID).Scan(&programID, &courseName, &assessmentID, &opensAt, &closesAt, &seats,
		&assessmentTitle, &assessmentStatus, &durationMinutes, &totalMarks)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, http.StatusBadRequest, "that course is not open for scholarship applications right now")
		return
	}
	if err != nil {
		h.Log.Error("resolve scholarship program failed", zap.Error(err), zap.String("course_id", courseID))
		h.fail(w, http.StatusInternalServerError, "could not submit your application, please try again")
		return
	}

	now := time.Now().UTC()
	if opensAt != nil && now.Before(*opensAt) {
		h.fail(w, http.StatusBadRequest, "applications for this course open on "+opensAt.UTC().Format("2 Jan 2006"))
		return
	}
	if closesAt != nil && now.After(*closesAt) {
		h.fail(w, http.StatusBadRequest, "applications for this course have closed")
		return
	}
	// Handing out a link to an unpublished paper would strand the candidate on
	// a dead test page, so refuse before anything is written.
	if assessmentStatus != "published" {
		h.Log.Warn("scholarship program points at an unpublished assessment",
			zap.String("course_id", courseID), zap.String("assessment_id", assessmentID))
		h.fail(w, http.StatusServiceUnavailable, "the test for this course is not open yet — please try again later")
		return
	}

	// ── Has this person already been through it? ─────────────────────────────
	// The stored status is the funnel state; the attempt is the fact. Somebody
	// who has already sat the paper must be turned away even though nothing
	// wrote 'submitted' back onto their application row.
	var existingStatus, existingAttemptStatus string
	err = h.Pool.QueryRow(ctx, `
		SELECT s.status,
		       COALESCE((SELECT at.status FROM attempts at
		                  WHERE at.assessment_id = s.assessment_id AND at.user_id = s.user_id
		                  ORDER BY at.started_at DESC LIMIT 1), '')
		FROM   scholarship_applications s
		WHERE  lower(s.email) = lower($1) AND s.program_id = $2::uuid
	`, email, programID).Scan(&existingStatus, &existingAttemptStatus)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		h.Log.Error("look up existing application failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not submit your application, please try again")
		return
	}
	if terminalStatuses[existingStatus] || spentAttempt[existingAttemptStatus] {
		h.fail(w, http.StatusConflict, "you have already taken the scholarship test for this course")
		return
	}

	// Seats are checked only for genuinely new applicants — someone re-opening
	// their own application must not be turned away by a cap they already
	// occupy a place in.
	if seats > 0 && existingStatus == "" {
		var used int
		if err := h.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM scholarship_applications WHERE program_id = $1::uuid`, programID).Scan(&used); err != nil {
			h.Log.Error("count scholarship seats failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not submit your application, please try again")
			return
		}
		if used >= seats {
			h.fail(w, http.StatusConflict, "all scholarship places for this course have been taken")
			return
		}
	}

	// ── Provision, in one transaction ────────────────────────────────────────
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		h.Log.Error("begin scholarship tx failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not submit your application, please try again")
		return
	}
	// Rollback on every path that does not reach Commit; a no-op afterwards.
	defer func() { _ = tx.Rollback(context.Background()) }()

	// The account. An applicant who already has one keeps their password, their
	// role, their name and their history.
	// Overwriting the password here (as the admin bulk import deliberately
	// does) would turn a public form into an account-takeover endpoint: anyone
	// could "apply" as admin@… and be handed a fresh credential.
	// No password, rather than a random one nobody is told. The applicant sets
	// theirs through the existing reset flow; until then nothing can
	// authenticate as this account. Bcrypting a throwaway secret would have cost
	// ~100ms of CPU per application to protect something unusable — at fifty
	// applications a minute it was the slowest step in the request.
	placeholder, err := pkgauth.UnusablePassword()
	if err != nil {
		h.Log.Error("generate placeholder password failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not submit your application, please try again")
		return
	}

	// role 'applicant', not 'student'. Somebody who has asked to sit a test is
	// not enrolled in anything — they become a student when they pass and pay,
	// and that is a decision staff make, not a side effect of a form. The row
	// exists because the assessment engine keys an attempt to a user id; it is
	// the mechanism for running the test, not a place on the course.
	//
	// ON CONFLICT deliberately leaves role alone: a real student who applies for
	// a scholarship on a second course must not be demoted out of their own
	// enrolment.
	var userID string
	var isNewAccount bool
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (email, name, password, role)
		VALUES ($1, $2, $3, 'applicant')
		-- The form is public and accepts any address, so it may only rename an
		-- account it created itself; a real student's name is theirs to keep.
		ON CONFLICT (email) DO UPDATE
			SET name = CASE WHEN users.role = 'applicant' THEN EXCLUDED.name ELSE users.name END,
			    updated_at = now()
		RETURNING id::text, (xmax = 0)
	`, email, name, placeholder).Scan(&userID, &isNewAccount); err != nil {
		h.Log.Error("upsert scholarship applicant failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not submit your application, please try again")
		return
	}

	if phone := clip(in.Phone, 40); phone != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_profiles (user_id, phone)
			VALUES ($1::uuid, $2)
			ON CONFLICT (user_id) DO UPDATE SET phone = EXCLUDED.phone, updated_at = now()
		`, userID, phone); err != nil {
			h.Log.Error("upsert applicant profile failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not submit your application, please try again")
			return
		}
	}

	// The invite. This row IS the eligibility grant — assessment-service's
	// resolveInvite refuses to start a scholarship paper without it. Re-applying
	// keeps the existing token rather than rotating it, so a link already sent
	// by email keeps working.
	inviteToken, err := randomToken(24)
	if err != nil {
		h.Log.Error("generate invite token failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not submit your application, please try again")
		return
	}
	var inviteID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO assessment_invites (assessment_id, email, user_id, token, status, expires_at, sent_at)
		VALUES ($1::uuid, $2, $3::uuid, $4, 'invited', $5, now())
		ON CONFLICT (assessment_id, email) DO UPDATE
			SET user_id    = EXCLUDED.user_id,
			    expires_at = EXCLUDED.expires_at
		RETURNING id::text, token
	`, assessmentID, email, userID, inviteToken, now.Add(claimTTL)).Scan(&inviteID, &inviteToken); err != nil {
		h.Log.Error("upsert scholarship invite failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not submit your application, please try again")
		return
	}

	// The claim credential. Raw value goes to the applicant; only its digest is
	// written down.
	rawClaim, err := randomToken(32)
	if err != nil {
		h.Log.Error("generate claim token failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not submit your application, please try again")
		return
	}
	utm := []byte("{}")
	if len(in.UTM) > 0 && json.Valid(in.UTM) {
		utm = in.UTM
	}
	var gradYear *int
	if in.GraduationYear >= 1950 && in.GraduationYear <= now.Year()+10 {
		y := in.GraduationYear
		gradYear = &y
	}

	var applicationID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO scholarship_applications (
			program_id, course_id, name, email, phone, whatsapp, qualification, college,
			graduation_year, city, page_url, utm, user_id, assessment_id, invite_id,
			claim_token_hash, claim_expires_at, status
		) VALUES (
			$1::uuid, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13::uuid, $14::uuid, $15::uuid,
			$16, $17, 'applied'
		)
		ON CONFLICT (lower(email), program_id) DO UPDATE SET
			name             = EXCLUDED.name,
			phone            = EXCLUDED.phone,
			whatsapp         = EXCLUDED.whatsapp,
			qualification    = EXCLUDED.qualification,
			college          = EXCLUDED.college,
			graduation_year  = EXCLUDED.graduation_year,
			city             = EXCLUDED.city,
			page_url         = EXCLUDED.page_url,
			utm              = EXCLUDED.utm,
			user_id          = EXCLUDED.user_id,
			assessment_id    = EXCLUDED.assessment_id,
			invite_id        = EXCLUDED.invite_id,
			claim_token_hash = EXCLUDED.claim_token_hash,
			claim_expires_at = EXCLUDED.claim_expires_at,
			-- An application that lapsed or was never claimed re-opens; one that
			-- is already under way keeps its place in the flow.
			status           = CASE WHEN scholarship_applications.status IN ('applied', 'invited', 'expired')
			                        THEN 'applied' ELSE scholarship_applications.status END,
			updated_at       = now()
		RETURNING id::text
	`,
		programID, courseID, name, email, clip(in.Phone, 40), clip(in.WhatsApp, 40),
		clip(in.Qualification, 120), clip(in.College, 200), gradYear, clip(in.City, 120),
		clip(in.PageURL, 300), utm, userID, assessmentID, inviteID,
		sha256Hex(rawClaim), now.Add(claimTTL),
	).Scan(&applicationID); err != nil {
		h.Log.Error("insert scholarship application failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not submit your application, please try again")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		h.Log.Error("commit scholarship application failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not submit your application, please try again")
		return
	}

	// ── After the commit ─────────────────────────────────────────────────────

	// Mirror into the enquiry inbox so the counselling team works one list.
	// Deliberately outside the transaction and best-effort: a lead that failed
	// to copy is a reporting gap, not a reason to lose an application.
	if _, err := h.Pool.Exec(context.Background(), `
		INSERT INTO inquiries (name, email, phone, whatsapp, interest, message, source, page_url)
		VALUES ($1, $2, $3, $4, $5, $6, 'scholarship', $7)
	`, name, email, clip(in.Phone, 40), clip(in.WhatsApp, 40), courseName,
		"Scholarship application — "+courseName, clip(in.PageURL, 300)); err != nil {
		h.Log.Warn("mirror scholarship lead to inquiries failed", zap.Error(err))
	}

	testURL := h.appBase + "/scholarship/start?t=" + rawClaim

	h.Log.Info("scholarship application received",
		zap.String("email", email), zap.String("course_id", courseID),
		zap.Bool("new_account", isNewAccount))

	// Fire-and-forget, for the same reason the enquiry alert is: a slow SMTP
	// server must not delay, or fail, the applicant's submission.
	h.mailer.notifyApplicant(name, email, courseName, assessmentTitle, testURL, durationMinutes, totalMarks, isNewAccount)
	h.mailer.notifyStaff(name, email, clip(in.Phone, 40), courseName, clip(in.College, 200))

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":       true,
		"applicationId": applicationID,
		"testUrl":       testURL,
		"assessment": map[string]any{
			"title":           assessmentTitle,
			"durationMinutes": durationMinutes,
			"totalMarks":      totalMarks,
		},
	})
}

// ─── POST /api/scholarship/claim ──────────────────────────────────────────────

func (h *ScholarshipHandler) handleClaim(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	raw := clip(body.Token, 200)
	if raw == "" {
		h.fail(w, http.StatusBadRequest, "this link is missing its access token")
		return
	}
	// A claim is a credential check, so it gets its own tighter budget than the
	// application form's.
	if !h.ipLimiter.allow("claim:" + clientIP(r)) {
		h.Log.Warn("scholarship claim rate limited by IP", zap.String("ip", clientIP(r)))
		h.fail(w, http.StatusTooManyRequests, "too many attempts from this network — please try again in a few minutes")
		return
	}

	ctx := r.Context()

	var appID, status, userID, assessmentID, courseID, inviteToken string
	var email, userName, role string
	var expiresAt *time.Time
	err := h.Pool.QueryRow(ctx, `
		SELECT s.id::text, s.status, s.user_id::text, s.assessment_id::text, s.course_id,
		       s.claim_expires_at, COALESCE(i.token, ''), u.email, u.name, u.role
		FROM   scholarship_applications s
		JOIN   users u ON u.id = s.user_id
		LEFT   JOIN assessment_invites i ON i.id = s.invite_id
		WHERE  s.claim_token_hash = $1
	`, sha256Hex(raw)).Scan(&appID, &status, &userID, &assessmentID, &courseID,
		&expiresAt, &inviteToken, &email, &userName, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		// Same answer for a token that never existed and one that was replaced,
		// so this cannot be used to probe for live links.
		h.fail(w, http.StatusUnauthorized, "this link is no longer valid — please apply again to get a new one")
		return
	}
	if err != nil {
		h.Log.Error("look up claim token failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not open your test, please try again")
		return
	}

	if expiresAt != nil && time.Now().UTC().After(*expiresAt) {
		// Record the lapse so the admin list shows why a candidate went quiet.
		if _, err := h.Pool.Exec(ctx, `
			UPDATE scholarship_applications
			SET    status = 'expired', updated_at = now()
			WHERE  id = $1::uuid AND status IN ('applied', 'invited')
		`, appID); err != nil {
			h.Log.Warn("mark application expired failed", zap.Error(err))
		}
		h.fail(w, http.StatusUnauthorized, "this link has expired — please apply again to get a new one")
		return
	}
	if terminalStatuses[status] {
		h.fail(w, http.StatusConflict, "you have already completed this test")
		return
	}
	// A claim link is handed to whoever filled in the public form, and that
	// form accepts any email address. So it may only ever open an account the
	// funnel itself created. Minting a session for an existing student, admin
	// or recruiter here let anyone "apply" as that person and be signed in as
	// them. Those people already have a password; their invite still stands,
	// so they sign in normally and find the test in their list.
	if role != "applicant" {
		h.Log.Warn("scholarship claim refused for existing account",
			zap.String("email", email), zap.String("role", role))
		h.fail(w, http.StatusConflict,
			"you already have a Knovate account — sign in with your password to take this test (use \"Forgot password\" if you need to set one)")
		return
	}

	// Deliberately not single-use. A candidate who refreshes the redirect page
	// before the session reaches localStorage would otherwise be locked out of
	// their own test. The link stays usable until it expires or the attempt is
	// submitted; claimed_at records the first use.
	if _, err := h.Pool.Exec(ctx, `
		UPDATE scholarship_applications
		SET    claimed_at = COALESCE(claimed_at, now()),
		       status     = CASE WHEN status = 'applied' THEN 'invited' ELSE status END,
		       updated_at = now()
		WHERE  id = $1::uuid
	`, appID); err != nil {
		h.Log.Error("mark application claimed failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not open your test, please try again")
		return
	}

	// The same signer, claims and lifetime as /api/login — once claimed, this is
	// an ordinary student session with no special powers.
	token, err := pkgauth.GenerateToken(userID, email, role, h.jwtSecret, 24*time.Hour)
	if err != nil {
		h.Log.Error("mint scholarship session failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not open your test, please try again")
		return
	}

	h.Log.Info("scholarship claim accepted", zap.String("email", email), zap.String("course_id", courseID))

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token": token,
		"user": map[string]any{
			"id": userID, "email": email, "name": userName, "role": role,
		},
		"assessmentId":  assessmentID,
		"inviteToken":   inviteToken,
		"courseId":      courseID,
		"applicationId": appID,
	})
}

// ─── GET /api/scholarship/outcome?attemptId= ──────────────────────────────────

// awardSlab is one band of the programme's ladder: score at or above
// MinPercent earns AwardPercent off the fee.
type awardSlab struct {
	MinPercent   float64 `json:"minPercent"`
	AwardPercent int     `json:"awardPercent"`
}

// bandFor returns the best band a percentage qualifies for, and whether it
// qualified at all.
//
// Slabs are sorted here rather than trusted to arrive in order — they are
// operator-entered JSON, and a ladder listed low-to-high would otherwise award
// the *smallest* matching discount to the strongest candidate.
func bandFor(slabs []awardSlab, percent float64) (awardSlab, bool) {
	sort.Slice(slabs, func(i, j int) bool { return slabs[i].MinPercent > slabs[j].MinPercent })
	for _, s := range slabs {
		if percent >= s.MinPercent {
			return s, true
		}
	}
	return awardSlab{}, false
}

// handleOutcome tells a candidate that their paper arrived. It deliberately
// does not tell them how they did.
//
// A scholarship result is a fee decision, not a test score. It is reviewed
// against the award ladder, staff can adjust it, and it is delivered by email
// along with what happens next. This endpoint used to hand back the percentage,
// the band and the whole ladder the moment the paper was submitted, which
// pre-empted that decision — and because coding answers grade asynchronously it
// frequently showed a confident zero to somebody who had in fact passed.
//
// The score is still computed and stored; it is read by staff through the
// applications list, which is a different path with a different audience.
func (h *ScholarshipHandler) handleOutcome(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		h.fail(w, http.StatusUnauthorized, "authentication required")
		return
	}
	attemptID := clip(r.URL.Query().Get("attemptId"), 60)
	if attemptID == "" {
		h.fail(w, http.StatusBadRequest, "attemptId is required")
		return
	}

	var courseName, appStatus, contactEmail string
	err := h.Pool.QueryRow(r.Context(), `
		SELECT p.course_name, s.status, s.email
		FROM   scholarship_applications s
		JOIN   scholarship_programs p ON p.id = s.program_id
		JOIN   attempts at ON at.assessment_id = s.assessment_id AND at.user_id = s.user_id
		WHERE  at.id = $1::uuid AND s.user_id = $2::uuid
	`, attemptID, userID).Scan(&courseName, &appStatus, &contactEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		// Not an error worth alarming anyone about: most attempts are practice
		// or hiring, and have no scholarship attached.
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"isScholarship": false})
		return
	}
	if err != nil {
		h.Log.Error("load scholarship outcome failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load your scholarship result")
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"isScholarship": true,
		"withheld":      true,
		"courseName":    courseName,
		"status":        appStatus,
		// Echoed so the page can say "we will write to you at <address>" —
		// which is also the moment a candidate notices they typed it wrong.
		"email": contactEmail,
	})
}

// ─── Admin surface ────────────────────────────────────────────────────────────

type scholarshipRow struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Email          string   `json:"email"`
	Phone          string   `json:"phone"`
	CourseID       string   `json:"course_id"`
	CourseName     string   `json:"course_name"`
	Qualification  string   `json:"qualification"`
	College        string   `json:"college"`
	GraduationYear *int     `json:"graduation_year"`
	City           string   `json:"city"`
	Status         string   `json:"status"`
	AttemptID      string   `json:"attempt_id"`
	Score          *float64 `json:"score"`
	MaxScore       *float64 `json:"max_score"`
	Percentage     *float64 `json:"percentage"`
	AwardPercent   *int     `json:"award_percent"`
	Notes          string   `json:"notes"`
	CreatedAt      string   `json:"created_at"`
}

// applicationQuery builds the shared CTE and filter clause behind both the
// admin table and its CSV export.
//
// It is one function on purpose: an export that honoured different filters from
// the screen it was launched from would be a quietly wrong spreadsheet, which is
// worse than a missing one. The CTE also feeds the COUNT, so a status filter
// cannot mean one thing when counting and another when listing.
func applicationQuery(q url.Values) (base, where string, args []any) {
	base = `
		WITH app AS (
			SELECT s.id, s.name, s.email, s.phone, s.course_id, p.course_name,
			       s.qualification, s.college, s.graduation_year, s.city,
			       s.award_percent, s.notes, s.created_at,
			       at.id AS attempt_id, at.score, at.max_score,
			       CASE
			         WHEN s.status IN ('awarded', 'rejected')        THEN s.status
			         WHEN at.status = 'evaluated'                    THEN 'evaluated'
			         WHEN at.status IN ('submitted', 'evaluating')   THEN 'submitted'
			         WHEN at.status = 'disqualified'                 THEN 'rejected'
			         WHEN at.status = 'expired'                      THEN 'expired'
			         WHEN at.status = 'in_progress'                  THEN 'started'
			         ELSE s.status
			       END AS status
			FROM   scholarship_applications s
			JOIN   scholarship_programs p ON p.id = s.program_id
			LEFT   JOIN LATERAL (
			         SELECT at.id, at.status, at.score, at.max_score
			         FROM   attempts at
			         WHERE  at.assessment_id = s.assessment_id
			           AND  at.user_id       = s.user_id
			         ORDER  BY at.started_at DESC
			         LIMIT  1
			       ) at ON true
		)
	`

	var clauses []string
	if v := q.Get("status"); v != "" {
		args = append(args, v)
		clauses = append(clauses, fmt.Sprintf("status = $%d", len(args)))
	}
	if v := q.Get("courseId"); v != "" {
		args = append(args, v)
		clauses = append(clauses, fmt.Sprintf("course_id = $%d", len(args)))
	}
	if v := q.Get("search"); v != "" {
		args = append(args, v)
		clauses = append(clauses, fmt.Sprintf(
			"(name ILIKE '%%' || $%d || '%%' OR email ILIKE '%%' || $%d || '%%')", len(args), len(args)))
	}
	// Percentage is derived, so it is filtered here rather than in the CTE.
	if v := q.Get("minPercent"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			args = append(args, n)
			clauses = append(clauses, fmt.Sprintf(
				"(max_score > 0 AND (score / max_score) * 100 >= $%d)", len(args)))
		}
	}
	if d, err := time.Parse("2006-01-02", q.Get("from")); err == nil {
		args = append(args, d)
		clauses = append(clauses, fmt.Sprintf("created_at >= $%d", len(args)))
	}
	if d, err := time.Parse("2006-01-02", q.Get("to")); err == nil {
		args = append(args, d.AddDate(0, 0, 1))
		clauses = append(clauses, fmt.Sprintf("created_at < $%d", len(args)))
	}
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	return base, where, args
}

// ListApplications backs GET /api/admin/scholarships.
//
// Both the score and the displayed status are derived from the candidate's
// attempt rather than copied onto the application row. Coding questions grade
// asynchronously, so any cached copy would be stale exactly when staff are
// watching a drive — and a write-back path would be one more thing to fall
// behind. The attempt is found by (assessment_id, user_id), which the
// application already knows.
//
// Only the award decision (awarded / rejected) is genuinely stored, because
// only a human can make it.
func (h *ScholarshipHandler) ListApplications(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}

	base, where, args := applicationQuery(q)
	var total int
	if err := h.Pool.QueryRow(r.Context(),
		base+"SELECT COUNT(*) FROM app "+where, args...).Scan(&total); err != nil {
		h.Log.Error("count scholarship applications failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load applications")
		return
	}

	rows, err := h.Pool.Query(r.Context(), fmt.Sprintf(`%s
		SELECT id::text, name, email, phone, course_id, course_name,
		       qualification, college, graduation_year, city, status,
		       COALESCE(attempt_id::text, ''), score, max_score,
		       award_percent, notes, created_at
		FROM   app
		%s
		ORDER  BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, base, where, len(args)+1, len(args)+2), append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		h.Log.Error("list scholarship applications failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load applications")
		return
	}
	defer rows.Close()

	out := []scholarshipRow{}
	for rows.Next() {
		var a scholarshipRow
		var created time.Time
		if err := rows.Scan(&a.ID, &a.Name, &a.Email, &a.Phone, &a.CourseID, &a.CourseName,
			&a.Qualification, &a.College, &a.GraduationYear, &a.City, &a.Status,
			&a.AttemptID, &a.Score, &a.MaxScore, &a.AwardPercent, &a.Notes, &created); err != nil {
			h.Log.Error("scan scholarship application failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not load applications")
			return
		}
		if a.Score != nil && a.MaxScore != nil && *a.MaxScore > 0 {
			pct := (*a.Score / *a.MaxScore) * 100
			a.Percentage = &pct
		}
		a.CreatedAt = created.Format(time.RFC3339)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		h.Log.Error("iterate scholarship applications failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load applications")
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"applications": out, "total": total, "page": page, "pageSize": pageSize,
	})
}

// ExportApplications backs GET /api/admin/scholarships/export.csv.
//
// It honours exactly the filters the screen was showing, via the same
// applicationQuery the table uses, and is deliberately unpaginated: a
// counsellor exporting "everyone above 65%" wants everyone, not the first
// fifty. Rows stream straight to the response rather than being buffered.
func (h *ScholarshipHandler) ExportApplications(w http.ResponseWriter, r *http.Request) {
	base, where, args := applicationQuery(r.URL.Query())

	rows, err := h.Pool.Query(r.Context(), fmt.Sprintf(`%s
		SELECT name, email, phone, course_name, qualification, college,
		       graduation_year, city, status, score, max_score, award_percent,
		       notes, created_at
		FROM   app
		%s
		ORDER  BY created_at DESC
	`, base, where), args...)
	if err != nil {
		h.Log.Error("export scholarship applications failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not export applications")
		return
	}
	defer rows.Close()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%q", "scholarship-applications.csv"))

	cw := csv.NewWriter(w)
	defer cw.Flush()
	_ = cw.Write([]string{
		"Name", "Email", "Phone", "Course", "Qualification", "College",
		"Graduation year", "City", "Status", "Score", "Out of", "Percentage",
		"Award %", "Notes", "Applied",
	})

	for rows.Next() {
		var name, email, phone, course, qual, college, city, status, notes string
		var gradYear *int
		var score, maxScore *float64
		var award *int
		var created time.Time
		if err := rows.Scan(&name, &email, &phone, &course, &qual, &college,
			&gradYear, &city, &status, &score, &maxScore, &award, &notes, &created); err != nil {
			h.Log.Error("scan scholarship export row failed", zap.Error(err))
			return // headers are already sent; truncation beats a half-JSON error
		}

		pct := ""
		if score != nil && maxScore != nil && *maxScore > 0 {
			pct = strconv.FormatFloat((*score / *maxScore)*100, 'f', 1, 64)
		}
		_ = cw.Write([]string{
			name, email, phone, course, qual, college,
			intOrBlank(gradYear), city, status,
			floatOrBlank(score), floatOrBlank(maxScore), pct,
			intOrBlank(award), notes, created.Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		h.Log.Error("iterate scholarship export failed", zap.Error(err))
	}
}

func intOrBlank(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}

func floatOrBlank(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', -1, 64)
}

// UpdateApplication backs PATCH /api/admin/scholarships/{id} — the award decision.
func (h *ScholarshipHandler) UpdateApplication(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		Status       string `json:"status"`
		AwardPercent *int   `json:"award_percent"`
		Notes        string `json:"notes"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Only the decision states are settable by hand. The funnel states are
	// owned by the candidate's own progress through the flow, and letting an
	// admin write them would desynchronise the application from its attempt.
	switch body.Status {
	case "awarded", "rejected", "evaluated", "":
	default:
		h.fail(w, http.StatusBadRequest, "status must be evaluated, awarded or rejected")
		return
	}
	if body.AwardPercent != nil && (*body.AwardPercent < 0 || *body.AwardPercent > 100) {
		h.fail(w, http.StatusBadRequest, "award percent must be between 0 and 100")
		return
	}

	// RETURNING gives us what the award email needs without a second read, and
	// tells us whether the row existed at all.
	var name, email, courseName string
	var finalStatus string
	var finalAward *int
	err := h.Pool.QueryRow(r.Context(), `
		UPDATE scholarship_applications s
		SET    status        = COALESCE(NULLIF($2, ''), s.status),
		       award_percent = COALESCE($3, s.award_percent),
		       notes         = $4,
		       updated_at    = now()
		FROM   scholarship_programs p
		WHERE  s.id = $1::uuid AND p.id = s.program_id
		RETURNING s.name, s.email, p.course_name, s.status, s.award_percent
	`, id, body.Status, body.AwardPercent, clip(body.Notes, 2000)).
		Scan(&name, &email, &courseName, &finalStatus, &finalAward)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, http.StatusNotFound, "application not found")
		return
	}
	if err != nil {
		h.Log.Error("update scholarship application failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not update the application")
		return
	}

	// Only on the transition an admin actually made, and only when there is a
	// figure to quote — "you have been awarded a 0% scholarship" helps nobody.
	if body.Status == "awarded" && finalAward != nil && *finalAward > 0 {
		h.mailer.notifyAward(name, email, courseName, *finalAward)
	}

	h.ok(w)
}

// DeleteApplication backs DELETE /api/admin/scholarships/{id}.
//
// It removes the application, the invitation that admits them and any attempt
// they made at that paper — the three things that together are "this person is
// in the drive". They have no foreign keys between them, so deleting only the
// application would leave an invite that still lets the candidate start and an
// attempt that still counts against max_attempts.
//
// The account is a separate question. A scholarship applicant is often a real
// student who also has courses, so this only removes the user when the funnel
// itself created them and they have nothing else: no courses, no other
// application, and a password that was never set. Anything else is somebody's
// account, not a by-product of this row.
func (h *ScholarshipHandler) DeleteApplication(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		h.Log.Error("begin delete application failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not delete the application")
		return
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	email, accountRemoved, err := h.deleteApplicationTx(ctx, tx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, http.StatusNotFound, "application not found")
		return
	} else if err != nil {
		h.Log.Error("delete application failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not delete the application")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		h.Log.Error("commit delete application failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not delete the application")
		return
	}

	h.Log.Info("scholarship application deleted",
		zap.String("email", email), zap.Bool("account_removed", accountRemoved))

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":        true,
		"email":          email,
		"accountRemoved": accountRemoved,
	})
}

// deleteApplicationTx removes one application with everything the funnel
// created for it: the invite, the attempts, and the applicant account when
// nothing else depends on it. It runs inside the caller's transaction so a bulk
// delete is all-or-nothing. Returns pgx.ErrNoRows when the id does not exist.
func (h *ScholarshipHandler) deleteApplicationTx(ctx context.Context, tx pgx.Tx, id string) (email string, accountRemoved bool, err error) {
	var userID, assessmentID string
	if err = tx.QueryRow(ctx, `
		SELECT email, COALESCE(user_id::text, ''), COALESCE(assessment_id::text, '')
		FROM   scholarship_applications WHERE id = $1::uuid
	`, id).Scan(&email, &userID, &assessmentID); err != nil {
		return "", false, err
	}

	if _, err = tx.Exec(ctx, `DELETE FROM scholarship_applications WHERE id = $1::uuid`, id); err != nil {
		return "", false, fmt.Errorf("delete application: %w", err)
	}

	if assessmentID != "" {
		if _, err = tx.Exec(ctx, `
			DELETE FROM assessment_invites WHERE assessment_id = $1::uuid AND lower(email) = lower($2)
		`, assessmentID, email); err != nil {
			return "", false, fmt.Errorf("delete invite: %w", err)
		}
	}
	if assessmentID != "" && userID != "" {
		if _, err = tx.Exec(ctx, `
			DELETE FROM attempts WHERE assessment_id = $1::uuid AND user_id = $2::uuid
		`, assessmentID, userID); err != nil {
			return "", false, fmt.Errorf("delete attempts: %w", err)
		}
	}

	// Only an account this funnel created and nothing else depends on.
	// user_profiles, user_courses and the rest cascade from users.
	if userID != "" {
		tag, err := tx.Exec(ctx, `
			DELETE FROM users u
			 WHERE u.id = $1::uuid
			   AND u.role = 'applicant'
			   AND u.password LIKE '$2a$unusable$%'
			   AND NOT EXISTS (SELECT 1 FROM user_courses c WHERE c.user_id = u.id)
			   AND NOT EXISTS (SELECT 1 FROM scholarship_applications s WHERE s.user_id = u.id)
			   AND NOT EXISTS (SELECT 1 FROM attempts a WHERE a.user_id = u.id)
		`, userID)
		if err != nil {
			return "", false, fmt.Errorf("delete provisioned account: %w", err)
		}
		accountRemoved = tag.RowsAffected() > 0
	}
	return email, accountRemoved, nil
}

// ResendLink backs POST /api/admin/scholarships/{id}/resend.
//
// The email is the only route into the test, which makes a bounced or deleted
// message a dead end: without this, staff could only tell the candidate to
// apply again and hope. It issues a *fresh* token with a fresh window rather
// than resending the old one, so it fixes an expired link as well as a lost
// one — and the old link stops working, which is the correct behaviour for a
// reissued credential.
//
// The new link is returned to the admin as well as emailed. When the problem is
// delivery itself — a typo'd domain, a school mail server bouncing us — a
// counsellor with the link in front of them can read it out or send it another
// way, instead of the candidate simply never sitting the test.
func (h *ScholarshipHandler) ResendLink(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()

	var name, email, courseName, assessmentID, assessmentTitle, assessmentStatus, appStatus string
	var durationMinutes, totalMarks int32
	var attemptStatus string
	err := h.Pool.QueryRow(ctx, `
		SELECT s.name, s.email, p.course_name, s.assessment_id::text, s.status,
		       a.title, a.status, a.duration_minutes, a.total_marks,
		       COALESCE((SELECT at.status FROM attempts at
		                  WHERE at.assessment_id = s.assessment_id AND at.user_id = s.user_id
		                  ORDER BY at.started_at DESC LIMIT 1), '')
		FROM   scholarship_applications s
		JOIN   scholarship_programs p ON p.id = s.program_id
		JOIN   assessments a ON a.id = s.assessment_id
		WHERE  s.id = $1::uuid
	`, id).Scan(&name, &email, &courseName, &assessmentID, &appStatus,
		&assessmentTitle, &assessmentStatus, &durationMinutes, &totalMarks, &attemptStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, http.StatusNotFound, "application not found")
		return
	}
	if err != nil {
		h.Log.Error("load application for resend failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not resend the link")
		return
	}

	// A candidate who has already sat the paper has nothing to come back to,
	// and a link implying otherwise would only confuse them.
	if terminalStatuses[appStatus] || spentAttempt[attemptStatus] {
		h.fail(w, http.StatusConflict, "this candidate has already taken the test")
		return
	}
	if assessmentStatus != "published" {
		h.fail(w, http.StatusConflict, "the paper for this course is not published, so the link would not work")
		return
	}

	raw, err := randomToken(32)
	if err != nil {
		h.Log.Error("generate resend token failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not resend the link")
		return
	}
	expires := time.Now().UTC().Add(claimTTL)

	if _, err := h.Pool.Exec(ctx, `
		UPDATE scholarship_applications
		SET    claim_token_hash = $2,
		       claim_expires_at = $3,
		       -- Back to 'invited' if it had lapsed: the candidate can sit it again.
		       status           = CASE WHEN status = 'expired' THEN 'invited' ELSE status END,
		       updated_at       = now()
		WHERE  id = $1::uuid
	`, id, sha256Hex(raw), expires); err != nil {
		h.Log.Error("store resent token failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not resend the link")
		return
	}

	// The invite is what actually admits them; extend it alongside the link so a
	// lapsed invitation cannot outlive the token that points at it.
	if _, err := h.Pool.Exec(ctx, `
		UPDATE assessment_invites SET expires_at = $3
		WHERE  assessment_id = $1::uuid AND lower(email) = lower($2)
	`, assessmentID, email, expires); err != nil {
		h.Log.Warn("extend invite on resend failed", zap.Error(err))
	}

	testURL := h.appBase + "/scholarship/start?t=" + raw
	h.Log.Info("scholarship link resent", zap.String("email", email), zap.String("application_id", id))

	// Sent synchronously, unlike the applicant's own submission. Somebody is
	// watching this button and acting on what it says; reporting "emailed" for
	// a message the relay refused sends a counsellor away believing a candidate
	// has their link when they do not. The link itself is still returned either
	// way, so a failed send leaves them something to pass on by hand.
	sendErr := h.mailer.deliverApplicant(name, email, courseName, assessmentTitle, testURL,
		durationMinutes, totalMarks, false)

	w.WriteHeader(http.StatusOK)
	body := map[string]any{
		"success": sendErr == nil,
		"email":   email,
		"testUrl": testURL,
		"expires": expires.Format(time.RFC3339),
	}
	if sendErr != nil {
		body["emailError"] = sendErr.Error()
	}
	_ = json.NewEncoder(w).Encode(body)
}

type programRow struct {
	ID           string          `json:"id"`
	CourseID     string          `json:"course_id"`
	CourseName   string          `json:"course_name"`
	AssessmentID string          `json:"assessment_id"`
	Title        string          `json:"assessment_title"`
	Status       string          `json:"assessment_status"`
	IsActive     bool            `json:"is_active"`
	OpensAt      string          `json:"opens_at,omitempty"`
	ClosesAt     string          `json:"closes_at,omitempty"`
	Seats        int             `json:"seats"`
	Used         int             `json:"used"`
	AwardSlabs   json.RawMessage `json:"award_slabs"`
}

// ListPrograms backs GET /api/admin/scholarship-programs.
func (h *ScholarshipHandler) ListPrograms(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(), `
		SELECT p.id::text, p.course_id, p.course_name, p.assessment_id::text,
		       COALESCE(a.title, ''), COALESCE(a.status, 'missing'),
		       p.is_active, p.opens_at, p.closes_at, p.seats, p.award_slabs,
		       (SELECT COUNT(*) FROM scholarship_applications s WHERE s.program_id = p.id)
		FROM   scholarship_programs p
		LEFT   JOIN assessments a ON a.id = p.assessment_id
		ORDER  BY p.course_name
	`)
	if err != nil {
		h.Log.Error("list scholarship programs failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load programmes")
		return
	}
	defer rows.Close()

	out := []programRow{}
	for rows.Next() {
		var p programRow
		var opensAt, closesAt *time.Time
		var slabs []byte
		if err := rows.Scan(&p.ID, &p.CourseID, &p.CourseName, &p.AssessmentID, &p.Title, &p.Status,
			&p.IsActive, &opensAt, &closesAt, &p.Seats, &slabs, &p.Used); err != nil {
			h.Log.Error("scan scholarship program failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not load programmes")
			return
		}
		if opensAt != nil {
			p.OpensAt = opensAt.UTC().Format(time.RFC3339)
		}
		if closesAt != nil {
			p.ClosesAt = closesAt.UTC().Format(time.RFC3339)
		}
		p.AwardSlabs = json.RawMessage(slabs)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		h.Log.Error("iterate scholarship programs failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load programmes")
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"programs": out})
}

// UpsertProgram backs POST /api/admin/scholarship-programs — the course-to-paper
// mapping, keyed on course_id so ops can repoint a course without a deploy.
//
// Optional fields are *absent-preserving*, not defaulting. That distinction is
// load-bearing: a caller that sends only {course_id, course_name, assessment_id,
// is_active:false} to retire a programme must not thereby erase its award
// ladder — and an erased ladder does not fail loudly, it quietly tells a
// candidate who scored 92% that they earned nothing. Absent means "leave it";
// an explicitly empty date means "clear it".
func (h *ScholarshipHandler) UpsertProgram(w http.ResponseWriter, r *http.Request) {
	var body struct {
		CourseID     string          `json:"course_id"`
		CourseName   string          `json:"course_name"`
		AssessmentID string          `json:"assessment_id"`
		IsActive     *bool           `json:"is_active"`
		OpensAt      *string         `json:"opens_at"`
		ClosesAt     *string         `json:"closes_at"`
		Seats        *int            `json:"seats"`
		AwardSlabs   json.RawMessage `json:"award_slabs"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	courseID := clip(body.CourseID, 80)
	courseName := clip(body.CourseName, 160)
	assessmentID := clip(body.AssessmentID, 60)
	if courseID == "" || courseName == "" || assessmentID == "" {
		h.fail(w, http.StatusBadRequest, "course_id, course_name and assessment_id are required")
		return
	}

	// Refuse to point a programme at a paper that is not invite-gated: a
	// practice test ignores invites entirely, so mapping one here would publish
	// an open scholarship exam.
	var purpose string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT purpose FROM assessments WHERE id = $1::uuid`, assessmentID).Scan(&purpose)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, http.StatusBadRequest, "no assessment with that id")
		return
	}
	if err != nil {
		h.Log.Error("look up assessment purpose failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not save the programme")
		return
	}
	if purpose != "scholarship" && purpose != "hiring" {
		h.fail(w, http.StatusBadRequest,
			"that assessment is a practice test — a scholarship paper must have purpose 'scholarship' so it stays invite-only")
		return
	}

	// nil for every optional field the caller left out; the SQL below then
	// keeps whatever is already stored.
	var slabs any
	if len(body.AwardSlabs) > 0 && json.Valid(body.AwardSlabs) {
		slabs = []byte(body.AwardSlabs)
	}
	var opensAt, closesAt *time.Time
	if body.OpensAt != nil {
		opensAt = parseOptionalTime(*body.OpensAt)
	}
	if body.ClosesAt != nil {
		closesAt = parseOptionalTime(*body.ClosesAt)
	}
	if body.Seats != nil && *body.Seats < 0 {
		zero := 0
		body.Seats = &zero
	}

	var id string
	if err := h.Pool.QueryRow(r.Context(), `
		INSERT INTO scholarship_programs
			(course_id, course_name, assessment_id, is_active, opens_at, closes_at, seats, award_slabs)
		VALUES ($1, $2, $3::uuid, COALESCE($4, true), $5, $6, COALESCE($7, 0),
		        COALESCE($8::jsonb, '[]'::jsonb))
		ON CONFLICT (course_id) DO UPDATE SET
			course_name   = EXCLUDED.course_name,
			assessment_id = EXCLUDED.assessment_id,
			is_active     = COALESCE($4, scholarship_programs.is_active),
			-- $9/$10 say whether the caller mentioned the field at all, which is
			-- what lets an explicit "" clear a date while absence preserves it.
			opens_at      = CASE WHEN $9  THEN $5 ELSE scholarship_programs.opens_at  END,
			closes_at     = CASE WHEN $10 THEN $6 ELSE scholarship_programs.closes_at END,
			seats         = COALESCE($7, scholarship_programs.seats),
			award_slabs   = COALESCE($8::jsonb, scholarship_programs.award_slabs),
			updated_at    = now()
		RETURNING id::text
	`, courseID, courseName, assessmentID, body.IsActive, opensAt, closesAt, body.Seats, slabs,
		body.OpensAt != nil, body.ClosesAt != nil).Scan(&id); err != nil {
		h.Log.Error("upsert scholarship program failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not save the programme")
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "id": id})
}

// ─── Expiry sweeper ───────────────────────────────────────────────────────────

// StartExpirySweeper retires applications whose claim link lapsed without ever
// being used, until ctx is cancelled.
//
// handleClaim already marks a lapsed application expired, but only if somebody
// turns up with the link. Most people who let it lapse simply never come back —
// so without this they would sit in the admin list as "invited" forever, and
// staff chasing warm leads could not tell the difference between someone who is
// about to sit the test and someone who walked away three weeks ago.
func (h *ScholarshipHandler) StartExpirySweeper(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		h.Log.Info("scholarship expiry sweeper started", zap.Duration("interval", interval))

		for {
			select {
			case <-ctx.Done():
				h.Log.Info("scholarship expiry sweeper stopped")
				return
			case <-ticker.C:
				sweepCtx, cancel := context.WithTimeout(ctx, interval)
				n, err := h.ExpireStaleApplications(sweepCtx)
				cancel()
				if err != nil {
					h.Log.Error("scholarship expiry sweep failed", zap.Error(err))
					continue
				}
				if n > 0 {
					h.Log.Info("retired lapsed scholarship applications", zap.Int64("count", n))
				}
			}
		}
	}()
}

// ExpireStaleApplications marks unclaimed, lapsed applications expired.
//
// The NOT EXISTS guard is the important part: somebody who started their paper
// is mid-exam or already finished, and their claim link lapsing afterwards says
// nothing about them. Expiring those would misreport candidates who are
// actually sitting the test.
func (h *ScholarshipHandler) ExpireStaleApplications(ctx context.Context) (int64, error) {
	tag, err := h.Pool.Exec(ctx, `
		UPDATE scholarship_applications s
		SET    status = 'expired', updated_at = now()
		WHERE  s.status IN ('applied', 'invited')
		  AND  s.claim_expires_at IS NOT NULL
		  AND  s.claim_expires_at < now()
		  AND  NOT EXISTS (
		         SELECT 1 FROM attempts at
		         WHERE  at.assessment_id = s.assessment_id
		           AND  at.user_id       = s.user_id
		       )
	`)
	if err != nil {
		return 0, fmt.Errorf("expire stale applications: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// envInt reads a positive integer setting, falling back when unset or nonsense.
// A misconfigured limit should degrade to the default rather than to zero,
// which would refuse every request.
func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func envMinutes(key string, fallback int) time.Duration {
	return time.Duration(envInt(key, fallback)) * time.Minute
}

// randomToken returns a URL-safe token from crypto/rand. math/rand would be
// predictable, and these tokens are credentials.
func randomToken(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// parseOptionalTime accepts RFC3339 and returns nil for anything blank or
// unparseable, so an empty field in the admin form clears the window rather
// than failing the save.
func parseOptionalTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}

func (h *ScholarshipHandler) ok(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

func (h *ScholarshipHandler) fail(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// EnrolApplicant promotes an applicant to a student and grants them the course.
//
// This is the only way somebody crosses from "sat our test" to "is on the
// course", and it is deliberately a button a human presses. Applying does not
// enrol anyone; passing does not enrol anyone. Staff enrol them once the fee is
// settled, and that judgement has no reliable signal in this system — there is
// no payment record here to key it off.
//
// Both writes are in one transaction: a role that says student with no course
// attached would show up in the Users list as a phantom enrolment.
func (h *ScholarshipHandler) EnrolApplicant(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		h.fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id = clip(id, 60)
	if id == "" {
		h.fail(w, http.StatusBadRequest, "application id is required")
		return
	}

	ctx := r.Context()
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		h.Log.Error("begin enrol failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not enrol this applicant")
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID, email, courseID, courseName string
	err = tx.QueryRow(ctx, `
		SELECT s.user_id::text, s.email, p.course_id, p.course_name
		FROM   scholarship_applications s
		JOIN   scholarship_programs p ON p.id = s.program_id
		WHERE  s.id = $1::uuid
	`, id).Scan(&userID, &email, &courseID, &courseName)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, http.StatusNotFound, "application not found")
		return
	}
	if err != nil {
		h.Log.Error("load application for enrol failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not enrol this applicant")
		return
	}

	// Only promote out of 'applicant'. An admin or recruiter who once applied
	// for a scholarship must not be demoted to student by this button.
	if _, err := tx.Exec(ctx, `
		UPDATE users SET role = 'student', updated_at = now()
		 WHERE id = $1::uuid AND role = 'applicant'
	`, userID); err != nil {
		h.Log.Error("promote applicant failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not enrol this applicant")
		return
	}

	// Idempotent: enrolling twice is a double-click, not an error.
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_courses (user_id, course_id)
		VALUES ($1::uuid, $2)
		ON CONFLICT DO NOTHING
	`, userID, courseID); err != nil {
		h.Log.Error("grant course failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not enrol this applicant")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		h.Log.Error("commit enrol failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not enrol this applicant")
		return
	}

	h.Log.Info("scholarship applicant enrolled",
		zap.String("email", email), zap.String("course_id", courseID))

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true, "email": email, "courseName": courseName,
	})
}

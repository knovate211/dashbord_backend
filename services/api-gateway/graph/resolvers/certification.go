package resolvers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
	pkgauth "github.com/knovate211/pkg/auth"
)

// CertificationHandler sells and runs paid certification exams.
//
//	GET  /api/certification/config              — exams currently on sale
//	POST /api/certification/order               — register + create a Razorpay order
//	POST /api/certification/verify              — browser callback after checkout
//	POST /api/certification/webhook             — Razorpay server-to-server confirmation
//	POST /api/certification/claim               — exchange the emailed link for a session
//	GET  /api/certification/outcome?attemptId=  — pass/fail once grading settles
//	GET  /api/certification/mine                — the signed-in learner's certificates
//	GET  /api/certification/credential?id=      — public credential verification
//
// It is the third front door onto the assessment engine, after the scholarship
// funnel and recruiter hiring drives, and deliberately mirrors the scholarship
// handler: provision an account, write an invite row (the invite *is* the
// eligibility), hand out a hashed one-time link, mint a session on claim.
//
// The one structural difference is ordering. Scholarship grants eligibility the
// moment someone applies; here the candidate has paid, so eligibility waits for
// a verified payment and is granted by fulfil() — from either the browser
// callback or the webhook, whichever arrives first, exactly once.
//
// Three invariants carried over from the scholarship funnel, because this
// endpoint is public and accepts any email address:
//   - an existing account is never re-credentialed and never has its role changed;
//   - only the SHA-256 digest of a claim token is stored;
//   - a claim may only open an account the funnel itself created (role 'applicant').
type CertificationHandler struct {
	Pool   *pgxpool.Pool
	Log    *zap.Logger
	Mailer *scholarshipMailer
	// Referrals prices the friend discount and books the referrer's reward;
	// nil simply means no referral is applied.
	Referrals *ReferralHandler

	jwtSecret string
	appBase   string // where a candidate sits the exam
	siteBase  string // where a credential is verified

	keyID, keySecret, webhookSecret string
	apiBase                         string
	client                          *http.Client
	ipLimiter                       *rateLimiter
	emailLimiter                    *rateLimiter
}

// Registration statuses that mean the exam is over for this payment. A second
// registration (and a second fee) is the way back in.
var certTerminal = map[string]bool{
	"submitted": true, "passed": true, "failed": true, "expired": true, "refunded": true,
}

func NewCertificationHandler(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger,
	jwtSecret, appBase, siteBase string, mailer *scholarshipMailer) (*CertificationHandler, error) {

	h := &CertificationHandler{
		Pool:          pool,
		Log:           log,
		Mailer:        mailer,
		jwtSecret:     jwtSecret,
		appBase:       strings.TrimRight(appBase, "/"),
		siteBase:      strings.TrimRight(siteBase, "/"),
		keyID:         strings.TrimSpace(envOr("RAZORPAY_KEY_ID", "")),
		keySecret:     strings.TrimSpace(envOr("RAZORPAY_KEY_SECRET", "")),
		webhookSecret: strings.TrimSpace(envOr("RAZORPAY_WEBHOOK_SECRET", "")),
		apiBase:       strings.TrimRight(envOr("RAZORPAY_API_BASE", "https://api.razorpay.com"), "/"),
		client:        &http.Client{Timeout: 15 * time.Second},
		// Same reasoning as enrolment: per-IP off by default so one college
		// network cannot lock itself out, per-email tight enough to stop one
		// person hammering checkout.
		ipLimiter:    newRateLimiter(envInt("CERTIFICATION_IP_LIMIT", 0), envMinutes("CERTIFICATION_IP_WINDOW_MIN", 10)),
		emailLimiter: newRateLimiter(envInt("CERTIFICATION_EMAIL_LIMIT", 10), time.Hour),
	}
	if err := h.ensureTables(ctx); err != nil {
		return nil, err
	}
	if !h.paymentsEnabled() {
		log.Warn("certification exams cannot be sold (RAZORPAY_KEY_ID / RAZORPAY_KEY_SECRET unset)")
	}
	if h.paymentsEnabled() && h.webhookSecret == "" {
		log.Warn("RAZORPAY_WEBHOOK_SECRET unset — a candidate who closes the tab after paying gets no exam link until staff retry it")
	}
	return h, nil
}

func (h *CertificationHandler) paymentsEnabled() bool { return h.keyID != "" && h.keySecret != "" }

func (h *CertificationHandler) ensureTables(ctx context.Context) error {
	_, err := h.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS certification_exams (
			id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			-- The URL segment on the marketing site.
			slug            TEXT        NOT NULL UNIQUE,
			title           TEXT        NOT NULL,
			-- Optional: the course this exam certifies, matching user_courses.course_id.
			course_id       TEXT        NOT NULL DEFAULT '',
			-- FK-by-convention into assessments, matching how scholarship_programs
			-- references its paper.
			assessment_id   UUID        NOT NULL,
			price_paise     BIGINT      NOT NULL CHECK (price_paise >= 0),
			pass_percent    NUMERIC(5,2) NOT NULL DEFAULT 60 CHECK (pass_percent >= 0 AND pass_percent <= 100),
			-- How long the emailed exam link stays valid after payment.
			link_valid_days INT         NOT NULL DEFAULT 30 CHECK (link_valid_days BETWEEN 1 AND 365),
			-- Days a candidate must wait after failing before paying to resit.
			resit_wait_days INT         NOT NULL DEFAULT 7 CHECK (resit_wait_days >= 0),
			summary         TEXT        NOT NULL DEFAULT '',
			is_active       BOOLEAN     NOT NULL DEFAULT true,
			opens_at        TIMESTAMPTZ,
			closes_at       TIMESTAMPTZ,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
		);

		CREATE TABLE IF NOT EXISTS certification_registrations (
			id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			exam_id             UUID NOT NULL REFERENCES certification_exams(id) ON DELETE RESTRICT,
			name                TEXT NOT NULL,
			email               TEXT NOT NULL,
			phone               TEXT NOT NULL DEFAULT '',
			user_id             UUID,
			invite_id           UUID,
			razorpay_order_id   TEXT UNIQUE,
			razorpay_payment_id TEXT,
			amount_paise        BIGINT NOT NULL,
			-- created → paid (signature verified, invite written, link mailed)
			-- → started → submitted → passed | failed. expired is set by the
			-- sweeper when a link lapses unused; refunded by staff.
			status              TEXT NOT NULL DEFAULT 'created'
			                    CHECK (status IN ('created', 'paid', 'started', 'submitted',
			                                      'passed', 'failed', 'expired', 'refunded')),
			-- Only the digest is stored; the raw token lives in the candidate's email.
			claim_token_hash    TEXT UNIQUE,
			claim_expires_at    TIMESTAMPTZ,
			claimed_at          TIMESTAMPTZ,
			certificate_id      UUID,
			emailed_at          TIMESTAMPTZ,
			error               TEXT NOT NULL DEFAULT '',
			notes               TEXT NOT NULL DEFAULT '',
			created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
			paid_at             TIMESTAMPTZ,
			updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS idx_cert_reg_created ON certification_registrations(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_cert_reg_status  ON certification_registrations(status);
		CREATE INDEX IF NOT EXISTS idx_cert_reg_email   ON certification_registrations(lower(email));
		CREATE INDEX IF NOT EXISTS idx_cert_reg_exam    ON certification_registrations(exam_id);
		-- Who sent this candidate, kept on the registration for the same reason
		-- the enrolment keeps it: attribution has to outlive the checkout.
		ALTER TABLE certification_registrations ADD COLUMN IF NOT EXISTS referral_code TEXT NOT NULL DEFAULT '';
		ALTER TABLE certification_registrations ADD COLUMN IF NOT EXISTS referral_discount_paise BIGINT NOT NULL DEFAULT 0;
		ALTER TABLE certification_registrations ADD COLUMN IF NOT EXISTS referral_flags TEXT NOT NULL DEFAULT '';

		CREATE TABLE IF NOT EXISTS certificates (
			id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			-- Printed on the certificate and used by the public verification page.
			credential_id  TEXT        NOT NULL UNIQUE,
			user_id        UUID        NOT NULL,
			-- Frozen at issue time: a later name change must not silently rewrite
			-- a credential somebody has already shown an employer.
			holder_name    TEXT        NOT NULL,
			title          TEXT        NOT NULL,
			-- exam = earned by passing a certification exam.
			-- course = issued on course completion (reserved for the app's
			-- Certificate tab, which has no real issuance yet).
			source         TEXT        NOT NULL DEFAULT 'exam' CHECK (source IN ('exam', 'course')),
			exam_id        UUID,
			registration_id UUID,
			attempt_id     UUID,
			score_percent  NUMERIC(5,2),
			issued_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			revoked_at     TIMESTAMPTZ,
			revoke_reason  TEXT        NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_certificates_user ON certificates(user_id);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_certificates_registration
			ON certificates(registration_id) WHERE registration_id IS NOT NULL;
	`)
	if err != nil {
		return fmt.Errorf("create certification tables: %w", err)
	}
	return nil
}

// ─── Routing ──────────────────────────────────────────────────────────────────

func (h *CertificationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	path := strings.TrimRight(strings.TrimPrefix(r.URL.Path, "/api/certification"), "/")
	switch {
	case path == "/config" && r.Method == http.MethodGet:
		h.handleConfig(w, r)
	case path == "/order" && r.Method == http.MethodPost:
		h.handleOrder(w, r)
	case path == "/verify" && r.Method == http.MethodPost:
		h.handleVerify(w, r)
	case path == "/webhook" && r.Method == http.MethodPost:
		h.handleWebhook(w, r)
	case path == "/claim" && r.Method == http.MethodPost:
		h.handleClaim(w, r)
	case path == "/outcome" && r.Method == http.MethodGet:
		h.handleOutcome(w, r)
	// A query parameter, not a path segment: the gateway's public-path set is
	// exact-match by design, so a credential id in the path could never be
	// reachable without a session — and an employer checking a certificate has
	// no account.
	case path == "/mine" && r.Method == http.MethodGet:
		h.handleMine(w, r)
	case path == "/credential" && r.Method == http.MethodGet:
		h.handleCredential(w, r, r.URL.Query().Get("id"))
	default:
		h.fail(w, http.StatusNotFound, "not found")
	}
}

// ─── GET /api/certification/config ────────────────────────────────────────────

type certExamPublic struct {
	Slug            string  `json:"slug"`
	Title           string  `json:"title"`
	CourseID        string  `json:"courseId"`
	Summary         string  `json:"summary"`
	PriceRupees     int64   `json:"priceRupees"`
	PassPercent     float64 `json:"passPercent"`
	LinkValidDays   int     `json:"linkValidDays"`
	DurationMinutes int32   `json:"durationMinutes"`
	TotalMarks      int32   `json:"totalMarks"`
	PaperTitle      string  `json:"paperTitle"`
}

// handleConfig lists the exams a visitor can actually buy right now: active,
// inside their window, and pointing at a published paper. An exam whose paper
// is still a draft is invisible rather than sold — the same rule the
// scholarship config applies, for the same reason.
func (h *CertificationHandler) handleConfig(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(), `
		SELECT c.slug, c.title, c.course_id, c.summary, c.price_paise, c.pass_percent, c.link_valid_days,
		       a.title, a.duration_minutes, a.total_marks
		FROM   certification_exams c
		JOIN   assessments a ON a.id = c.assessment_id
		WHERE  c.is_active
		  AND  a.status = 'published'
		  AND  (c.opens_at  IS NULL OR c.opens_at  <= now())
		  AND  (c.closes_at IS NULL OR c.closes_at >= now())
		ORDER  BY c.title
	`)
	if err != nil {
		h.Log.Error("list certification exams failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load the exams, please try again")
		return
	}
	defer rows.Close()

	out := []certExamPublic{}
	for rows.Next() {
		var e certExamPublic
		var pricePaise int64
		if err := rows.Scan(&e.Slug, &e.Title, &e.CourseID, &e.Summary, &pricePaise, &e.PassPercent,
			&e.LinkValidDays, &e.PaperTitle, &e.DurationMinutes, &e.TotalMarks); err != nil {
			h.Log.Error("scan certification exam failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not load the exams, please try again")
			return
		}
		e.PriceRupees = pricePaise / 100
		out = append(out, e)
	}
	h.write(w, http.StatusOK, map[string]any{
		"enabled": h.paymentsEnabled(),
		"exams":   out,
	})
}

// ─── POST /api/certification/order ────────────────────────────────────────────

// handleOrder registers a candidate and opens a Razorpay order. Nothing is
// granted here: the row exists so a payment can be matched back to a person,
// and eligibility is written only once the payment verifies.
func (h *CertificationHandler) handleOrder(w http.ResponseWriter, r *http.Request) {
	if !h.paymentsEnabled() {
		h.fail(w, http.StatusServiceUnavailable, "exam registration is not available right now — please contact us")
		return
	}
	if !h.ipLimiter.allow(clientIP(r)) {
		h.fail(w, http.StatusTooManyRequests, "too many attempts from this network — please try again in a few minutes")
		return
	}

	var req struct {
		Slug         string `json:"slug"`
		Name         string `json:"name"`
		Email        string `json:"email"`
		Phone        string `json:"phone"`
		ReferralCode string `json:"referral_code"`
		Website      string `json:"website"` // honeypot
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&req); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request")
		return
	}
	if strings.TrimSpace(req.Website) != "" {
		// Answer a bot exactly like a success, so it learns nothing — but never
		// create an order for it.
		h.Log.Info("certification honeypot triggered", zap.String("ip", clientIP(r)))
		h.write(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	name := clip(strings.TrimSpace(req.Name), 120)
	email := strings.ToLower(clip(strings.TrimSpace(req.Email), 200))
	phone := clip(strings.TrimSpace(req.Phone), 30)
	if name == "" {
		h.fail(w, http.StatusBadRequest, "please enter your name — it is printed on the certificate")
		return
	}
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email {
		h.fail(w, http.StatusBadRequest, "please enter a valid email address — your exam link is sent there")
		return
	}
	if !h.emailLimiter.allow(email) {
		h.fail(w, http.StatusTooManyRequests, "too many attempts for this email — please try again later")
		return
	}

	ctx := r.Context()
	var examID, title string
	var pricePaise int64
	var resitWait int
	err := h.Pool.QueryRow(ctx, `
		SELECT c.id::text, c.title, c.price_paise, c.resit_wait_days
		FROM   certification_exams c
		JOIN   assessments a ON a.id = c.assessment_id
		WHERE  c.slug = $1 AND c.is_active AND a.status = 'published'
		  AND  (c.opens_at  IS NULL OR c.opens_at  <= now())
		  AND  (c.closes_at IS NULL OR c.closes_at >= now())
	`, clip(req.Slug, 80)).Scan(&examID, &title, &pricePaise, &resitWait)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, http.StatusNotFound, "that exam is not open for registration")
		return
	}
	if err != nil {
		h.Log.Error("look up certification exam failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not start your registration, please try again")
		return
	}

	// A live registration this person has not used yet: sell them nothing, and
	// point them back at the link they already hold.
	var liveStatus string
	err = h.Pool.QueryRow(ctx, `
		SELECT status FROM certification_registrations
		WHERE  exam_id = $1::uuid AND lower(email) = $2
		  AND  status IN ('paid', 'started')
		  AND  (claim_expires_at IS NULL OR claim_expires_at > now())
		LIMIT  1
	`, examID, email).Scan(&liveStatus)
	if err == nil {
		h.fail(w, http.StatusConflict,
			"you already have an exam link for "+title+" — check your email, or contact us to have it resent")
		return
	}

	// Already certified: nothing to sell.
	var alreadyPassed bool
	_ = h.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM certification_registrations
		               WHERE exam_id = $1::uuid AND lower(email) = $2 AND status = 'passed')
	`, examID, email).Scan(&alreadyPassed)
	if alreadyPassed {
		h.fail(w, http.StatusConflict, "you have already passed "+title+" — your certificate is in your account")
		return
	}

	// Cooling-off after a failure, so the question bank cannot be mapped by
	// paying repeatedly in one sitting.
	if resitWait > 0 {
		var failedAt *time.Time
		_ = h.Pool.QueryRow(ctx, `
			SELECT max(updated_at) FROM certification_registrations
			WHERE  exam_id = $1::uuid AND lower(email) = $2 AND status = 'failed'
		`, examID, email).Scan(&failedAt)
		if failedAt != nil {
			if wait := failedAt.Add(time.Duration(resitWait) * 24 * time.Hour); time.Now().UTC().Before(wait) {
				h.fail(w, http.StatusConflict, fmt.Sprintf(
					"you can resit this exam from %s — resits open %d days after an attempt",
					wait.Format("2 January 2006"), resitWait))
				return
			}
		}
	}

	// Priced here, not in the browser — the same rule the fee itself follows.
	amountPaise := pricePaise
	var quote referralQuote
	if h.Referrals != nil && req.ReferralCode != "" {
		quote = h.Referrals.Quote(ctx, req.ReferralCode, email, phone, referralKindExam, amountPaise)
		amountPaise -= quote.DiscountPaise
	}

	var regID string
	if err := h.Pool.QueryRow(ctx, `
		INSERT INTO certification_registrations (exam_id, name, email, phone, amount_paise,
		                                         referral_code, referral_discount_paise, referral_flags)
		VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8) RETURNING id::text
	`, examID, name, email, phone, amountPaise, quote.Code, quote.DiscountPaise, quote.Flags).Scan(&regID); err != nil {
		h.Log.Error("insert certification registration failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not start your registration, please try again")
		return
	}

	rzpOrderID, err := h.createRazorpayOrder(ctx, amountPaise, regID, map[string]string{
		"kind": "certification", "exam": req.Slug, "email": email,
	})
	if err != nil {
		h.Log.Error("create certification razorpay order failed", zap.Error(err))
		_, _ = h.Pool.Exec(ctx, `UPDATE certification_registrations SET error = $2, updated_at = now() WHERE id = $1::uuid`,
			regID, clip(err.Error(), 500))
		h.fail(w, http.StatusBadGateway, "the payment gateway did not respond — please try again in a minute")
		return
	}
	if _, err := h.Pool.Exec(ctx, `
		UPDATE certification_registrations SET razorpay_order_id = $2, updated_at = now() WHERE id = $1::uuid
	`, regID, rzpOrderID); err != nil {
		h.Log.Error("store certification order id failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not start your registration, please try again")
		return
	}

	h.write(w, http.StatusOK, map[string]any{
		"key_id":            h.keyID,
		"order_id":          rzpOrderID,
		"amount":            amountPaise,
		"currency":          "INR",
		"exam_name":         title,
		"list_amount":       pricePaise,
		"referral_discount": quote.DiscountPaise,
		"referral_code":     quote.Code,
		"prefill":           map[string]string{"name": name, "email": email, "contact": phone},
	})
}

// createRazorpayOrder mirrors the enrolment handler's call; kept separate so a
// change to exam receipts cannot alter course checkout.
func (h *CertificationHandler) createRazorpayOrder(ctx context.Context, amountPaise int64, receipt string, notes map[string]string) (string, error) {
	e := &EnrollHandler{keyID: h.keyID, keySecret: h.keySecret, apiBase: h.apiBase, client: h.client}
	return e.createRazorpayOrder(ctx, amountPaise, receipt, notes)
}

// ─── POST /api/certification/verify and /webhook ──────────────────────────────

func (h *CertificationHandler) handleVerify(w http.ResponseWriter, r *http.Request) {
	if !h.paymentsEnabled() {
		h.fail(w, http.StatusServiceUnavailable, "exam registration is not available")
		return
	}
	var req struct {
		OrderID   string `json:"razorpay_order_id"`
		PaymentID string `json:"razorpay_payment_id"`
		Signature string `json:"razorpay_signature"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&req); err != nil || req.OrderID == "" || req.PaymentID == "" {
		h.fail(w, http.StatusBadRequest, "invalid request")
		return
	}
	if !validPaymentSignature(req.OrderID, req.PaymentID, req.Signature, h.keySecret) {
		h.Log.Warn("certification payment signature mismatch", zap.String("order", req.OrderID))
		h.fail(w, http.StatusBadRequest, "the payment could not be verified — if money was deducted, contact us with your payment id")
		return
	}
	res, err := h.fulfil(r.Context(), req.OrderID, req.PaymentID)
	if err != nil {
		h.Log.Error("certification fulfilment failed", zap.String("order", req.OrderID), zap.Error(err))
		h.fail(w, http.StatusInternalServerError,
			"your payment was received but we could not issue your exam link — our team can send it to you")
		return
	}
	h.write(w, http.StatusOK, res)
}

func (h *CertificationHandler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if h.webhookSecret == "" {
		h.fail(w, http.StatusNotFound, "not found")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
	if err != nil {
		h.fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !validWebhookSignature(raw, r.Header.Get("X-Razorpay-Signature"), h.webhookSecret) {
		h.fail(w, http.StatusBadRequest, "bad signature")
		return
	}
	var ev struct {
		Event   string `json:"event"`
		Payload struct {
			Payment struct {
				Entity struct {
					ID      string            `json:"id"`
					OrderID string            `json:"order_id"`
					Notes   map[string]string `json:"notes"`
				} `json:"entity"`
			} `json:"payment"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid json")
		return
	}
	p := ev.Payload.Payment.Entity
	if (ev.Event == "payment.captured" || ev.Event == "order.paid") && p.OrderID != "" && p.ID != "" {
		// Course enrolments and exam registrations both arrive here; fulfil
		// resolves by order id and simply finds nothing for an order that is
		// not ours, so the two handlers cannot fulfil each other's payments.
		if _, err := h.fulfil(r.Context(), p.OrderID, p.ID); err != nil {
			if errors.Is(err, errNotOurOrder) {
				h.write(w, http.StatusOK, map[string]bool{"ok": true})
				return
			}
			h.Log.Error("certification webhook fulfilment failed", zap.String("order", p.OrderID), zap.Error(err))
			// 500 makes Razorpay retry, which is what we want for a transient error.
			h.fail(w, http.StatusInternalServerError, "fulfilment failed")
			return
		}
	}
	h.write(w, http.StatusOK, map[string]bool{"ok": true})
}

var errNotOurOrder = errors.New("order is not a certification registration")

type certFulfilResult struct {
	Status   string `json:"status"`
	Email    string `json:"email"`
	ExamName string `json:"exam_name"`
	Emailed  bool   `json:"emailed"`
	// ExpiresAt is when the exam link lapses, so the confirmation page can say so.
	ExpiresAt string `json:"expires_at,omitempty"`
}

// fulfil grants eligibility for a paid registration: it provisions the account,
// writes the invite row the assessment engine checks, stores the digest of a
// fresh claim token and emails the link.
//
// Idempotent in the same way as enrolment: only the caller that moves the row
// out of 'created' does the work, so verify and webhook racing each other
// cannot mail two links or write two invites.
func (h *CertificationHandler) fulfil(ctx context.Context, rzpOrderID, paymentID string) (*certFulfilResult, error) {
	var regID, examID, name, email, phone, examTitle, assessmentID string
	var referralCode, referralFlags string
	var amountPaise, referralDiscount int64
	var linkValidDays int
	err := h.Pool.QueryRow(ctx, `
		UPDATE certification_registrations reg
		   SET status = 'paid', razorpay_payment_id = $2, paid_at = now(), updated_at = now()
		  FROM certification_exams e
		 WHERE reg.razorpay_order_id = $1
		   AND e.id = reg.exam_id
		   -- A registration whose invite could not be written is retried by the
		   -- next verify / webhook rather than staying stuck: 'paid' with no
		   -- claim token is exactly that state.
		   AND (reg.status = 'created' OR (reg.status = 'paid' AND reg.claim_token_hash IS NULL))
		RETURNING reg.id::text, reg.exam_id::text, reg.name, reg.email, reg.phone,
		          e.title, e.assessment_id::text, e.link_valid_days,
		          reg.referral_code, reg.referral_discount_paise, reg.referral_flags, reg.amount_paise
	`, rzpOrderID, paymentID).Scan(&regID, &examID, &name, &email, &phone, &examTitle, &assessmentID,
		&linkValidDays, &referralCode, &referralDiscount, &referralFlags, &amountPaise)

	if errors.Is(err, pgx.ErrNoRows) {
		// Already fulfilled, or not one of ours. Report the stored outcome.
		var status, storedEmail, title string
		var expires *time.Time
		if err := h.Pool.QueryRow(ctx, `
			SELECT reg.status, reg.email, e.title, reg.claim_expires_at
			FROM   certification_registrations reg JOIN certification_exams e ON e.id = reg.exam_id
			WHERE  reg.razorpay_order_id = $1
		`, rzpOrderID).Scan(&status, &storedEmail, &title, &expires); err != nil {
			return nil, errNotOurOrder
		}
		res := &certFulfilResult{Status: status, Email: storedEmail, ExamName: title}
		if expires != nil {
			res.ExpiresAt = expires.UTC().Format(time.RFC3339)
		}
		return res, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mark registration paid: %w", err)
	}

	rawToken, err := randomToken(32)
	if err != nil {
		return nil, fmt.Errorf("generate claim token: %w", err)
	}
	expiresAt := time.Now().UTC().Add(time.Duration(linkValidDays) * 24 * time.Hour)

	userID, inviteID, err := h.provision(ctx, name, email, phone, assessmentID, expiresAt)
	if err != nil {
		_, _ = h.Pool.Exec(context.Background(), `
			UPDATE certification_registrations SET error = $2, updated_at = now() WHERE id = $1::uuid`,
			regID, clip(err.Error(), 500))
		return nil, err
	}

	if _, err := h.Pool.Exec(ctx, `
		UPDATE certification_registrations
		   SET user_id = $2::uuid, invite_id = $3::uuid, claim_token_hash = $4,
		       claim_expires_at = $5, error = '', updated_at = now()
		 WHERE id = $1::uuid
	`, regID, userID, inviteID, sha256Hex(rawToken), expiresAt); err != nil {
		return nil, fmt.Errorf("store claim token: %w", err)
	}

	examURL := h.appBase + "/certification/start?t=" + rawToken
	emailed := h.sendExamLink(name, email, examTitle, examURL, expiresAt)
	if emailed {
		_, _ = h.Pool.Exec(ctx, `UPDATE certification_registrations SET emailed_at = now() WHERE id = $1::uuid`, regID)
	} else {
		_, _ = h.Pool.Exec(context.Background(), `
			UPDATE certification_registrations
			   SET error = 'exam link email not sent — resend it from the admin panel', updated_at = now()
			 WHERE id = $1::uuid`, regID)
	}

	// Booked only once the registration is real and the exam link has been
	// issued, and idempotent on the registration — so the verify/webhook race
	// cannot pay a referrer twice.
	if h.Referrals != nil && referralCode != "" {
		h.Referrals.RecordConversion(ctx, referralKindExam, regID, referralCode,
			name, email, examTitle, referralFlags, amountPaise, referralDiscount)
	}

	h.Log.Info("certification exam purchased",
		zap.String("exam", examTitle), zap.String("email", email), zap.Bool("emailed", emailed))

	return &certFulfilResult{
		Status: "paid", Email: email, ExamName: examTitle,
		Emailed: emailed, ExpiresAt: expiresAt.Format(time.RFC3339),
	}, nil
}

// provision upserts the candidate's account and their invite for the paper.
//
// The account rules are the scholarship funnel's, and for the same reason: this
// endpoint is public and accepts any address, so it must never overwrite an
// existing person's password or promote/demote their role. An existing student
// keeps their account and simply gains an invite; their exam link still works,
// but claim() will send them to sign in rather than mint a session.
func (h *CertificationHandler) provision(ctx context.Context, name, email, phone, assessmentID string, expiresAt time.Time) (userID, inviteID string, err error) {
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	// No password, rather than a random one nobody is told: the candidate signs
	// in through their exam link, and sets a real password later if they ever
	// become a student.
	unusable, err := pkgauth.UnusablePassword()
	if err != nil {
		return "", "", fmt.Errorf("prepare candidate account: %w", err)
	}
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (email, name, password, role)
		VALUES ($1, $2, $3, 'applicant')
		ON CONFLICT (email) DO UPDATE
		   SET name = CASE WHEN users.role = 'applicant' THEN EXCLUDED.name ELSE users.name END,
		       updated_at = now()
		RETURNING id::text
	`, email, name, unusable).Scan(&userID); err != nil {
		return "", "", fmt.Errorf("provision candidate: %w", err)
	}

	if phone != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_profiles (user_id, phone) VALUES ($1::uuid, $2)
			ON CONFLICT (user_id) DO UPDATE SET phone = COALESCE(NULLIF(user_profiles.phone, ''), EXCLUDED.phone)
		`, userID, phone); err != nil {
			h.Log.Warn("store candidate phone failed", zap.Error(err))
		}
	}

	inviteToken, err := randomToken(32)
	if err != nil {
		return "", "", err
	}
	// The invite row IS the eligibility: assessment-service lets nobody start a
	// non-practice paper without one. Re-registering after a failed attempt
	// refreshes the same row rather than accumulating invites.
	if err := tx.QueryRow(ctx, `
		INSERT INTO assessment_invites (assessment_id, email, user_id, token, status, expires_at, sent_at)
		VALUES ($1::uuid, $2, $3::uuid, $4, 'invited', $5, now())
		ON CONFLICT (assessment_id, email) DO UPDATE
		   SET user_id    = EXCLUDED.user_id,
		       token      = EXCLUDED.token,
		       status     = 'invited',
		       expires_at = EXCLUDED.expires_at,
		       sent_at    = now()
		RETURNING id::text
	`, assessmentID, email, userID, inviteToken, expiresAt).Scan(&inviteID); err != nil {
		return "", "", fmt.Errorf("write invite: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return userID, inviteID, nil
}

// ─── POST /api/certification/claim ────────────────────────────────────────────

func (h *CertificationHandler) handleClaim(w http.ResponseWriter, r *http.Request) {
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
	if !h.ipLimiter.allow("cert-claim:" + clientIP(r)) {
		h.fail(w, http.StatusTooManyRequests, "too many attempts from this network — please try again in a few minutes")
		return
	}

	ctx := r.Context()
	var regID, status, userID, assessmentID, examTitle, inviteToken, email, userName, role string
	var expiresAt *time.Time
	err := h.Pool.QueryRow(ctx, `
		SELECT reg.id::text, reg.status, reg.user_id::text, e.assessment_id::text, e.title,
		       reg.claim_expires_at, COALESCE(i.token, ''), u.email, u.name, u.role
		FROM   certification_registrations reg
		JOIN   certification_exams e ON e.id = reg.exam_id
		JOIN   users u ON u.id = reg.user_id
		LEFT   JOIN assessment_invites i ON i.id = reg.invite_id
		WHERE  reg.claim_token_hash = $1
	`, sha256Hex(raw)).Scan(&regID, &status, &userID, &assessmentID, &examTitle,
		&expiresAt, &inviteToken, &email, &userName, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		// One answer for a token that never existed and one that was replaced,
		// so this cannot be used to probe for live links.
		h.fail(w, http.StatusUnauthorized, "this link is no longer valid — please contact us if you have paid for this exam")
		return
	}
	if err != nil {
		h.Log.Error("look up certification claim failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not open your exam, please try again")
		return
	}

	if expiresAt != nil && time.Now().UTC().After(*expiresAt) {
		if _, err := h.Pool.Exec(ctx, `
			UPDATE certification_registrations SET status = 'expired', updated_at = now()
			WHERE id = $1::uuid AND status IN ('paid', 'started')
		`, regID); err != nil {
			h.Log.Warn("mark registration expired failed", zap.Error(err))
		}
		h.fail(w, http.StatusUnauthorized, "this exam link has expired — contact us to have it extended")
		return
	}
	if certTerminal[status] {
		h.fail(w, http.StatusConflict, "you have already sat this exam")
		return
	}
	if status == "created" {
		h.fail(w, http.StatusConflict, "we have not received payment for this exam yet")
		return
	}
	// Same rule as the scholarship funnel: a public form accepts any address,
	// so a claim may only ever open an account the funnel itself created.
	if role != "applicant" {
		h.Log.Warn("certification claim refused for existing account",
			zap.String("email", email), zap.String("role", role))
		h.fail(w, http.StatusConflict,
			"you already have a Knovate account — sign in with your password and the exam will be waiting in your tests (use \"Forgot password\" if you need to set one)")
		return
	}

	// Deliberately not single-use: a candidate who refreshes the redirect before
	// the session reaches localStorage must not be locked out of an exam they
	// paid for. The link dies at expiry or on submission.
	if _, err := h.Pool.Exec(ctx, `
		UPDATE certification_registrations
		SET    claimed_at = COALESCE(claimed_at, now()), updated_at = now()
		WHERE  id = $1::uuid
	`, regID); err != nil {
		h.Log.Error("mark registration claimed failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not open your exam, please try again")
		return
	}

	token, err := pkgauth.GenerateToken(userID, email, role, h.jwtSecret, 24*time.Hour)
	if err != nil {
		h.Log.Error("mint certification session failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not open your exam, please try again")
		return
	}

	h.Log.Info("certification claim accepted", zap.String("email", email), zap.String("exam", examTitle))
	h.write(w, http.StatusOK, map[string]any{
		"token": token,
		"user": map[string]any{
			"id": userID, "email": email, "name": userName, "role": role,
		},
		"assessmentId":   assessmentID,
		"inviteToken":    inviteToken,
		"examTitle":      examTitle,
		"registrationId": regID,
	})
}

// ─── GET /api/certification/outcome?attemptId= ────────────────────────────────

// handleOutcome reports pass or fail for a finished exam, and issues the
// certificate the first time a pass is read.
//
// Issuance is lazy rather than driven by a sweeper: grading settles
// asynchronously (coding submissions arrive over NATS), so "the moment the
// result is known" is exactly when somebody asks for it. Admin can also issue
// by hand for an attempt held back by an integrity review.
func (h *CertificationHandler) handleOutcome(w http.ResponseWriter, r *http.Request) {
	attemptID := strings.TrimSpace(r.URL.Query().Get("attemptId"))
	if attemptID == "" {
		h.fail(w, http.StatusBadRequest, "attemptId is required")
		return
	}

	ctx := r.Context()
	var regID, status, examTitle, holderName, userID string
	var passPercent float64
	var score, maxScore *float64
	var attemptStatus string
	err := h.Pool.QueryRow(ctx, `
		SELECT reg.id::text, reg.status, e.title, e.pass_percent, u.name, u.id::text,
		       at.score, at.max_score, at.status
		FROM   attempts at
		JOIN   certification_exams e ON e.assessment_id = at.assessment_id
		JOIN   certification_registrations reg
		       ON reg.exam_id = e.id AND reg.user_id = at.user_id
		JOIN   users u ON u.id = at.user_id
		WHERE  at.id = $1::uuid
		ORDER  BY reg.created_at DESC
		LIMIT  1
	`, attemptID).Scan(&regID, &status, &examTitle, &passPercent, &holderName, &userID,
		&score, &maxScore, &attemptStatus)
	if errors.Is(err, pgx.ErrNoRows) || (err != nil && strings.Contains(err.Error(), "invalid input syntax")) {
		// Not a certification attempt — the caller is a scholarship or practice
		// taker and the result page should fall back to its own rendering.
		h.write(w, http.StatusOK, map[string]any{"isCertification": false})
		return
	}
	if err != nil {
		h.Log.Error("load certification outcome failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load your result, please try again")
		return
	}

	// Still grading: say so rather than reporting a fail on a partial score.
	if attemptStatus == "in_progress" || attemptStatus == "submitted" || attemptStatus == "evaluating" {
		h.write(w, http.StatusOK, map[string]any{
			"isCertification": true, "examTitle": examTitle, "evaluating": true,
			"passPercent": passPercent,
		})
		return
	}

	percent := 0.0
	if score != nil && maxScore != nil && *maxScore > 0 {
		percent = (*score / *maxScore) * 100
	}
	passed := percent >= passPercent && attemptStatus == "evaluated"

	newStatus := "failed"
	if passed {
		newStatus = "passed"
	}
	if _, err := h.Pool.Exec(ctx, `
		UPDATE certification_registrations SET status = $2, updated_at = now()
		WHERE  id = $1::uuid AND status IN ('paid', 'started', 'submitted')
	`, regID, newStatus); err != nil {
		h.Log.Warn("update registration outcome failed", zap.Error(err))
	}

	resp := map[string]any{
		"isCertification": true,
		"examTitle":       examTitle,
		"evaluating":      false,
		"passed":          passed,
		"scorePercent":    roundTo(percent, 2),
		"passPercent":     passPercent,
	}

	if passed {
		cert, err := h.issueCertificate(ctx, regID, userID, holderName, examTitle, attemptID, percent)
		if err != nil {
			h.Log.Error("issue certificate failed", zap.String("registration", regID), zap.Error(err))
		} else if cert != nil {
			resp["credentialId"] = cert.CredentialID
			resp["verifyUrl"] = h.siteBase + "/verify/" + cert.CredentialID
			resp["issuedAt"] = cert.IssuedAt.Format(time.RFC3339)
		}
	}
	h.write(w, http.StatusOK, resp)
}

type certificate struct {
	CredentialID string
	HolderName   string
	Title        string
	ScorePercent float64
	IssuedAt     time.Time
	RevokedAt    *time.Time
	RevokeReason string
}

// issueCertificate records the credential, exactly once per registration. The
// unique index on registration_id is what makes a double read harmless.
func (h *CertificationHandler) issueCertificate(ctx context.Context, regID, userID, holderName, title, attemptID string, percent float64) (*certificate, error) {
	var c certificate
	err := h.Pool.QueryRow(ctx, `
		SELECT credential_id, holder_name, title, COALESCE(score_percent, 0), issued_at, revoked_at, revoke_reason
		FROM   certificates WHERE registration_id = $1::uuid
	`, regID).Scan(&c.CredentialID, &c.HolderName, &c.Title, &c.ScorePercent, &c.IssuedAt, &c.RevokedAt, &c.RevokeReason)
	if err == nil {
		return &c, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	credentialID, err := newCredentialID()
	if err != nil {
		return nil, err
	}
	err = h.Pool.QueryRow(ctx, `
		INSERT INTO certificates (credential_id, user_id, holder_name, title, source,
		                          exam_id, registration_id, attempt_id, score_percent)
		SELECT $1, $2::uuid, $3, $4, 'exam', reg.exam_id, reg.id, $5::uuid, $6
		FROM   certification_registrations reg WHERE reg.id = $7::uuid
		ON CONFLICT (registration_id) WHERE registration_id IS NOT NULL DO NOTHING
		RETURNING credential_id, holder_name, title, COALESCE(score_percent, 0), issued_at
	`, credentialID, userID, holderName, title, attemptID, roundTo(percent, 2), regID).
		Scan(&c.CredentialID, &c.HolderName, &c.Title, &c.ScorePercent, &c.IssuedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// Another request won the race; read theirs back.
		if err := h.Pool.QueryRow(ctx, `
			SELECT credential_id, holder_name, title, COALESCE(score_percent, 0), issued_at
			FROM   certificates WHERE registration_id = $1::uuid
		`, regID).Scan(&c.CredentialID, &c.HolderName, &c.Title, &c.ScorePercent, &c.IssuedAt); err != nil {
			return nil, err
		}
		return &c, nil
	}
	if err != nil {
		return nil, err
	}

	_, _ = h.Pool.Exec(ctx, `
		UPDATE certification_registrations SET certificate_id =
			(SELECT id FROM certificates WHERE registration_id = $1::uuid), updated_at = now()
		WHERE id = $1::uuid`, regID)

	h.Log.Info("certificate issued", zap.String("credential", c.CredentialID), zap.String("exam", title))
	return &c, nil
}

// ─── GET /api/certification/mine ──────────────────────────────────────────────

// handleMine lists the signed-in learner's certificates, newest first, for the
// Certificate tab in the student app. Revoked ones are included and marked:
// hiding a revoked credential would leave a learner puzzled about where their
// certificate went.
func (h *CertificationHandler) handleMine(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		h.fail(w, http.StatusUnauthorized, "authentication required")
		return
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT credential_id, title, COALESCE(score_percent, 0), issued_at,
		       revoked_at IS NOT NULL, source
		FROM   certificates
		WHERE  user_id = $1::uuid
		ORDER  BY issued_at DESC
	`, userID)
	if err != nil {
		h.Log.Error("list certificates failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load your certificates")
		return
	}
	defer rows.Close()

	type item struct {
		CredentialID string  `json:"credentialId"`
		Title        string  `json:"title"`
		ScorePercent float64 `json:"scorePercent"`
		IssuedAt     string  `json:"issuedAt"`
		Revoked      bool    `json:"revoked"`
		Source       string  `json:"source"`
		VerifyURL    string  `json:"verifyUrl"`
	}
	out := []item{}
	for rows.Next() {
		var it item
		var issued time.Time
		if err := rows.Scan(&it.CredentialID, &it.Title, &it.ScorePercent, &issued, &it.Revoked, &it.Source); err != nil {
			h.Log.Error("scan certificate failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not load your certificates")
			return
		}
		it.IssuedAt = issued.Format("2006-01-02")
		it.VerifyURL = h.siteBase + "/verify/" + it.CredentialID
		out = append(out, it)
	}
	h.write(w, http.StatusOK, map[string]any{"certificates": out})
}

// ─── GET /api/certification/credential/{id} — public verification ─────────────

// handleCredential backs GET /api/certification/credential?id=. It is what an
// employer hits. It confirms a credential exists
// and is not revoked, and says nothing else about the holder: no email, no
// phone, no attempt detail.
func (h *CertificationHandler) handleCredential(w http.ResponseWriter, r *http.Request, credentialID string) {
	id := strings.ToUpper(clip(strings.TrimSpace(credentialID), 40))
	if id == "" {
		h.fail(w, http.StatusBadRequest, "credential id is required")
		return
	}
	var c certificate
	err := h.Pool.QueryRow(r.Context(), `
		SELECT credential_id, holder_name, title, COALESCE(score_percent, 0), issued_at, revoked_at, revoke_reason
		FROM   certificates WHERE upper(credential_id) = $1
	`, id).Scan(&c.CredentialID, &c.HolderName, &c.Title, &c.ScorePercent, &c.IssuedAt, &c.RevokedAt, &c.RevokeReason)
	if errors.Is(err, pgx.ErrNoRows) {
		h.write(w, http.StatusNotFound, map[string]any{"found": false})
		return
	}
	if err != nil {
		h.Log.Error("credential lookup failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not check that credential, please try again")
		return
	}
	resp := map[string]any{
		"found":        true,
		"credentialId": c.CredentialID,
		"holderName":   c.HolderName,
		"title":        c.Title,
		"issuedAt":     c.IssuedAt.Format("2006-01-02"),
		"revoked":      c.RevokedAt != nil,
	}
	if c.RevokedAt != nil {
		resp["revokedAt"] = c.RevokedAt.Format("2006-01-02")
		resp["revokeReason"] = c.RevokeReason
	} else {
		resp["scorePercent"] = c.ScorePercent
	}
	h.write(w, http.StatusOK, resp)
}

// ─── Expiry sweeper ───────────────────────────────────────────────────────────

// StartExpirySweeper retires links that lapsed without ever being used, so the
// admin list distinguishes a candidate about to sit their exam from one who
// paid and walked away. A registration whose attempt has begun is left alone.
func (h *CertificationHandler) StartExpirySweeper(ctx context.Context, interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				n, err := h.expireStale(ctx)
				if err != nil {
					h.Log.Warn("certification expiry sweep failed", zap.Error(err))
				} else if n > 0 {
					h.Log.Info("certification links expired", zap.Int64("count", n))
				}
			}
		}
	}()
}

func (h *CertificationHandler) expireStale(ctx context.Context) (int64, error) {
	tag, err := h.Pool.Exec(ctx, `
		UPDATE certification_registrations reg
		   SET status = 'expired', updated_at = now()
		 WHERE reg.status IN ('paid', 'started')
		   AND reg.claim_expires_at IS NOT NULL
		   AND reg.claim_expires_at < now()
		   AND NOT EXISTS (
		       SELECT 1 FROM attempts a
		       JOIN   certification_exams e ON e.id = reg.exam_id
		       WHERE  a.user_id = reg.user_id AND a.assessment_id = e.assessment_id)
	`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ─── Email ────────────────────────────────────────────────────────────────────

// sendExamLink delivers the candidate's exam link and reports whether it
// actually went out — the confirmation page and the admin list must never claim
// an email was sent when it was not.
func (h *CertificationHandler) sendExamLink(name, email, examTitle, examURL string, expiresAt time.Time) bool {
	if h.Mailer == nil || !h.Mailer.enabled() {
		// Same degradation as the scholarship funnel: with SMTP unset the link
		// is logged so local development and a misconfigured deploy are both
		// recoverable rather than silently broken.
		h.Log.Warn("SMTP not configured — certification exam link logged instead of emailed",
			zap.String("email", email), zap.String("url", examURL))
		return false
	}
	when := expiresAt.Format("2 January 2006")
	subject := "Your " + examTitle + " exam link"
	text := fmt.Sprintf(`Hi %s,

Your payment is confirmed and your %s exam is ready.

Start the exam: %s

You can sit it any time before %s. Before you begin:
- Allow about the full duration in one sitting — the timer does not pause.
- Use a laptop or desktop with a working camera, on a stable connection.
- The exam runs in fullscreen and is proctored; switching tabs is recorded.

Pass and your certificate is issued immediately, with a link an employer can verify.

Knovate`, name, examTitle, examURL, when)

	html := certificationLinkHTML(name, examTitle, examURL, when)
	if err := h.Mailer.deliver([]string{email}, "", subject, text, html, email); err != nil {
		h.Log.Error("certification exam link email failed", zap.String("email", email), zap.Error(err))
		return false
	}
	return true
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// newCredentialID builds a readable credential like KNV-7F3K-92QD. Readable
// matters: people type these from a printed certificate.
func newCredentialID() (string, error) {
	raw, err := randomToken(16)
	if err != nil {
		return "", err
	}
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no look-alikes
	out := make([]byte, 8)
	for i := 0; i < 8; i++ {
		out[i] = alphabet[int(raw[i])%len(alphabet)]
	}
	return "KNV-" + string(out[:4]) + "-" + string(out[4:]), nil
}

func roundTo(v float64, places int) float64 {
	f, err := strconv.ParseFloat(strconv.FormatFloat(v, 'f', places, 64), 64)
	if err != nil {
		return v
	}
	return f
}

func (h *CertificationHandler) write(w http.ResponseWriter, code int, v any) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *CertificationHandler) fail(w http.ResponseWriter, code int, msg string) {
	h.write(w, code, map[string]string{"error": msg})
}

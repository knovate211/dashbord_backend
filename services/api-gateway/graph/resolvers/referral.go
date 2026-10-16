package resolvers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
)

// ReferralHandler runs the refer-a-friend programme.
//
//	POST /api/referral/join     — name, email, phone → a permanent referral link
//	GET  /api/referral/resolve  — is this code live, and whose first name to greet with
//	POST /api/referral/click    — records that a link was opened
//	POST /api/referral/status   — a referrer's own progress, keyed by code + email
//	GET  /api/referral/me       — a signed-in learner's own code and progress
//
// Two rules shape everything here.
//
// The first: the browser never decides money. A referral code travels with a
// checkout request, and the server looks up the discount — the same stance the
// course and exam checkouts already take with prices.
//
// The second: nothing pays out on its own. Anyone can create a referral link
// from a public form, so an automatic payout would be a faucet for whoever
// noticed first. A conversion is recorded as owed; a person approves it.
type ReferralHandler struct {
	Pool   *pgxpool.Pool
	Log    *zap.Logger
	Mailer *scholarshipMailer

	siteBase     string
	ipLimiter    *rateLimiter
	emailLimiter *rateLimiter
}

// Conversion kinds. These match the two things a friend can actually buy.
const (
	referralKindCourse = "course"
	referralKindExam   = "certification"
)

func NewReferralHandler(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger,
	siteBase string, mailer *scholarshipMailer) (*ReferralHandler, error) {

	h := &ReferralHandler{
		Pool:     pool,
		Log:      log,
		Mailer:   mailer,
		siteBase: strings.TrimRight(siteBase, "/"),
		// Per-IP off by default (one college network must not lock itself out);
		// per-email tight, because one address only ever needs one link.
		ipLimiter:    newRateLimiter(envInt("REFERRAL_IP_LIMIT", 0), envMinutes("REFERRAL_IP_WINDOW_MIN", 10)),
		emailLimiter: newRateLimiter(envInt("REFERRAL_EMAIL_LIMIT", 10), time.Hour),
	}
	if err := h.ensureTables(ctx); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *ReferralHandler) ensureTables(ctx context.Context) error {
	_, err := h.Pool.Exec(ctx, `
		-- One row. Every number the programme runs on lives here so staff can
		-- change the offer without a deploy.
		CREATE TABLE IF NOT EXISTS referral_program (
			id                      INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
			is_active               BOOLEAN NOT NULL DEFAULT false,
			-- What the referrer earns, per kind of purchase. Ticket sizes differ
			-- by ~10x between a course and an exam, so the rewards do too.
			course_reward_paise     BIGINT  NOT NULL DEFAULT 50000,
			exam_reward_paise       BIGINT  NOT NULL DEFAULT 20000,
			-- What the friend saves. Percent of the order, capped in rupees so a
			-- percentage on an expensive course cannot run away.
			friend_discount_percent NUMERIC(5,2) NOT NULL DEFAULT 10 CHECK (friend_discount_percent >= 0 AND friend_discount_percent <= 100),
			friend_discount_cap_paise BIGINT NOT NULL DEFAULT 150000,
			-- No reward below this order value.
			min_order_paise         BIGINT  NOT NULL DEFAULT 100000,
			-- The most one referrer can earn in a rolling month before their
			-- conversions are held for review rather than approved.
			monthly_cap_paise       BIGINT  NOT NULL DEFAULT 1000000,
			attribution_days        INT     NOT NULL DEFAULT 90 CHECK (attribution_days BETWEEN 1 AND 365),
			terms_url               TEXT    NOT NULL DEFAULT '/referrals#terms',
			updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		INSERT INTO referral_program (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

		CREATE TABLE IF NOT EXISTS referrers (
			id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			-- The code in the URL. Readable, because people read these aloud.
			code         TEXT NOT NULL UNIQUE,
			name         TEXT NOT NULL,
			-- One address, one code, forever: asking again returns the same link
			-- rather than fragmenting someone's referrals across two codes.
			email        TEXT NOT NULL UNIQUE,
			phone        TEXT NOT NULL DEFAULT '',
			-- Set when the referrer is also a learner, so the student app can
			-- show them their own code without asking for it again.
			user_id      UUID,
			upi_id       TEXT NOT NULL DEFAULT '',
			-- Collected only once payouts cross the tax threshold.
			pan          TEXT NOT NULL DEFAULT '',
			is_blocked   BOOLEAN NOT NULL DEFAULT false,
			notes        TEXT NOT NULL DEFAULT '',
			created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS idx_referrers_email ON referrers(lower(email));
		CREATE INDEX IF NOT EXISTS idx_referrers_user  ON referrers(user_id);

		-- Traffic, so a link that nobody opens is distinguishable from a link
		-- that everybody opens and nobody buys from. The IP is hashed: it is
		-- needed to spot one person clicking their own link a hundred times,
		-- not to identify anybody.
		CREATE TABLE IF NOT EXISTS referral_clicks (
			id         BIGSERIAL PRIMARY KEY,
			code       TEXT NOT NULL,
			at         TIMESTAMPTZ NOT NULL DEFAULT now(),
			ip_hash    TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
			path       TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_referral_clicks_code ON referral_clicks(code, at DESC);

		CREATE TABLE IF NOT EXISTS referral_conversions (
			id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			referrer_id   UUID NOT NULL REFERENCES referrers(id) ON DELETE RESTRICT,
			code          TEXT NOT NULL,
			kind          TEXT NOT NULL CHECK (kind IN ('course', 'certification')),
			-- The enrolment order or exam registration this reward is owed on.
			-- One reward per purchase, enforced by the unique index below.
			order_id      UUID NOT NULL,
			friend_name   TEXT NOT NULL DEFAULT '',
			friend_email  TEXT NOT NULL,
			item_name     TEXT NOT NULL DEFAULT '',
			order_paise   BIGINT NOT NULL DEFAULT 0,
			discount_paise BIGINT NOT NULL DEFAULT 0,
			reward_paise  BIGINT NOT NULL DEFAULT 0,
			-- pending → approved → paid, with rejected and reversed as exits.
			status        TEXT NOT NULL DEFAULT 'pending'
			              CHECK (status IN ('pending', 'approved', 'paid', 'rejected', 'reversed')),
			-- Why a human should look: self-referral near-miss, over the monthly
			-- cap, repeat address. Empty means nothing stood out.
			flags         TEXT NOT NULL DEFAULT '',
			reason        TEXT NOT NULL DEFAULT '',
			approved_by   TEXT NOT NULL DEFAULT '',
			approved_at   TIMESTAMPTZ,
			paid_at       TIMESTAMPTZ,
			payout_ref    TEXT NOT NULL DEFAULT '',
			created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_referral_conv_order ON referral_conversions(kind, order_id);
		CREATE INDEX IF NOT EXISTS idx_referral_conv_referrer ON referral_conversions(referrer_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_referral_conv_status ON referral_conversions(status);
		-- One reward per friend per kind: a second course bought by the same
		-- person does not pay the referrer twice.
		CREATE UNIQUE INDEX IF NOT EXISTS idx_referral_conv_friend
			ON referral_conversions(lower(friend_email), kind)
			WHERE status <> 'reversed';
	`)
	if err != nil {
		return fmt.Errorf("create referral tables: %w", err)
	}
	return nil
}

// ─── Programme settings ───────────────────────────────────────────────────────

type referralProgram struct {
	IsActive              bool    `json:"is_active"`
	CourseRewardPaise     int64   `json:"course_reward_paise"`
	ExamRewardPaise       int64   `json:"exam_reward_paise"`
	FriendDiscountPercent float64 `json:"friend_discount_percent"`
	FriendDiscountCap     int64   `json:"friend_discount_cap_paise"`
	MinOrderPaise         int64   `json:"min_order_paise"`
	MonthlyCapPaise       int64   `json:"monthly_cap_paise"`
	AttributionDays       int     `json:"attribution_days"`
	TermsURL              string  `json:"terms_url"`
}

func (h *ReferralHandler) program(ctx context.Context) (referralProgram, error) {
	var p referralProgram
	err := h.Pool.QueryRow(ctx, `
		SELECT is_active, course_reward_paise, exam_reward_paise, friend_discount_percent,
		       friend_discount_cap_paise, min_order_paise, monthly_cap_paise, attribution_days, terms_url
		FROM   referral_program WHERE id = 1
	`).Scan(&p.IsActive, &p.CourseRewardPaise, &p.ExamRewardPaise, &p.FriendDiscountPercent,
		&p.FriendDiscountCap, &p.MinOrderPaise, &p.MonthlyCapPaise, &p.AttributionDays, &p.TermsURL)
	return p, err
}

func (p referralProgram) rewardFor(kind string) int64 {
	if kind == referralKindExam {
		return p.ExamRewardPaise
	}
	return p.CourseRewardPaise
}

// ─── Routing ──────────────────────────────────────────────────────────────────

func (h *ReferralHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	switch path := strings.TrimRight(strings.TrimPrefix(r.URL.Path, "/api/referral"), "/"); {
	case path == "/config" && r.Method == http.MethodGet:
		h.handlePublicConfig(w, r)
	case path == "/join" && r.Method == http.MethodPost:
		h.handleJoin(w, r)
	case path == "/resolve" && r.Method == http.MethodGet:
		h.handleResolve(w, r)
	case path == "/click" && r.Method == http.MethodPost:
		h.handleClick(w, r)
	case path == "/status" && r.Method == http.MethodPost:
		h.handleStatus(w, r)
	case path == "/me" && r.Method == http.MethodGet:
		h.handleMe(w, r)
	default:
		h.fail(w, http.StatusNotFound, "not found")
	}
}

// handlePublicConfig gives the marketing site the numbers to put on the page,
// so the offer is never described in two places that can disagree.
func (h *ReferralHandler) handlePublicConfig(w http.ResponseWriter, r *http.Request) {
	p, err := h.program(r.Context())
	if err != nil {
		h.Log.Error("load referral program failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load the referral programme")
		return
	}
	h.write(w, http.StatusOK, map[string]any{
		"active":                  p.IsActive,
		"courseRewardRupees":      p.CourseRewardPaise / 100,
		"examRewardRupees":        p.ExamRewardPaise / 100,
		"friendDiscountPercent":   p.FriendDiscountPercent,
		"friendDiscountCapRupees": p.FriendDiscountCap / 100,
		"minOrderRupees":          p.MinOrderPaise / 100,
		"attributionDays":         p.AttributionDays,
		"termsUrl":                p.TermsURL,
	})
}

// ─── POST /api/referral/join ──────────────────────────────────────────────────

func (h *ReferralHandler) handleJoin(w http.ResponseWriter, r *http.Request) {
	// Everything cheap happens before anything that touches Postgres: this is a
	// public endpoint, and junk must cost us a parse, not a query.
	if !h.ipLimiter.allow(clientIP(r)) {
		h.fail(w, http.StatusTooManyRequests, "too many attempts from this network — please try again in a few minutes")
		return
	}

	var req struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Phone   string `json:"phone"`
		Website string `json:"website"` // honeypot
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&req); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request")
		return
	}
	if strings.TrimSpace(req.Website) != "" {
		// Looks like success to a bot, creates nothing.
		h.Log.Info("referral honeypot triggered", zap.String("ip", clientIP(r)))
		h.write(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	name := clip(strings.TrimSpace(req.Name), 120)
	email := strings.ToLower(clip(strings.TrimSpace(req.Email), 200))
	phone := clip(strings.TrimSpace(req.Phone), 30)
	if name == "" {
		h.fail(w, http.StatusBadRequest, "please enter your name")
		return
	}
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email {
		h.fail(w, http.StatusBadRequest, "please enter a valid email address — your link is sent there")
		return
	}
	if !h.emailLimiter.allow("referral:" + email) {
		h.fail(w, http.StatusTooManyRequests, "too many attempts for this email — please try again later")
		return
	}

	p, err := h.program(r.Context())
	if err != nil {
		h.Log.Error("load referral program failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not create your referral link")
		return
	}
	if !p.IsActive {
		h.fail(w, http.StatusServiceUnavailable, "the referral programme is not open right now")
		return
	}

	ctx := r.Context()

	// Already have a link? Hand back the same one. Two codes for one person
	// would split their referrals and their payout.
	var code string
	var blocked bool
	err = h.Pool.QueryRow(ctx, `SELECT code, is_blocked FROM referrers WHERE lower(email) = $1`, email).Scan(&code, &blocked)
	if err == nil {
		if blocked {
			// Say nothing about why. A blocked referrer who knows the rule can
			// work around it; one who does not, stops.
			h.fail(w, http.StatusForbidden, "we cannot create a referral link for this address — please contact us")
			return
		}
		h.write(w, http.StatusOK, h.linkPayload(code, name, p, false))
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		h.Log.Error("look up referrer failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not create your referral link")
		return
	}

	code, err = h.uniqueCode(ctx, name)
	if err != nil {
		h.Log.Error("generate referral code failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not create your referral link")
		return
	}

	// Link the account when this address already belongs to a learner, so the
	// student app can show them the same code.
	var userID *string
	var uid string
	if err := h.Pool.QueryRow(ctx, `SELECT id::text FROM users WHERE lower(email) = $1`, email).Scan(&uid); err == nil {
		userID = &uid
	}

	if err := h.Pool.QueryRow(ctx, `
		INSERT INTO referrers (code, name, email, phone, user_id)
		VALUES ($1, $2, $3, $4, $5::uuid)
		ON CONFLICT (email) DO UPDATE SET name = referrers.name
		RETURNING code
	`, code, name, email, phone, userID).Scan(&code); err != nil {
		h.Log.Error("create referrer failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not create your referral link")
		return
	}

	h.Log.Info("referral link created", zap.String("code", code))
	h.sendLinkEmail(name, email, code, p)
	h.write(w, http.StatusOK, h.linkPayload(code, name, p, true))
}

func (h *ReferralHandler) linkPayload(code, name string, p referralProgram, created bool) map[string]any {
	return map[string]any{
		"code":                  code,
		"link":                  h.siteBase + "/r/" + code,
		"name":                  name,
		"created":               created,
		"courseRewardRupees":    p.CourseRewardPaise / 100,
		"examRewardRupees":      p.ExamRewardPaise / 100,
		"friendDiscountPercent": p.FriendDiscountPercent,
	}
}

// uniqueCode builds a readable code from the referrer's first name plus random
// characters — "ASHA4K2P" rather than a UUID, because these get read aloud and
// typed from a WhatsApp message.
func (h *ReferralHandler) uniqueCode(ctx context.Context, name string) (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no look-alikes
	stem := strings.ToUpper(nonAlnum.ReplaceAllString(strings.Fields(name + " ")[0], ""))
	if len(stem) > 6 {
		stem = stem[:6]
	}
	if len(stem) < 3 {
		stem = "KNOV"
	}
	for attempt := 0; attempt < 8; attempt++ {
		raw, err := randomToken(8)
		if err != nil {
			return "", err
		}
		suffix := make([]byte, 4)
		for i := range suffix {
			suffix[i] = alphabet[int(raw[i])%len(alphabet)]
		}
		code := stem + string(suffix)
		var exists bool
		if err := h.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM referrers WHERE code = $1)`, code).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return code, nil
		}
	}
	return "", errors.New("could not find a free referral code")
}

// ─── GET /api/referral/resolve?code= ──────────────────────────────────────────

// handleResolve backs the "Asha sent you" banner. It returns a first name and
// nothing else: this endpoint is public to anyone holding a code, so an email
// address, a phone number or an earnings total here would be a leak.
func (h *ReferralHandler) handleResolve(w http.ResponseWriter, r *http.Request) {
	code := normalizeCode(r.URL.Query().Get("code"))
	if code == "" {
		h.fail(w, http.StatusBadRequest, "code is required")
		return
	}
	p, err := h.program(r.Context())
	if err != nil {
		h.fail(w, http.StatusInternalServerError, "could not check that link")
		return
	}

	var name string
	var blocked bool
	err = h.Pool.QueryRow(r.Context(), `SELECT name, is_blocked FROM referrers WHERE code = $1`, code).Scan(&name, &blocked)
	if errors.Is(err, pgx.ErrNoRows) || blocked || !p.IsActive {
		h.write(w, http.StatusOK, map[string]any{"valid": false})
		return
	}
	if err != nil {
		h.Log.Error("resolve referral code failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not check that link")
		return
	}
	h.write(w, http.StatusOK, map[string]any{
		"valid":                 true,
		"firstName":             strings.Fields(name + " ")[0],
		"friendDiscountPercent": p.FriendDiscountPercent,
		"attributionDays":       p.AttributionDays,
	})
}

// ─── POST /api/referral/click ─────────────────────────────────────────────────

func (h *ReferralHandler) handleClick(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request")
		return
	}
	code := normalizeCode(req.Code)
	if code == "" {
		h.fail(w, http.StatusBadRequest, "code is required")
		return
	}
	// Best effort: a click that fails to record must never block the redirect
	// that sends a visitor to the page they asked for.
	if _, err := h.Pool.Exec(r.Context(), `
		INSERT INTO referral_clicks (code, ip_hash, user_agent, path)
		SELECT $1, $2, $3, $4 WHERE EXISTS (SELECT 1 FROM referrers WHERE code = $1)
	`, code, hashIP(clientIP(r)), clip(r.UserAgent(), 300), clip(req.Path, 300)); err != nil {
		h.Log.Warn("record referral click failed", zap.Error(err))
	}
	h.write(w, http.StatusOK, map[string]any{"ok": true})
}

// ─── POST /api/referral/status ────────────────────────────────────────────────

// handleStatus lets a referrer see their own progress without an account. Both
// the code and the matching email are required: the code alone travels in every
// link they share, so it cannot be the only key to their earnings.
func (h *ReferralHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code  string `json:"code"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request")
		return
	}
	code := normalizeCode(req.Code)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if code == "" || email == "" {
		h.fail(w, http.StatusBadRequest, "enter your referral code and the email you signed up with")
		return
	}
	if !h.ipLimiter.allow("referral-status:" + clientIP(r)) {
		h.fail(w, http.StatusTooManyRequests, "too many attempts — please try again in a few minutes")
		return
	}

	var referrerID, name string
	err := h.Pool.QueryRow(r.Context(), `
		SELECT id::text, name FROM referrers WHERE code = $1 AND lower(email) = $2
	`, code, email).Scan(&referrerID, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		// One answer for a wrong code and a wrong email, so this cannot be used
		// to test which codes exist.
		h.fail(w, http.StatusUnauthorized, "that code and email do not match")
		return
	}
	if err != nil {
		h.Log.Error("referral status lookup failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load your referrals")
		return
	}

	summary, rows, err := h.referrerProgress(r.Context(), referrerID)
	if err != nil {
		h.Log.Error("referral progress failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load your referrals")
		return
	}
	summary["name"] = name
	summary["code"] = code
	summary["link"] = h.siteBase + "/r/" + code
	summary["referrals"] = rows
	h.write(w, http.StatusOK, summary)
}

// referrerProgress is the referrer's own view: their conversions, with the
// friend's name masked down to a first name. They referred the person, so they
// may see that it worked — not the address it was bought with.
func (h *ReferralHandler) referrerProgress(ctx context.Context, referrerID string) (map[string]any, []map[string]any, error) {
	rows, err := h.Pool.Query(ctx, `
		SELECT friend_name, item_name, kind, reward_paise, status, created_at, paid_at
		FROM   referral_conversions
		WHERE  referrer_id = $1::uuid AND status <> 'rejected'
		ORDER  BY created_at DESC
		LIMIT  100
	`, referrerID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	out := []map[string]any{}
	var earned, paid int64
	for rows.Next() {
		var friend, item, kind, status string
		var reward int64
		var created time.Time
		var paidAt *time.Time
		if err := rows.Scan(&friend, &item, &kind, &reward, &status, &created, &paidAt); err != nil {
			return nil, nil, err
		}
		if status != "reversed" {
			earned += reward
		}
		if status == "paid" {
			paid += reward
		}
		entry := map[string]any{
			"friend":       strings.Fields(friend + " ")[0],
			"item":         item,
			"kind":         kind,
			"rewardRupees": reward / 100,
			"status":       status,
			"at":           created.UTC().Format("2006-01-02"),
		}
		if paidAt != nil {
			entry["paidAt"] = paidAt.UTC().Format("2006-01-02")
		}
		out = append(out, entry)
	}

	var clicks int
	_ = h.Pool.QueryRow(ctx, `
		SELECT count(*) FROM referral_clicks c
		JOIN   referrers r ON r.code = c.code WHERE r.id = $1::uuid`, referrerID).Scan(&clicks)

	return map[string]any{
		"clicks":        clicks,
		"conversions":   len(out),
		"earnedRupees":  earned / 100,
		"paidRupees":    paid / 100,
		"pendingRupees": (earned - paid) / 100,
	}, out, nil
}

// ─── GET /api/referral/me ─────────────────────────────────────────────────────

// handleMe is the student app's Referral tab. A signed-in learner never fills
// in the public form: the platform already knows their name and address, so
// their code is created on first view and returned with their progress.
func (h *ReferralHandler) handleMe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		h.fail(w, http.StatusUnauthorized, "authentication required")
		return
	}
	p, err := h.program(r.Context())
	if err != nil {
		h.fail(w, http.StatusInternalServerError, "could not load your referral link")
		return
	}
	if !p.IsActive {
		// The tab renders an honest "not open yet" rather than a code that
		// would earn nothing.
		h.write(w, http.StatusOK, map[string]any{"active": false})
		return
	}

	code, name, err := h.CodeForUser(r.Context(), userID)
	if err != nil {
		h.Log.Error("referral code for user failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load your referral link")
		return
	}

	var referrerID string
	if err := h.Pool.QueryRow(r.Context(), `SELECT id::text FROM referrers WHERE code = $1`, code).Scan(&referrerID); err != nil {
		h.fail(w, http.StatusInternalServerError, "could not load your referral link")
		return
	}
	summary, rows, err := h.referrerProgress(r.Context(), referrerID)
	if err != nil {
		h.Log.Error("referral progress failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load your referrals")
		return
	}
	summary["active"] = true
	summary["name"] = name
	summary["code"] = code
	summary["link"] = h.siteBase + "/r/" + code
	summary["referrals"] = rows
	summary["courseRewardRupees"] = p.CourseRewardPaise / 100
	summary["examRewardRupees"] = p.ExamRewardPaise / 100
	summary["friendDiscountPercent"] = p.FriendDiscountPercent
	h.write(w, http.StatusOK, summary)
}

// ─── Checkout integration ─────────────────────────────────────────────────────

// referralQuote is what a checkout needs to know about a code before it prices
// an order: who gets credited, and how much comes off.
type referralQuote struct {
	ReferrerID    string
	Code          string
	DiscountPaise int64
	// Flags worth recording on the conversion for staff to look at later.
	Flags string
}

// Quote validates a referral code against a buyer and an order, and returns the
// discount to apply. An unusable code is never an error the buyer sees: it
// returns a zero discount, because a checkout must not fail over a typo in a
// referral link.
func (h *ReferralHandler) Quote(ctx context.Context, code, buyerEmail, buyerPhone, kind string, amountPaise int64) referralQuote {
	code = normalizeCode(code)
	if code == "" {
		return referralQuote{}
	}
	p, err := h.program(ctx)
	if err != nil || !p.IsActive {
		return referralQuote{}
	}

	var id, refEmail, refPhone string
	var blocked bool
	err = h.Pool.QueryRow(ctx, `SELECT id::text, lower(email), phone, is_blocked FROM referrers WHERE code = $1`, code).
		Scan(&id, &refEmail, &refPhone, &blocked)
	if err != nil || blocked {
		return referralQuote{}
	}

	buyerEmail = strings.ToLower(strings.TrimSpace(buyerEmail))
	// Self-referral: refuse outright. Nobody pays themselves a commission.
	if buyerEmail != "" && buyerEmail == refEmail {
		h.Log.Info("self-referral refused", zap.String("code", code))
		return referralQuote{}
	}

	flags := ""
	// A shared phone number is a family, a hostel room, or a person with two
	// email addresses. Worth a human look, not worth refusing.
	if digitsOnly(buyerPhone) != "" && digitsOnly(buyerPhone) == digitsOnly(refPhone) {
		flags = "same phone number as the referrer"
	}

	if amountPaise < p.MinOrderPaise {
		// The friend still gets no discount below the minimum, so the offer
		// cannot be used to make a cheap item cheaper than it is worth serving.
		return referralQuote{}
	}

	discount := amountPaise * int64(p.FriendDiscountPercent*100) / 10000
	if discount > p.FriendDiscountCap {
		discount = p.FriendDiscountCap
	}
	if discount >= amountPaise {
		discount = 0 // never a free order through a referral link
	}

	return referralQuote{ReferrerID: id, Code: code, DiscountPaise: discount, Flags: flags}
}

// RecordConversion books the reward owed on a paid order. It is idempotent on
// (kind, order_id) for the same reason fulfilment is: the browser callback and
// the payment webhook both arrive, and only one reward is owed.
func (h *ReferralHandler) RecordConversion(ctx context.Context, kind, orderID, code, friendName, friendEmail, itemName, flags string, orderPaise, discountPaise int64) {
	code = normalizeCode(code)
	if code == "" || orderID == "" {
		return
	}
	p, err := h.program(ctx)
	if err != nil || !p.IsActive {
		return
	}

	var referrerID string
	if err := h.Pool.QueryRow(ctx, `SELECT id::text FROM referrers WHERE code = $1 AND NOT is_blocked`, code).Scan(&referrerID); err != nil {
		return
	}

	reward := p.rewardFor(kind)

	// Over the monthly cap the reward is still recorded — the referral did
	// happen — but flagged, so staff decide rather than the system paying out
	// an unusual month silently.
	var lastMonth int64
	_ = h.Pool.QueryRow(ctx, `
		SELECT COALESCE(sum(reward_paise), 0) FROM referral_conversions
		WHERE  referrer_id = $1::uuid AND status IN ('approved', 'paid')
		  AND  created_at > now() - interval '30 days'
	`, referrerID).Scan(&lastMonth)
	if p.MonthlyCapPaise > 0 && lastMonth+reward > p.MonthlyCapPaise {
		flags = strings.TrimSpace(flags + "; over the monthly cap")
	}

	var convID string
	err = h.Pool.QueryRow(ctx, `
		INSERT INTO referral_conversions
			(referrer_id, code, kind, order_id, friend_name, friend_email, item_name,
			 order_paise, discount_paise, reward_paise, flags)
		VALUES ($1::uuid, $2, $3, $4::uuid, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (kind, order_id) DO NOTHING
		RETURNING id::text
	`, referrerID, code, kind, orderID, clip(friendName, 120), strings.ToLower(clip(friendEmail, 200)),
		clip(itemName, 160), orderPaise, discountPaise, reward, clip(flags, 300)).Scan(&convID)

	if errors.Is(err, pgx.ErrNoRows) {
		return // already recorded, or this friend has already earned a reward
	}
	if err != nil {
		// A unique violation on the friend index lands here: the same person
		// buying a second time does not pay the referrer twice.
		h.Log.Info("referral conversion not recorded", zap.String("code", code), zap.Error(err))
		return
	}

	h.Log.Info("referral conversion recorded",
		zap.String("code", code), zap.String("kind", kind), zap.Int64("reward_paise", reward))
	h.notifyEarned(ctx, referrerID, friendName, itemName, reward)
}

// ReverseConversion undoes a reward when the purchase behind it is refunded.
// An unpaid reward is reversed; a paid one is flagged instead, because no
// amount of SQL claws back a UPI transfer.
func (h *ReferralHandler) ReverseConversion(ctx context.Context, kind, orderID, reason string) {
	tag, err := h.Pool.Exec(ctx, `
		UPDATE referral_conversions
		   SET status = 'reversed', reason = $3, updated_at = now()
		 WHERE kind = $1 AND order_id = $2::uuid AND status IN ('pending', 'approved')
	`, kind, orderID, clip(reason, 300))
	if err != nil {
		h.Log.Warn("reverse referral conversion failed", zap.Error(err))
		return
	}
	if tag.RowsAffected() > 0 {
		return
	}
	if _, err := h.Pool.Exec(ctx, `
		UPDATE referral_conversions
		   SET flags = trim(both '; ' from flags || '; paid, then the purchase was refunded'),
		       updated_at = now()
		 WHERE kind = $1 AND order_id = $2::uuid AND status = 'paid'
	`, kind, orderID); err != nil {
		h.Log.Warn("flag paid referral conversion failed", zap.Error(err))
	}
}

// CodeForUser returns a learner's own referral code, creating one on first ask.
// This is what the student app's Referral tab calls, so a learner never fills in
// the public form for details the platform already holds.
func (h *ReferralHandler) CodeForUser(ctx context.Context, userID string) (string, string, error) {
	var code, name string
	err := h.Pool.QueryRow(ctx, `
		SELECT r.code, r.name FROM referrers r WHERE r.user_id = $1::uuid
		UNION ALL
		SELECT r.code, r.name FROM referrers r
		JOIN   users u ON lower(u.email) = lower(r.email)
		WHERE  u.id = $1::uuid
		LIMIT  1
	`, userID).Scan(&code, &name)
	if err == nil {
		// Backfill the link between account and code, so the next read is a
		// single-row lookup.
		_, _ = h.Pool.Exec(ctx, `UPDATE referrers SET user_id = $1::uuid WHERE code = $2 AND user_id IS NULL`, userID, code)
		return code, name, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", "", err
	}

	var email string
	if err := h.Pool.QueryRow(ctx, `SELECT name, email FROM users WHERE id = $1::uuid`, userID).Scan(&name, &email); err != nil {
		return "", "", err
	}
	code, err = h.uniqueCode(ctx, name)
	if err != nil {
		return "", "", err
	}
	if err := h.Pool.QueryRow(ctx, `
		INSERT INTO referrers (code, name, email, user_id) VALUES ($1, $2, $3, $4::uuid)
		ON CONFLICT (email) DO UPDATE SET user_id = EXCLUDED.user_id
		RETURNING code
	`, code, name, strings.ToLower(email), userID).Scan(&code); err != nil {
		return "", "", err
	}
	return code, name, nil
}

// ─── Email ────────────────────────────────────────────────────────────────────

func (h *ReferralHandler) sendLinkEmail(name, email, code string, p referralProgram) {
	if h.Mailer == nil || !h.Mailer.enabled() {
		h.Log.Warn("SMTP not configured — referral link logged instead of emailed",
			zap.String("email", email), zap.String("link", h.siteBase+"/r/"+code))
		return
	}
	link := h.siteBase + "/r/" + code
	text := fmt.Sprintf(`Hi %s,

Here is your Knovate referral link:

%s

Share it with anyone thinking about a course or a certification exam. They get
%.0f%% off their first purchase, and you earn ₹%d when they enrol on a course
(₹%d for an exam).

We pay by UPI once the purchase is confirmed. You can check your referrals any
time at %s/referrals/status — you will need this link's code, %s, and this email
address.

Knovate`, name, link, p.FriendDiscountPercent, p.CourseRewardPaise/100, p.ExamRewardPaise/100, h.siteBase, code)

	go func() {
		if err := h.Mailer.deliver([]string{email}, "", "Your Knovate referral link", text,
			referralLinkHTML(name, link, code, p), email); err != nil {
			h.Log.Error("referral link email failed", zap.String("email", email), zap.Error(err))
		}
	}()
}

func (h *ReferralHandler) notifyEarned(ctx context.Context, referrerID, friendName, itemName string, rewardPaise int64) {
	if h.Mailer == nil || !h.Mailer.enabled() {
		return
	}
	var name, email string
	if err := h.Pool.QueryRow(ctx, `SELECT name, email FROM referrers WHERE id = $1::uuid`, referrerID).Scan(&name, &email); err != nil {
		return
	}
	friend := strings.Fields(friendName + " ")[0]
	text := fmt.Sprintf(`Hi %s,

Good news — %s just enrolled in %s using your referral link.

You have earned ₹%d. We check each referral before paying, and send payouts by
UPI. Nothing more for you to do.

Knovate`, name, friend, itemName, rewardPaise/100)

	go func() {
		if err := h.Mailer.deliver([]string{email}, "", "You earned a referral reward", text,
			referralEarnedHTML(name, friend, itemName, rewardPaise/100), email); err != nil {
			h.Log.Warn("referral earned email failed", zap.Error(err))
		}
	}()
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// normalizeCode accepts what people actually paste: lower case, stray spaces,
// or a whole referral URL.
func normalizeCode(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "/r/"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "?#&"); i >= 0 {
		s = s[:i]
	}
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) > 24 || !referralCodeRe.MatchString(s) {
		return ""
	}
	return s
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	// Compare the last ten digits, so +91 prefixes do not hide a match.
	if len(d) > 10 {
		d = d[len(d)-10:]
	}
	return d
}

// hashIP keeps enough to spot one person clicking their own link repeatedly,
// without storing an address that identifies them.
func hashIP(ip string) string {
	if ip == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("knovate-referral:" + ip))
	return hex.EncodeToString(sum[:8])
}

func (h *ReferralHandler) write(w http.ResponseWriter, code int, v any) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *ReferralHandler) fail(w http.ResponseWriter, code int, msg string) {
	h.write(w, code, map[string]string{"error": msg})
}

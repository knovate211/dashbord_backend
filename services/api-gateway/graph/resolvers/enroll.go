package resolvers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	pkgauth "github.com/knovate211/pkg/auth"
)

// EnrollHandler sells courses online through Razorpay.
//
//	GET  /api/enroll/config   — is online payment on, and the live fee table
//	POST /api/enroll/order    — create a Razorpay order for a course + plan
//	POST /api/enroll/verify   — browser callback after Razorpay checkout
//	POST /api/enroll/webhook  — Razorpay server-to-server confirmation
//
// The price is decided here, never by the browser: the site sends a course and
// a plan, and the amount comes from courseFees. A payment only counts once its
// Razorpay signature verifies, and fulfilment (account + course access) runs
// exactly once per order however many of verify / webhook arrive — the order
// row's status moves from 'created' to 'paid' in one UPDATE, and only the
// caller that wins that update fulfils.
type EnrollHandler struct {
	Pool   *pgxpool.Pool
	Log    *zap.Logger
	Mailer *userMailer
	// Referrals prices the friend discount and books the referrer's reward.
	// nil when the referral programme is unavailable, which changes nothing
	// about buying a course.
	Referrals *ReferralHandler

	keyID, keySecret, webhookSecret string
	apiBase                         string
	client                          *http.Client
	limiter                         *rateLimiter
	emailLimiter                    *rateLimiter
}

// courseFee is one plan's price for a course, in rupees.
type courseFee struct {
	Name      string
	SelfPaced int
	MentorLed int
}

// courseFees is the authoritative price list for online payment. It MUST match
// knovate-web/src/data/pricing.ts, which is what the site displays; the site
// also shows the amount this handler returns before the student pays, so a
// drift is visible rather than silently charged.
var courseFees = map[string]courseFee{
	"5":                 {"Full Stack Development", 8999, 14999},
	"1":                 {"Java Development", 8999, 14999},
	"2":                 {"Front-End Technologies", 8999, 14999},
	"4":                 {"Golang", 8999, 14999},
	"genai":             {"GenAI & Forward Deployed Engineering", 11999, 18999},
	"3":                 {"Mastering SQL", 4999, 7999},
	"digital-marketing": {"Digital Marketing", 5999, 9999},
	"seo":               {"AI SEO Specialist", 5999, 9999},
	"testing":           {"Software Testing", 5999, 9999},
}

var planNames = map[string]string{"self": "Self-Paced", "mentor": "Mentor-Led"}

func feeFor(courseID, plan string) (rupees int, courseName string, ok bool) {
	f, found := courseFees[courseID]
	if !found {
		return 0, "", false
	}
	switch plan {
	case "self":
		return f.SelfPaced, f.Name, true
	case "mentor":
		return f.MentorLed, f.Name, true
	}
	return 0, "", false
}

// NewEnrollHandler wires the handler and ensures its table exists. Online
// payment is enabled only when both Razorpay keys are set.
func NewEnrollHandler(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger, mailer *userMailer) (*EnrollHandler, error) {
	h := &EnrollHandler{
		Pool:          pool,
		Log:           log,
		Mailer:        mailer,
		keyID:         strings.TrimSpace(os.Getenv("RAZORPAY_KEY_ID")),
		keySecret:     strings.TrimSpace(os.Getenv("RAZORPAY_KEY_SECRET")),
		webhookSecret: strings.TrimSpace(os.Getenv("RAZORPAY_WEBHOOK_SECRET")),
		apiBase:       strings.TrimRight(envOr("RAZORPAY_API_BASE", "https://api.razorpay.com"), "/"),
		client:        &http.Client{Timeout: 15 * time.Second},
		// Per-IP off by default so a campus on one NAT address is never
		// blocked; the per-email cap stops one person hammering checkout.
		limiter:      newRateLimiter(envInt("ENROLL_IP_LIMIT", 0), envMinutes("ENROLL_IP_WINDOW_MIN", 10)),
		emailLimiter: newRateLimiter(envInt("ENROLL_EMAIL_LIMIT", 10), time.Hour),
	}
	if err := h.ensureTable(ctx); err != nil {
		return nil, err
	}
	switch {
	case !h.enabled():
		log.Info("online enrolment disabled (RAZORPAY_KEY_ID / RAZORPAY_KEY_SECRET unset)")
	case strings.HasPrefix(h.keyID, "rzp_test_"):
		log.Info("online enrolment enabled in Razorpay TEST mode")
	default:
		log.Info("online enrolment enabled in Razorpay LIVE mode")
	}
	if h.enabled() && h.webhookSecret == "" {
		log.Warn("RAZORPAY_WEBHOOK_SECRET unset — enrolment relies on the browser callback alone")
	}
	return h, nil
}

func (h *EnrollHandler) enabled() bool { return h.keyID != "" && h.keySecret != "" }

func (h *EnrollHandler) ensureTable(ctx context.Context) error {
	_, err := h.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS enrollment_orders (
			id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			course_id           TEXT        NOT NULL,
			course_name         TEXT        NOT NULL,
			plan                TEXT        NOT NULL CHECK (plan IN ('self', 'mentor')),
			amount_paise        BIGINT      NOT NULL,
			name                TEXT        NOT NULL,
			email               TEXT        NOT NULL,
			phone               TEXT        NOT NULL DEFAULT '',
			razorpay_order_id   TEXT        UNIQUE,
			razorpay_payment_id TEXT,
			-- created → paid (signature verified) → enrolled (account + access
			-- granted). failed is set when fulfilment errored after payment,
			-- so staff can see a paid order that still needs attention.
			status              TEXT        NOT NULL DEFAULT 'created'
			                    CHECK (status IN ('created', 'paid', 'enrolled', 'failed')),
			user_id             UUID,
			new_account         BOOLEAN     NOT NULL DEFAULT false,
			error               TEXT        NOT NULL DEFAULT '',
			created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
			paid_at             TIMESTAMPTZ,
			updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		-- Who sent this buyer, and what it cost us. Stored on the order so
		-- attribution survives a refund, a dispute, or a support call months
		-- later — the referral row can be read back from here, not the reverse.
		ALTER TABLE enrollment_orders ADD COLUMN IF NOT EXISTS referral_code TEXT NOT NULL DEFAULT '';
		ALTER TABLE enrollment_orders ADD COLUMN IF NOT EXISTS referral_discount_paise BIGINT NOT NULL DEFAULT 0;
		ALTER TABLE enrollment_orders ADD COLUMN IF NOT EXISTS referral_flags TEXT NOT NULL DEFAULT '';
		CREATE INDEX IF NOT EXISTS idx_enroll_orders_created ON enrollment_orders(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_enroll_orders_status  ON enrollment_orders(status);
		CREATE INDEX IF NOT EXISTS idx_enroll_orders_email   ON enrollment_orders(lower(email));
	`)
	return err
}

func (h *EnrollHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	switch strings.TrimRight(r.URL.Path, "/") {
	case "/api/enroll/config":
		if r.Method == http.MethodGet {
			h.handleConfig(w)
			return
		}
	case "/api/enroll/order":
		if r.Method == http.MethodPost {
			h.handleOrder(w, r)
			return
		}
	case "/api/enroll/verify":
		if r.Method == http.MethodPost {
			h.handleVerify(w, r)
			return
		}
	case "/api/enroll/webhook":
		if r.Method == http.MethodPost {
			h.handleWebhook(w, r)
			return
		}
	}
	h.fail(w, http.StatusNotFound, "not found")
}

func (h *EnrollHandler) handleConfig(w http.ResponseWriter) {
	type fee struct {
		CourseID  string `json:"course_id"`
		Name      string `json:"name"`
		SelfPaced int    `json:"self_paced"`
		MentorLed int    `json:"mentor_led"`
	}
	fees := []fee{}
	for id, f := range courseFees {
		fees = append(fees, fee{id, f.Name, f.SelfPaced, f.MentorLed})
	}
	h.write(w, http.StatusOK, map[string]any{
		"enabled":   h.enabled(),
		"test_mode": strings.HasPrefix(h.keyID, "rzp_test_"),
		"fees":      fees,
	})
}

// POST /api/enroll/order {course_id, plan, name, email, phone}
func (h *EnrollHandler) handleOrder(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		h.fail(w, http.StatusServiceUnavailable, "online payment is not available right now — please request a callback instead")
		return
	}
	if !h.limiter.allow(clientIP(r)) {
		h.fail(w, http.StatusTooManyRequests, "too many attempts from this network — please try again in a few minutes")
		return
	}
	var req struct {
		CourseID     string `json:"course_id"`
		Plan         string `json:"plan"`
		Name         string `json:"name"`
		Email        string `json:"email"`
		Phone        string `json:"phone"`
		ReferralCode string `json:"referral_code"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&req); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request")
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
		h.fail(w, http.StatusBadRequest, "please enter a valid email address — your login is sent there")
		return
	}
	if !h.emailLimiter.allow(email) {
		h.fail(w, http.StatusTooManyRequests, "too many attempts for this email — please try again later")
		return
	}
	rupees, courseName, ok := feeFor(req.CourseID, req.Plan)
	if !ok {
		h.fail(w, http.StatusBadRequest, "unknown course or plan")
		return
	}

	ctx := r.Context()
	// Someone already on the course should sign in, not pay twice.
	var already bool
	_ = h.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM users u JOIN user_courses uc ON uc.user_id = u.id
		               WHERE lower(u.email) = $1 AND uc.course_id = $2)`, email, req.CourseID).Scan(&already)
	if already {
		h.fail(w, http.StatusConflict, "this email is already enrolled in "+courseName+" — sign in to continue learning")
		return
	}

	// The referral discount is priced here, never by the browser: the request
	// carries a code, and the server decides what — if anything — it is worth.
	amountPaise := int64(rupees) * 100
	var quote referralQuote
	if h.Referrals != nil && req.ReferralCode != "" {
		quote = h.Referrals.Quote(ctx, req.ReferralCode, email, phone, referralKindCourse, amountPaise)
		amountPaise -= quote.DiscountPaise
	}

	var orderRowID string
	if err := h.Pool.QueryRow(ctx, `
		INSERT INTO enrollment_orders (course_id, course_name, plan, amount_paise, name, email, phone,
		                               referral_code, referral_discount_paise, referral_flags)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id::text`,
		req.CourseID, courseName, req.Plan, amountPaise, name, email, phone,
		quote.Code, quote.DiscountPaise, quote.Flags).Scan(&orderRowID); err != nil {
		h.Log.Error("insert enrollment order failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not start the payment — please try again")
		return
	}

	rzpOrderID, err := h.createRazorpayOrder(ctx, amountPaise, orderRowID, map[string]string{
		"course_id": req.CourseID, "plan": req.Plan, "email": email,
	})
	if err != nil {
		h.Log.Error("create razorpay order failed", zap.Error(err))
		_, _ = h.Pool.Exec(ctx, `UPDATE enrollment_orders SET status = 'failed', error = $2, updated_at = now() WHERE id = $1::uuid`,
			orderRowID, clip(err.Error(), 500))
		h.fail(w, http.StatusBadGateway, "the payment gateway did not respond — please try again in a minute")
		return
	}
	if _, err := h.Pool.Exec(ctx, `UPDATE enrollment_orders SET razorpay_order_id = $2, updated_at = now() WHERE id = $1::uuid`,
		orderRowID, rzpOrderID); err != nil {
		h.Log.Error("store razorpay order id failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not start the payment — please try again")
		return
	}

	h.write(w, http.StatusOK, map[string]any{
		"key_id":      h.keyID,
		"order_id":    rzpOrderID,
		"amount":      amountPaise,
		"currency":    "INR",
		"course_name": courseName,
		"plan_name":   planNames[req.Plan],
		// So the order summary can show what the referral took off, and the
		// buyer can see why the total changed.
		"list_amount":       int64(rupees) * 100,
		"referral_discount": quote.DiscountPaise,
		"referral_code":     quote.Code,
		"prefill":           map[string]string{"name": name, "email": email, "contact": phone},
	})
}

func (h *EnrollHandler) createRazorpayOrder(ctx context.Context, amountPaise int64, receipt string, notes map[string]string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"amount":   amountPaise,
		"currency": "INR",
		"receipt":  receipt[:min(len(receipt), 40)],
		"notes":    notes,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.apiBase+"/v1/orders", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(h.keyID, h.keySecret)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("razorpay orders: HTTP %d: %s", resp.StatusCode, clip(string(raw), 300))
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.ID == "" {
		return "", fmt.Errorf("razorpay orders: unexpected response")
	}
	return out.ID, nil
}

// POST /api/enroll/verify {razorpay_order_id, razorpay_payment_id, razorpay_signature}
func (h *EnrollHandler) handleVerify(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		h.fail(w, http.StatusServiceUnavailable, "online payment is not available")
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
		h.Log.Warn("enrolment payment signature mismatch", zap.String("order", req.OrderID))
		h.fail(w, http.StatusBadRequest, "the payment could not be verified — if money was deducted, contact us with your payment id")
		return
	}
	res, err := h.fulfil(r.Context(), req.OrderID, req.PaymentID)
	if err != nil {
		h.Log.Error("enrolment fulfilment failed", zap.String("order", req.OrderID), zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "your payment was received but we could not finish setting up your account — our team has been notified and will contact you")
		return
	}
	h.write(w, http.StatusOK, res)
}

// POST /api/enroll/webhook — Razorpay's server-to-server notice. It is what
// completes an enrolment when the student closed the tab before the browser
// callback ran.
func (h *EnrollHandler) handleWebhook(w http.ResponseWriter, r *http.Request) {
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
					ID      string `json:"id"`
					OrderID string `json:"order_id"`
					Status  string `json:"status"`
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
		if _, err := h.fulfil(r.Context(), p.OrderID, p.ID); err != nil {
			h.Log.Error("webhook fulfilment failed", zap.String("order", p.OrderID), zap.Error(err))
			// 500 makes Razorpay retry, which is what we want for a transient error.
			h.fail(w, http.StatusInternalServerError, "fulfilment failed")
			return
		}
	}
	h.write(w, http.StatusOK, map[string]bool{"ok": true})
}

type fulfilResult struct {
	Status     string `json:"status"`
	Email      string `json:"email"`
	CourseName string `json:"course_name"`
	PlanName   string `json:"plan_name"`
	NewAccount bool   `json:"new_account"`
	Emailed    bool   `json:"emailed"`
}

// fulfil marks the order paid and enrols the student. Idempotent: only the
// first caller to move the order out of 'created' does the work; later callers
// (the webhook after the browser, or a retried verify) read back the outcome.
func (h *EnrollHandler) fulfil(ctx context.Context, rzpOrderID, paymentID string) (*fulfilResult, error) {
	var (
		rowID, courseID, courseName, plan, name, email string
		referralCode, referralFlags                    string
		amountPaise, referralDiscount                  int64
	)
	err := h.Pool.QueryRow(ctx, `
		UPDATE enrollment_orders
		   SET status = 'paid', razorpay_payment_id = $2, paid_at = now(), updated_at = now()
		 WHERE razorpay_order_id = $1
		   -- 'failed' too: a verified payment whose account setup errored is
		   -- retried by the next verify / webhook instead of staying stuck.
		   AND status IN ('created', 'failed')
		RETURNING id::text, course_id, course_name, plan, name, email,
		          referral_code, referral_discount_paise, referral_flags, amount_paise`,
		rzpOrderID, paymentID).Scan(&rowID, &courseID, &courseName, &plan, &name, &email,
		&referralCode, &referralDiscount, &referralFlags, &amountPaise)
	if errors.Is(err, pgx.ErrNoRows) {
		// Already handled (or unknown). Report the stored outcome.
		var status string
		var newAcct bool
		if err := h.Pool.QueryRow(ctx, `
			SELECT status, email, course_name, plan, new_account FROM enrollment_orders WHERE razorpay_order_id = $1`,
			rzpOrderID).Scan(&status, &email, &courseName, &plan, &newAcct); err != nil {
			return nil, fmt.Errorf("unknown order %s", rzpOrderID)
		}
		return &fulfilResult{Status: status, Email: email, CourseName: courseName, PlanName: planNames[plan], NewAccount: newAcct}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("mark order paid: %w", err)
	}

	userID, password, newAccount, err := h.ensureStudent(ctx, name, email, courseID)
	if err != nil {
		_, _ = h.Pool.Exec(context.Background(), `
			UPDATE enrollment_orders SET status = 'failed', error = $2, updated_at = now() WHERE id = $1::uuid`,
			rowID, clip(err.Error(), 500))
		return nil, err
	}
	if _, err := h.Pool.Exec(ctx, `
		UPDATE enrollment_orders SET status = 'enrolled', user_id = $2::uuid, new_account = $3, updated_at = now()
		WHERE id = $1::uuid`, rowID, userID, newAccount); err != nil {
		return nil, fmt.Errorf("mark order enrolled: %w", err)
	}

	// The welcome email carries the only copy of the password, so it is sent
	// synchronously and its real outcome reported — the success page and the
	// admin list must not claim an email went out when it did not.
	emailed := false
	if newAccount {
		if err := h.sendWelcome(name, email, password); err != nil {
			h.Log.Error("enrolment welcome email failed", zap.String("order", rzpOrderID), zap.Error(err))
			_, _ = h.Pool.Exec(context.Background(), `
				UPDATE enrollment_orders SET error = $2, updated_at = now() WHERE id = $1::uuid`,
				rowID, clip("login email not sent — reset their password from Users and share it: "+err.Error(), 500))
		} else {
			emailed = true
		}
	}
	// Booked after the student is enrolled, so a reward never exists for a
	// purchase that did not complete. Recording it is idempotent on the order,
	// which is what makes the verify/webhook race harmless here too.
	if h.Referrals != nil && referralCode != "" {
		h.Referrals.RecordConversion(ctx, referralKindCourse, rowID, referralCode,
			name, email, courseName, referralFlags, amountPaise, referralDiscount)
	}

	h.Log.Info("course purchased online",
		zap.String("course", courseID), zap.String("plan", plan), zap.Bool("new_account", newAccount))
	return &fulfilResult{Status: "enrolled", Email: email, CourseName: courseName, PlanName: planNames[plan],
		NewAccount: newAccount, Emailed: emailed}, nil
}

// ensureStudent finds or creates the student account for this email and grants
// the course. A new account gets a generated password (returned so it can be
// emailed); an existing one keeps its password. A scholarship applicant who
// buys the course becomes a student.
func (h *EnrollHandler) ensureStudent(ctx context.Context, name, email, courseID string) (userID, password string, created bool, err error) {
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return "", "", false, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var role string
	err = tx.QueryRow(ctx, `SELECT id::text, role FROM users WHERE lower(email) = $1 FOR UPDATE`, email).Scan(&userID, &role)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		password, err = generatePassword()
		if err != nil {
			return "", "", false, err
		}
		hashed, err := pkgauth.HashPassword(password)
		if err != nil {
			return "", "", false, err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO users (email, name, password, role) VALUES ($1, $2, $3, 'student') RETURNING id::text`,
			email, name, hashed).Scan(&userID); err != nil {
			return "", "", false, fmt.Errorf("create account: %w", err)
		}
		created = true
	case err != nil:
		return "", "", false, fmt.Errorf("look up account: %w", err)
	case role == "applicant":
		// A scholarship applicant's account has an unusable password; give
		// them a real one, since this is now their learning account.
		password, err = generatePassword()
		if err != nil {
			return "", "", false, err
		}
		hashed, err := pkgauth.HashPassword(password)
		if err != nil {
			return "", "", false, err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET role = 'student', password = $2, updated_at = now() WHERE id = $1::uuid`,
			userID, hashed); err != nil {
			return "", "", false, fmt.Errorf("promote applicant: %w", err)
		}
		created = true
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO user_courses (user_id, course_id) VALUES ($1::uuid, $2)
		ON CONFLICT (user_id, course_id) DO NOTHING`, userID, courseID); err != nil {
		return "", "", false, fmt.Errorf("grant course: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", "", false, err
	}
	return userID, password, created, nil
}

// sendWelcome delivers the login email, bounded so a stuck mail relay cannot
// hold the student's payment confirmation open.
func (h *EnrollHandler) sendWelcome(name, email, password string) error {
	if h.Mailer == nil || !h.Mailer.enabled() {
		return errors.New("email is not configured")
	}
	done := make(chan error, 1)
	go func() { done <- h.Mailer.deliver(name, email, password) }()
	select {
	case err := <-done:
		return err
	case <-time.After(20 * time.Second):
		return errors.New("mail relay timed out")
	}
}

// validPaymentSignature checks Razorpay's checkout signature:
// HMAC-SHA256(order_id + "|" + payment_id, key_secret), hex encoded.
func validPaymentSignature(orderID, paymentID, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(orderID + "|" + paymentID))
	return hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(strings.TrimSpace(signature)))
}

// validWebhookSignature checks HMAC-SHA256(raw body, webhook secret), hex.
func validWebhookSignature(body []byte, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(strings.TrimSpace(signature)))
}

// generatePassword makes a 12-character password without look-alike characters.
func generatePassword() (string, error) {
	const alphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 12)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b), nil
}

func (h *EnrollHandler) write(w http.ResponseWriter, code int, v any) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *EnrollHandler) fail(w http.ResponseWriter, code int, msg string) {
	h.write(w, code, map[string]string{"error": msg})
}

// ListOrders backs GET /api/admin/enrollments?status&search&page&page_size.
func (h *EnrollHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	size, _ := strconv.Atoi(q.Get("page_size"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 50
	}
	conds, args := []string{"TRUE"}, []any{}
	if s := q.Get("status"); s != "" {
		args = append(args, s)
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	if s := strings.TrimSpace(q.Get("search")); s != "" {
		args = append(args, "%"+s+"%")
		conds = append(conds, fmt.Sprintf("(name ILIKE $%d OR email ILIKE $%d OR razorpay_payment_id ILIKE $%d)", len(args), len(args), len(args)))
	}
	where := strings.Join(conds, " AND ")
	var total int
	var revenue int64
	if err := h.Pool.QueryRow(r.Context(), `
		SELECT COUNT(*), COALESCE(SUM(amount_paise) FILTER (WHERE status IN ('paid','enrolled')), 0)
		FROM enrollment_orders WHERE `+where, args...).Scan(&total, &revenue); err != nil {
		h.Log.Error("count enrollment orders failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load enrolments")
		return
	}
	args = append(args, size, (page-1)*size)
	rows, err := h.Pool.Query(r.Context(), fmt.Sprintf(`
		SELECT id::text, course_id, course_name, plan, amount_paise, name, email, phone,
		       COALESCE(razorpay_order_id, ''), COALESCE(razorpay_payment_id, ''), status, new_account, error,
		       created_at, paid_at
		FROM enrollment_orders WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, len(args)-1, len(args)), args...)
	if err != nil {
		h.Log.Error("list enrollment orders failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load enrolments")
		return
	}
	defer rows.Close()
	type row struct {
		ID         string  `json:"id"`
		CourseID   string  `json:"course_id"`
		CourseName string  `json:"course_name"`
		Plan       string  `json:"plan"`
		Amount     int64   `json:"amount"`
		Name       string  `json:"name"`
		Email      string  `json:"email"`
		Phone      string  `json:"phone"`
		OrderID    string  `json:"razorpay_order_id"`
		PaymentID  string  `json:"razorpay_payment_id"`
		Status     string  `json:"status"`
		NewAccount bool    `json:"new_account"`
		Error      string  `json:"error"`
		CreatedAt  string  `json:"created_at"`
		PaidAt     *string `json:"paid_at"`
	}
	out := []row{}
	for rows.Next() {
		var x row
		var created time.Time
		var paid *time.Time
		if err := rows.Scan(&x.ID, &x.CourseID, &x.CourseName, &x.Plan, &x.Amount, &x.Name, &x.Email, &x.Phone,
			&x.OrderID, &x.PaymentID, &x.Status, &x.NewAccount, &x.Error, &created, &paid); err != nil {
			continue
		}
		x.Amount /= 100
		x.CreatedAt = created.Format(time.RFC3339)
		if paid != nil {
			s := paid.Format(time.RFC3339)
			x.PaidAt = &s
		}
		out = append(out, x)
	}
	h.write(w, http.StatusOK, map[string]any{
		"orders": out, "total": total, "revenue": revenue / 100,
		"enabled": h.enabled(), "test_mode": strings.HasPrefix(h.keyID, "rzp_test_"),
	})
}

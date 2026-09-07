package resolvers

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pkgauth "github.com/knovate211/pkg/auth"
	"go.uber.org/zap"
)

// PasswordResetHandler runs the self-service password reset.
//
// It has two modes, and picks whichever verification it is actually capable of:
//
//   - Code mode (SMTP configured). A six-digit code is emailed and must be
//     supplied to change the password. This proves the requester controls the
//     inbox, which is what makes the reset safe.
//
//   - Direct mode (SMTP_HOST unset). The password is changed on an email
//     address alone, with nothing proving the requester owns it.
//
// DIRECT MODE IS AN ACCOUNT TAKEOVER PATH. Anyone who knows a learner's email
// address can set their password and sign in as them. It exists because this
// deployment has no mail delivery, and it is the honest consequence of that.
// Configuring SMTP_HOST switches the flow back to code mode automatically —
// nothing else needs to change. PASSWORD_RESET_ALLOW_DIRECT=false disables it
// outright even without SMTP, which turns password reset off rather than
// leaving it open.
//
// In code mode, several rules hold regardless:
//
//   - No account enumeration. /request answers identically whether or not the
//     address belongs to an account. /confirm returns the same message for a
//     wrong code, an expired code and an unknown address.
//   - The code is never stored. Only a bcrypt hash of it goes in the table, so
//     a leaked database still does not hand over live reset codes.
//   - Codes expire, are single-use, and die after a handful of wrong guesses,
//     because six digits is only a million possibilities.
//   - Both endpoints are rate limited per IP.
type PasswordResetHandler struct {
	Pool    *pgxpool.Pool
	Log     *zap.Logger
	limiter *rateLimiter
	mailer  *resetMailer
	// allowDirect permits resetting without a code. See the type comment.
	allowDirect bool
}

const (
	resetCodeTTL     = 15 * time.Minute
	resetMaxAttempts = 5
	resetMinPassword = 8
	// bcrypt ignores input past 72 bytes; reject rather than silently truncate.
	resetMaxPassword = 72
)

func NewPasswordResetHandler(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger) (*PasswordResetHandler, error) {
	mailer := newResetMailer(log)

	// Direct reset is the fallback for a deployment that cannot send mail. It
	// is never used when SMTP is available, and can be switched off entirely.
	allowDirect := !mailer.enabled()
	if v := os.Getenv("PASSWORD_RESET_ALLOW_DIRECT"); v != "" {
		allowDirect = v == "true" || v == "1"
	}

	h := &PasswordResetHandler{
		Pool: pool,
		Log:  log,
		// Tighter than the enquiry form: a reset is a rare, deliberate action,
		// but still loose enough for a shared campus NAT address.
		limiter:     newRateLimiter(10, 10*time.Minute),
		mailer:      mailer,
		allowDirect: allowDirect,
	}

	switch {
	case allowDirect:
		log.Warn("PASSWORD RESET IS UNVERIFIED — anyone who knows a user's email " +
			"address can change that user's password and sign in as them. " +
			"Set SMTP_HOST to require an emailed code, or " +
			"PASSWORD_RESET_ALLOW_DIRECT=false to disable password reset.")
	case mailer.enabled():
		log.Info("password reset requires an emailed code")
	default:
		log.Info("password reset is disabled (no SMTP and direct reset not allowed)")
	}

	if err := h.ensureTable(ctx); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *PasswordResetHandler) ensureTable(ctx context.Context) error {
	_, err := h.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS password_resets (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			email      TEXT NOT NULL,
			-- bcrypt hash of the six-digit code, never the code itself.
			code_hash  TEXT NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			used_at    TIMESTAMPTZ,
			attempts   INT NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS idx_password_resets_email ON password_resets(email, created_at DESC);
	`)
	if err != nil {
		return fmt.Errorf("create password_resets table: %w", err)
	}
	return nil
}

// ServeHTTP routes /api/password-reset/{request,confirm}. This route is public.
func (h *PasswordResetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		h.fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if !h.limiter.allow(clientIP(r)) {
		h.fail(w, http.StatusTooManyRequests, "too many attempts — please try again shortly")
		return
	}

	switch strings.TrimRight(strings.TrimPrefix(r.URL.Path, "/api/password-reset"), "/") {
	case "/request":
		h.handleRequest(w, r)
	case "/confirm":
		h.handleConfirm(w, r)
	default:
		h.fail(w, http.StatusNotFound, "not found")
	}
}

type resetRequestInput struct {
	Email string `json:"email"`
}

// handleRequest issues a code. It always reports success — see the type comment.
func (h *PasswordResetHandler) handleRequest(w http.ResponseWriter, r *http.Request) {
	var in resetRequestInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request body")
		return
	}

	email := strings.ToLower(strings.TrimSpace(clip(in.Email, 200)))
	if !looksLikeEmail(email) {
		h.fail(w, http.StatusBadRequest, "please enter a valid email address")
		return
	}

	// Direct mode has no code to send, so there is nothing to do here. The
	// client is told a code is not required and goes straight to /confirm.
	if h.allowDirect {
		h.okWith(w, map[string]any{"success": true, "code_required": false})
		return
	}

	var userID, name string
	err := h.Pool.QueryRow(r.Context(), `
		SELECT id::text, name FROM users WHERE lower(email) = $1
	`, email).Scan(&userID, &name)
	if err != nil {
		if err != pgx.ErrNoRows {
			h.Log.Error("password reset lookup failed", zap.Error(err))
		}
		// Unknown address: answer byte-for-byte as if a code had been sent.
		h.Log.Info("password reset requested for unknown email", zap.String("email", email))
		h.okWith(w, map[string]any{"success": true, "code_required": true})
		return
	}

	code, err := generateResetCode()
	if err != nil {
		h.Log.Error("generate reset code failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not start the reset, please try again")
		return
	}
	codeHash, err := pkgauth.HashPassword(code)
	if err != nil {
		h.Log.Error("hash reset code failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not start the reset, please try again")
		return
	}

	// Retire any code still outstanding for this account, so requesting a new
	// one immediately invalidates the old.
	_, _ = h.Pool.Exec(r.Context(), `
		UPDATE password_resets SET used_at = now()
		WHERE user_id = $1::uuid AND used_at IS NULL
	`, userID)

	_, err = h.Pool.Exec(r.Context(), `
		INSERT INTO password_resets (user_id, email, code_hash, expires_at)
		VALUES ($1::uuid, $2, $3, now() + $4::interval)
	`, userID, email, codeHash, fmt.Sprintf("%d seconds", int(resetCodeTTL.Seconds())))
	if err != nil {
		h.Log.Error("store reset code failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not start the reset, please try again")
		return
	}

	h.mailer.sendCode(email, name, code)
	h.okWith(w, map[string]any{"success": true, "code_required": true})
}

type resetConfirmInput struct {
	Email       string `json:"email"`
	Code        string `json:"code"`
	NewPassword string `json:"new_password"`
}

func (h *PasswordResetHandler) handleConfirm(w http.ResponseWriter, r *http.Request) {
	var in resetConfirmInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request body")
		return
	}

	email := strings.ToLower(strings.TrimSpace(clip(in.Email, 200)))
	code := strings.TrimSpace(clip(in.Code, 12))
	newPassword := in.NewPassword

	if len(newPassword) < resetMinPassword {
		h.fail(w, http.StatusBadRequest,
			fmt.Sprintf("your new password must be at least %d characters", resetMinPassword))
		return
	}
	if len(newPassword) > resetMaxPassword {
		h.fail(w, http.StatusBadRequest,
			fmt.Sprintf("your new password must be %d characters or fewer", resetMaxPassword))
		return
	}

	if h.allowDirect && code == "" {
		h.confirmDirect(w, r, email, newPassword)
		return
	}

	// One message for every failure below, so nothing distinguishes a wrong
	// code from an unknown account.
	const rejected = "that code is not valid or has expired — please request a new one"

	var resetID, userID, codeHash string
	var attempts int
	err := h.Pool.QueryRow(r.Context(), `
		SELECT id::text, user_id::text, code_hash, attempts
		FROM   password_resets
		WHERE  email = $1 AND used_at IS NULL AND expires_at > now()
		ORDER  BY created_at DESC
		LIMIT  1
	`, email).Scan(&resetID, &userID, &codeHash, &attempts)
	if err != nil {
		if err != pgx.ErrNoRows {
			h.Log.Error("password reset confirm lookup failed", zap.Error(err))
		}
		h.fail(w, http.StatusBadRequest, rejected)
		return
	}

	if attempts >= resetMaxAttempts {
		_, _ = h.Pool.Exec(r.Context(), `UPDATE password_resets SET used_at = now() WHERE id = $1::uuid`, resetID)
		h.Log.Warn("password reset code burned after too many attempts", zap.String("email", email))
		h.fail(w, http.StatusBadRequest, rejected)
		return
	}

	if ok, _ := pkgauth.CheckPassword(codeHash, code); !ok {
		_, _ = h.Pool.Exec(r.Context(), `UPDATE password_resets SET attempts = attempts + 1 WHERE id = $1::uuid`, resetID)
		h.fail(w, http.StatusBadRequest, rejected)
		return
	}

	hashed, err := pkgauth.HashPassword(newPassword)
	if err != nil {
		h.Log.Error("hash new password failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not update your password, please try again")
		return
	}

	// Consume the code in the same transaction that changes the password, so a
	// failure cannot leave a spent code usable or a used code unspent.
	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		h.Log.Error("begin password reset tx failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not update your password, please try again")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	if _, err := tx.Exec(r.Context(), `
		UPDATE users SET password = $1, updated_at = now() WHERE id = $2::uuid
	`, hashed, userID); err != nil {
		h.Log.Error("update password failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not update your password, please try again")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE password_resets SET used_at = now() WHERE id = $1::uuid
	`, resetID); err != nil {
		h.Log.Error("consume reset code failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not update your password, please try again")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		h.Log.Error("commit password reset failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not update your password, please try again")
		return
	}

	h.Log.Info("password reset completed", zap.String("email", email))
	h.ok(w)
}

// confirmDirect changes the password on an email address alone.
//
// There is no verification here — that is what direct mode means. Every reset
// is logged with the caller's IP, since an audit trail is the only control
// left once identity is not being proven.
func (h *PasswordResetHandler) confirmDirect(w http.ResponseWriter, r *http.Request, email, newPassword string) {
	var userID string
	err := h.Pool.QueryRow(r.Context(), `
		SELECT id::text FROM users WHERE lower(email) = $1
	`, email).Scan(&userID)
	if err != nil {
		if err != pgx.ErrNoRows {
			h.Log.Error("direct reset lookup failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not update your password, please try again")
			return
		}
		// Direct mode cannot hide which addresses exist — anyone can simply try
		// to reset one and see. Given that, a clear message is more useful than
		// a vague one that helps nobody.
		h.fail(w, http.StatusBadRequest, "we could not find an account with that email address")
		return
	}

	hashed, err := pkgauth.HashPassword(newPassword)
	if err != nil {
		h.Log.Error("hash new password failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not update your password, please try again")
		return
	}

	if _, err := h.Pool.Exec(r.Context(), `
		UPDATE users SET password = $1, updated_at = now() WHERE id = $2::uuid
	`, hashed, userID); err != nil {
		h.Log.Error("update password failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not update your password, please try again")
		return
	}

	h.Log.Warn("password changed without verification (direct reset mode)",
		zap.String("email", email),
		zap.String("ip", clientIP(r)))
	h.ok(w)
}

// generateResetCode returns a six-digit code from crypto/rand. math/rand would
// be predictable from a known seed, which for a reset code is the whole game.
func generateResetCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func (h *PasswordResetHandler) ok(w http.ResponseWriter) {
	h.okWith(w, map[string]any{"success": true})
}

func (h *PasswordResetHandler) okWith(w http.ResponseWriter, body map[string]any) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(body)
}

func (h *PasswordResetHandler) fail(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

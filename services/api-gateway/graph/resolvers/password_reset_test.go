package resolvers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	pkgauth "github.com/knovate211/pkg/auth"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// These exercise the real SQL against a real Postgres, because the parts most
// likely to be wrong — the interval cast on expires_at, the transaction that
// consumes the code, the single-use and attempt-limit conditions — are exactly
// the parts a mock would paper over.
//
// Set PASSWORD_RESET_TEST_DSN to run. Without it the suite skips, so `go test`
// stays green on a machine with no database.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("PASSWORD_RESET_TEST_DSN")
	if dsn == "" {
		t.Skip("PASSWORD_RESET_TEST_DSN not set — skipping database-backed reset tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// newTestHandler returns a handler plus a hook that reads back the code the
// mailer logged. With SMTP unset the mailer logs the code instead of sending
// it, which is the documented offline behaviour — and it is how the test learns
// what code to submit.
func newTestHandler(t *testing.T, pool *pgxpool.Pool) (*PasswordResetHandler, func() string) {
	t.Helper()

	// JSON logs into a buffer, rather than zaptest/observer — that would mean
	// vendoring a package the production build never needs.
	buf := &bytes.Buffer{}
	log := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(buf),
		zapcore.DebugLevel,
	))

	// SMTP off so the code is logged rather than sent, but direct reset forced
	// off too — these tests are about the code path specifically.
	t.Setenv("SMTP_HOST", "")
	t.Setenv("PASSWORD_RESET_ALLOW_DIRECT", "false")

	h, err := NewPasswordResetHandler(context.Background(), pool, log)
	if err != nil {
		t.Fatalf("NewPasswordResetHandler: %v", err)
	}
	if h.allowDirect {
		t.Fatal("expected code mode")
	}

	// Returns the most recently logged code, so each step reads the code its
	// own request produced rather than a stale one.
	lastCode := func() string {
		var found string
		for _, line := range bytes.Split(buf.Bytes(), []byte("\n")) {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var entry struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(line, &entry); err != nil {
				continue
			}
			if entry.Code != "" {
				found = entry.Code
			}
		}
		return found
	}
	return h, lastCode
}

func seedUser(t *testing.T, pool *pgxpool.Pool, email, password string) {
	t.Helper()
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email      TEXT NOT NULL UNIQUE,
			name       TEXT NOT NULL,
			password   TEXT NOT NULL,
			role       TEXT NOT NULL DEFAULT 'student',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
	`)
	if err != nil {
		t.Fatalf("create users table: %v", err)
	}

	hashed, err := pkgauth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO users (email, name, password, role) VALUES ($1, 'Test Learner', $2, 'student')
		ON CONFLICT (email) DO UPDATE SET password = EXCLUDED.password
	`, email, hashed)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func post(t *testing.T, h http.Handler, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.RemoteAddr = "203.0.113.7:12345"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func storedPassword(t *testing.T, pool *pgxpool.Pool, email string) string {
	t.Helper()
	var pw string
	if err := pool.QueryRow(context.Background(),
		`SELECT password FROM users WHERE email = $1`, email).Scan(&pw); err != nil {
		t.Fatalf("read password: %v", err)
	}
	return pw
}

func TestPasswordResetHappyPath(t *testing.T) {
	pool := testPool(t)
	email := "happy@example.com"
	seedUser(t, pool, email, "old-password-1")
	h, lastCode := newTestHandler(t, pool)

	if rec := post(t, h, "/api/password-reset/request", map[string]any{"email": email}); rec.Code != http.StatusOK {
		t.Fatalf("request: got %d, body %s", rec.Code, rec.Body.String())
	}

	code := lastCode()
	if !regexp.MustCompile(`^\d{6}$`).MatchString(code) {
		t.Fatalf("expected a six-digit code, got %q", code)
	}

	rec := post(t, h, "/api/password-reset/confirm", map[string]any{
		"email": email, "code": code, "new_password": "brand-new-password",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm: got %d, body %s", rec.Code, rec.Body.String())
	}

	stored := storedPassword(t, pool, email)
	if !pkgauth.IsHashed(stored) {
		t.Fatal("new password was not stored as a hash")
	}
	if ok, _ := pkgauth.CheckPassword(stored, "brand-new-password"); !ok {
		t.Fatal("new password does not verify against the stored hash")
	}
	if ok, _ := pkgauth.CheckPassword(stored, "old-password-1"); ok {
		t.Fatal("old password still works after reset")
	}
}

func TestPasswordResetCodeIsSingleUse(t *testing.T) {
	pool := testPool(t)
	email := "single-use@example.com"
	seedUser(t, pool, email, "old-password-1")
	h, lastCode := newTestHandler(t, pool)

	post(t, h, "/api/password-reset/request", map[string]any{"email": email})
	code := lastCode()

	first := post(t, h, "/api/password-reset/confirm", map[string]any{
		"email": email, "code": code, "new_password": "first-new-password",
	})
	if first.Code != http.StatusOK {
		t.Fatalf("first confirm should succeed: %d %s", first.Code, first.Body.String())
	}

	second := post(t, h, "/api/password-reset/confirm", map[string]any{
		"email": email, "code": code, "new_password": "second-new-password",
	})
	if second.Code == http.StatusOK {
		t.Fatal("a spent code was accepted a second time")
	}
	if ok, _ := pkgauth.CheckPassword(storedPassword(t, pool, email), "second-new-password"); ok {
		t.Fatal("password was changed by a replayed code")
	}
}

func TestPasswordResetWrongCodeRejectedAndBurnsAfterLimit(t *testing.T) {
	pool := testPool(t)
	email := "attempts@example.com"
	seedUser(t, pool, email, "old-password-1")
	h, lastCode := newTestHandler(t, pool)

	post(t, h, "/api/password-reset/request", map[string]any{"email": email})
	realCode := lastCode()

	wrong := "000000"
	if wrong == realCode {
		wrong = "111111"
	}

	for i := 0; i < resetMaxAttempts; i++ {
		rec := post(t, h, "/api/password-reset/confirm", map[string]any{
			"email": email, "code": wrong, "new_password": "should-not-apply",
		})
		if rec.Code == http.StatusOK {
			t.Fatalf("wrong code accepted on attempt %d", i+1)
		}
	}

	// The limit is spent, so even the correct code must now be refused.
	rec := post(t, h, "/api/password-reset/confirm", map[string]any{
		"email": email, "code": realCode, "new_password": "should-not-apply",
	})
	if rec.Code == http.StatusOK {
		t.Fatal("correct code still worked after the attempt limit was exhausted")
	}
	if ok, _ := pkgauth.CheckPassword(storedPassword(t, pool, email), "should-not-apply"); ok {
		t.Fatal("password changed despite exhausted attempts")
	}
}

func TestPasswordResetRequestingAgainInvalidatesTheOldCode(t *testing.T) {
	pool := testPool(t)
	email := "reissue@example.com"
	seedUser(t, pool, email, "old-password-1")
	h, lastCode := newTestHandler(t, pool)

	post(t, h, "/api/password-reset/request", map[string]any{"email": email})
	firstCode := lastCode()

	post(t, h, "/api/password-reset/request", map[string]any{"email": email})

	rec := post(t, h, "/api/password-reset/confirm", map[string]any{
		"email": email, "code": firstCode, "new_password": "using-stale-code",
	})
	if rec.Code == http.StatusOK {
		t.Fatal("superseded code was still accepted")
	}
}

func TestPasswordResetExpiredCodeRejected(t *testing.T) {
	pool := testPool(t)
	email := "expired@example.com"
	seedUser(t, pool, email, "old-password-1")
	h, lastCode := newTestHandler(t, pool)

	post(t, h, "/api/password-reset/request", map[string]any{"email": email})
	code := lastCode()

	// Age the code past its TTL rather than waiting fifteen minutes.
	if _, err := pool.Exec(context.Background(), `
		UPDATE password_resets SET expires_at = now() - interval '1 minute'
		WHERE email = $1 AND used_at IS NULL
	`, email); err != nil {
		t.Fatalf("expire code: %v", err)
	}

	rec := post(t, h, "/api/password-reset/confirm", map[string]any{
		"email": email, "code": code, "new_password": "too-late-password",
	})
	if rec.Code == http.StatusOK {
		t.Fatal("expired code was accepted")
	}
}

// An unknown address must be indistinguishable from a known one, or the
// endpoint becomes a way to find out who has an account.
func TestPasswordResetDoesNotRevealWhetherAccountExists(t *testing.T) {
	pool := testPool(t)
	known := "known@example.com"
	seedUser(t, pool, known, "old-password-1")
	h, _ := newTestHandler(t, pool)

	a := post(t, h, "/api/password-reset/request", map[string]any{"email": known})
	b := post(t, h, "/api/password-reset/request", map[string]any{"email": "nobody-here@example.com"})

	if a.Code != b.Code {
		t.Fatalf("status differs: known %d vs unknown %d", a.Code, b.Code)
	}
	if a.Body.String() != b.Body.String() {
		t.Fatalf("body differs: known %q vs unknown %q", a.Body.String(), b.Body.String())
	}
}

func TestPasswordResetRejectsShortPassword(t *testing.T) {
	pool := testPool(t)
	email := "short@example.com"
	seedUser(t, pool, email, "old-password-1")
	h, lastCode := newTestHandler(t, pool)

	post(t, h, "/api/password-reset/request", map[string]any{"email": email})

	rec := post(t, h, "/api/password-reset/confirm", map[string]any{
		"email": email, "code": lastCode(), "new_password": "short",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("short password should be rejected, got %d", rec.Code)
	}
}

// ── Direct mode ───────────────────────────────────────────────────────────────
// SMTP_HOST unset, so there is no code to send and the password changes on the
// email address alone.

func newDirectHandler(t *testing.T, pool *pgxpool.Pool) *PasswordResetHandler {
	t.Helper()
	t.Setenv("SMTP_HOST", "")
	t.Setenv("PASSWORD_RESET_ALLOW_DIRECT", "true")
	h, err := NewPasswordResetHandler(context.Background(), pool, zap.NewNop())
	if err != nil {
		t.Fatalf("NewPasswordResetHandler: %v", err)
	}
	if !h.allowDirect {
		t.Fatal("expected direct mode with SMTP_HOST unset")
	}
	return h
}

func TestDirectResetChangesPasswordWithoutACode(t *testing.T) {
	pool := testPool(t)
	email := "direct@example.com"
	seedUser(t, pool, email, "old-password-1")
	h := newDirectHandler(t, pool)

	// The client asks first and is told no code is needed.
	rec := post(t, h, "/api/password-reset/request", map[string]any{"email": email})
	if rec.Code != http.StatusOK {
		t.Fatalf("request: got %d", rec.Code)
	}
	var reqBody struct {
		CodeRequired bool `json:"code_required"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &reqBody); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if reqBody.CodeRequired {
		t.Fatal("direct mode should report code_required=false")
	}

	rec = post(t, h, "/api/password-reset/confirm", map[string]any{
		"email": email, "new_password": "direct-new-password",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm: got %d, body %s", rec.Code, rec.Body.String())
	}

	stored := storedPassword(t, pool, email)
	if !pkgauth.IsHashed(stored) {
		t.Fatal("direct reset stored the password unhashed")
	}
	if ok, _ := pkgauth.CheckPassword(stored, "direct-new-password"); !ok {
		t.Fatal("new password does not verify after a direct reset")
	}
	if ok, _ := pkgauth.CheckPassword(stored, "old-password-1"); ok {
		t.Fatal("old password still works after a direct reset")
	}
}

func TestDirectResetRejectsUnknownEmailAndShortPassword(t *testing.T) {
	pool := testPool(t)
	h := newDirectHandler(t, pool)
	seedUser(t, pool, "known-direct@example.com", "old-password-1")

	if rec := post(t, h, "/api/password-reset/confirm", map[string]any{
		"email": "ghost@example.com", "new_password": "long-enough-password",
	}); rec.Code == http.StatusOK {
		t.Fatal("direct reset succeeded for an email with no account")
	}

	if rec := post(t, h, "/api/password-reset/confirm", map[string]any{
		"email": "known-direct@example.com", "new_password": "short",
	}); rec.Code != http.StatusBadRequest {
		t.Fatalf("short password should be rejected, got %d", rec.Code)
	}
}

// Configuring SMTP must take the direct path away, so adding mail delivery
// closes the hole without any other change.
func TestSMTPConfiguredDisablesDirectReset(t *testing.T) {
	pool := testPool(t)
	email := "smtp-on@example.com"
	seedUser(t, pool, email, "old-password-1")

	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("PASSWORD_RESET_ALLOW_DIRECT", "")
	h, err := NewPasswordResetHandler(context.Background(), pool, zap.NewNop())
	if err != nil {
		t.Fatalf("NewPasswordResetHandler: %v", err)
	}
	if h.allowDirect {
		t.Fatal("direct reset must be off when SMTP is configured")
	}

	rec := post(t, h, "/api/password-reset/confirm", map[string]any{
		"email": email, "new_password": "should-not-apply",
	})
	if rec.Code == http.StatusOK {
		t.Fatal("codeless reset accepted while SMTP was configured")
	}
	if ok, _ := pkgauth.CheckPassword(storedPassword(t, pool, email), "should-not-apply"); ok {
		t.Fatal("password changed without a code while SMTP was configured")
	}
}

func TestGenerateResetCodeShape(t *testing.T) {
	six := regexp.MustCompile(`^\d{6}$`)
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		c, err := generateResetCode()
		if err != nil {
			t.Fatalf("generateResetCode: %v", err)
		}
		if !six.MatchString(c) {
			t.Fatalf("code %q is not six digits", c)
		}
		seen[c] = true
	}
	// Not a randomness test — just a guard against a constant or a stuck source.
	if len(seen) < 100 {
		t.Fatalf("only %d distinct codes in 200 draws — generator looks degenerate", len(seen))
	}
}

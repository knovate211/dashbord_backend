package resolvers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

// certHandler builds a handler with a nil pool.
//
// Same trick as the scholarship tests: any code path that reaches Postgres
// panics, so these prove a rejection happened *before* the database was
// touched. On a public, paid endpoint that ordering is the difference between a
// cheap 400 and free work for anyone who can send a request. SMTP_HOST is unset
// in tests, so the mailer is disabled and never dials anything.
func certHandler() *CertificationHandler {
	return &CertificationHandler{
		Log:          zap.NewNop(),
		jwtSecret:    "test-secret",
		appBase:      "http://portal.test",
		siteBase:     "http://site.test",
		pay:          &cashfree{appID: "TEST_APP", secret: "test_secret", env: "sandbox"},
		Mailer:       &scholarshipMailer{log: zap.NewNop()},
		ipLimiter:    newRateLimiter(1000, time.Minute),
		emailLimiter: newRateLimiter(1000, time.Minute),
	}
}

func postCert(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestOrderRejectsBeforeTouchingTheDatabase(t *testing.T) {
	h := certHandler()

	tests := []struct {
		name string
		body string
		want int
		why  string
	}{
		{
			"honeypot is answered like a success",
			`{"name":"Bot","email":"bot@example.com","slug":"java","website":"http://spam"}`,
			http.StatusOK,
			"a bot that fills the hidden field must learn nothing from the response — and must not get an order",
		},
		{"missing name", `{"email":"a@b.com","slug":"java"}`, http.StatusBadRequest, ""},
		{"missing email", `{"name":"A","slug":"java"}`, http.StatusBadRequest, ""},
		{"nonsense email", `{"name":"A","email":"not-an-email","slug":"java"}`, http.StatusBadRequest, ""},
		{"email with a display name is refused", `{"name":"A","email":"A <a@b.com>","slug":"java"}`, http.StatusBadRequest,
			"the address is used verbatim as the login, so it must be exactly what was typed"},
		{"broken json", `{`, http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postCert(t, h, "/api/certification/order", tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.want, tt.why)
			}
		})
	}
}

func TestOrderRefusedWhenPaymentsAreOff(t *testing.T) {
	h := certHandler()
	h.pay = &cashfree{}
	rec := postCert(t, h, "/api/certification/order", `{"name":"A","email":"a@b.com","slug":"java"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 — an exam must never look purchasable when Cashfree is unconfigured", rec.Code)
	}
}

func TestClaimRejectsBeforeTouchingTheDatabase(t *testing.T) {
	h := certHandler()
	for _, body := range []string{`{}`, `{"token":""}`, `{`} {
		rec := postCert(t, h, "/api/certification/claim", body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %q: status = %d, want 400", body, rec.Code)
		}
	}
}

func TestVerifyRefusesAnOrderThatIsNotAnExam(t *testing.T) {
	h := certHandler()
	for _, body := range []string{`{}`, `{"order_id":""}`, `{"order_id":"enr_x"}`, `{`} {
		rec := postCert(t, h, "/api/certification/verify", body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %q: status = %d, want 400 — only an exam order may reach the gateway lookup", body, rec.Code)
		}
	}
}

func TestWebhookRejectsAForgedSignature(t *testing.T) {
	h := certHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/certification/webhook",
		strings.NewReader(`{"type":"PAYMENT_SUCCESS_WEBHOOK","data":{"order":{"order_id":"cert_1"}}}`))
	req.Header.Set("x-webhook-timestamp", "1700000000000")
	req.Header.Set("x-webhook-signature", "Zm9yZ2Vk")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 — an unsigned payment notice must never reach fulfilment", rec.Code)
	}
}

func TestWebhookIsInvisibleWhenPaymentsAreOff(t *testing.T) {
	h := certHandler()
	h.pay = &cashfree{}
	rec := postCert(t, h, "/api/certification/webhook", `{"type":"PAYMENT_SUCCESS_WEBHOOK"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 — an unconfigured webhook must not accept payloads", rec.Code)
	}
}

func TestUnknownRouteIs404(t *testing.T) {
	h := certHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/certification/nope", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestCredentialIDIsReadableAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		id, err := newCredentialID()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(id, "KNV-") || len(id) != 13 {
			t.Fatalf("credential %q is not in the KNV-XXXX-XXXX shape", id)
		}
		// People read these off a printed certificate, so the alphabet must
		// exclude characters that look alike.
		if strings.ContainsAny(id, "OI01") {
			t.Fatalf("credential %q contains a look-alike character", id)
		}
		if seen[id] {
			t.Fatalf("duplicate credential id %q after %d draws", id, i)
		}
		seen[id] = true
	}
}

func TestTerminalStatusesCoverEveryEndState(t *testing.T) {
	// A status missing here would let a spent exam link be claimed again.
	for _, s := range []string{"submitted", "passed", "failed", "expired", "refunded"} {
		if !certTerminal[s] {
			t.Errorf("%q should be terminal", s)
		}
	}
	for _, s := range []string{"created", "paid", "started"} {
		if certTerminal[s] {
			t.Errorf("%q must not be terminal — the candidate has not sat the exam yet", s)
		}
	}
}

func TestOutcomeNeedsAnAttempt(t *testing.T) {
	h := certHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/certification/outcome", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] == "" {
		t.Fatal("expected an error message the result page can show")
	}
}

func TestRoundTo(t *testing.T) {
	for _, tt := range []struct {
		in   float64
		want float64
		// 59.995 is not exactly representable in binary — it is stored a hair
		// below — so it rounds down. Recorded here so nobody "fixes" it later:
		// a score one ten-thousandth under the line is not a pass.
	}{{66.666666, 66.67}, {100, 100}, {0, 0}, {59.995, 59.99}, {59.996, 60}} {
		if got := roundTo(tt.in, 2); got != tt.want {
			t.Errorf("roundTo(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

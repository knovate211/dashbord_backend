package resolvers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

// referralTestHandler has a nil pool: anything that reaches Postgres panics, so
// these prove the rejection happened before the database was touched. On a
// public endpoint that hands out something worth money, that ordering matters.
func referralTestHandler() *ReferralHandler {
	return &ReferralHandler{
		Log:          zap.NewNop(),
		siteBase:     "https://site.test",
		Mailer:       &scholarshipMailer{log: zap.NewNop()},
		ipLimiter:    newRateLimiter(1000, time.Minute),
		emailLimiter: newRateLimiter(1000, time.Minute),
	}
}

func TestNormalizeCodeAcceptsWhatPeoplePaste(t *testing.T) {
	cases := map[string]string{
		"ASHA4K2P":                           "ASHA4K2P",
		"asha4k2p":                           "ASHA4K2P",
		"  ASHA4K2P  ":                       "ASHA4K2P",
		"https://knovate.com/r/ASHA4K2P":     "ASHA4K2P",
		"https://knovate.com/r/asha4k2p?x=1": "ASHA4K2P",
		"knovate.com/r/ASHA4K2P#share":       "ASHA4K2P",
		"":                                   "",
		"not a code":                         "",
		"ASHA-4K2P":                          "", // the generator never emits a hyphen
		strings.Repeat("A", 40):              "", // absurdly long
	}
	for in, want := range cases {
		if got := normalizeCode(in); got != want {
			t.Errorf("normalizeCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDigitsOnlyComparesTheLastTen(t *testing.T) {
	// A referrer who stored "+91 98765 43210" and a buyer who typed
	// "9876543210" are the same phone, and the self-referral check has to see
	// that.
	if digitsOnly("+91 98765 43210") != digitsOnly("9876543210") {
		t.Fatal("country code should not hide a matching number")
	}
	if digitsOnly("98765 43211") == digitsOnly("9876543210") {
		t.Fatal("different numbers must not compare equal")
	}
	if digitsOnly("") != "" {
		t.Fatal("empty stays empty — a missing number is not a match")
	}
}

func TestHashIPIsStableAndNotReversible(t *testing.T) {
	a, b := hashIP("203.0.113.9"), hashIP("203.0.113.9")
	if a != b {
		t.Fatal("the same address must hash the same, or click counting is useless")
	}
	if strings.Contains(a, "203.0.113.9") || a == "" {
		t.Fatal("the stored value must not contain the address")
	}
	if hashIP("") != "" {
		t.Fatal("no address, nothing to store")
	}
}

func TestRewardDependsOnWhatWasBought(t *testing.T) {
	p := referralProgram{CourseRewardPaise: 50000, ExamRewardPaise: 20000}
	if got := p.rewardFor(referralKindCourse); got != 50000 {
		t.Errorf("course reward = %d, want 50000", got)
	}
	if got := p.rewardFor(referralKindExam); got != 20000 {
		t.Errorf("exam reward = %d, want 20000", got)
	}
	// An unknown kind must not pay the larger amount by accident; it falls back
	// to the course reward deliberately, which is the documented default.
	if got := p.rewardFor("something-else"); got != 50000 {
		t.Errorf("fallback reward = %d, want the course reward", got)
	}
}

func postReferral(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestJoinRejectsBeforeTouchingTheDatabase(t *testing.T) {
	h := referralTestHandler()
	for _, tt := range []struct {
		name, body string
		want       int
	}{
		{"broken json", `{`, http.StatusBadRequest},
		{"no name", `{"email":"a@b.com"}`, http.StatusBadRequest},
		{"no email", `{"name":"Asha"}`, http.StatusBadRequest},
		{"nonsense email", `{"name":"Asha","email":"nope"}`, http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// program() hits the database, so a handler that got this far would
			// panic — which is the point: validation comes first.
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("reached the database before validating: %v", r)
				}
			}()
			rec := postReferral(t, h, "/api/referral/join", tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestStatusNeedsBothCodeAndEmail(t *testing.T) {
	h := referralTestHandler()
	// The code travels in every link the referrer shares, so it cannot be the
	// only key to their earnings.
	for _, body := range []string{`{}`, `{"code":"ASHA4K2P"}`, `{"email":"a@b.com"}`, `{`} {
		rec := postReferral(t, h, "/api/referral/status", body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %q: status = %d, want 400", body, rec.Code)
		}
	}
}

func TestResolveNeedsACode(t *testing.T) {
	h := referralTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/referral/resolve", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestMeRequiresASession(t *testing.T) {
	h := referralTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/referral/me", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestUnknownReferralRouteIs404(t *testing.T) {
	h := referralTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/referral/nope", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

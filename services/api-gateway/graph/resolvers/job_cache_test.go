package resolvers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/graphql-go/graphql"
	"go.uber.org/zap"
)

func TestJobSearchReusesRecentResults(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"status":"OK","data":[{"job_title":"Backend Developer","employer_name":"Acme"}]}`))
	}))
	defer srv.Close()

	clock := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	c := NewJobClients(zap.NewNop())
	c.apiKey, c.endpoint = "test", srv.URL
	c.now = func() time.Time { return clock }

	search := func(keywords string) {
		t.Helper()
		_, err := c.SearchJobs(graphql.ResolveParams{Context: context.Background(),
			Args: map[string]interface{}{"keywords": keywords, "page": 1}})
		if err != nil {
			t.Fatal(err)
		}
	}

	search("Backend Developer")
	search("backend developer") // same search, different case
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("repeated search hit the API %d times, want 1", got)
	}
	search("Data Scientist")
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("a different search should call the API; calls = %d", got)
	}
	clock = clock.Add(jobCacheTTL + time.Second)
	search("Backend Developer")
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("an expired result should be fetched again; calls = %d", got)
	}
}

func TestJobSearchDoesNotCacheFailures(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := NewJobClients(zap.NewNop())
	c.apiKey, c.endpoint = "test", srv.URL
	args := map[string]interface{}{"keywords": "Go", "page": 1}
	for i := 0; i < 2; i++ {
		if _, err := c.SearchJobs(graphql.ResolveParams{Context: context.Background(), Args: args}); err == nil {
			t.Fatal("want the rate-limit error")
		}
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("a failed search was served from cache; calls = %d", got)
	}
}

// Learners never see our configuration: a missing key or a broken upstream
// gives the same friendly message; a rate limit still tells them to wait.
func TestJobSearchErrorsAreStudentSafe(t *testing.T) {
	args := map[string]interface{}{"keywords": "Go", "page": 1}
	search := func(c *JobClients) error {
		_, err := c.SearchJobs(graphql.ResolveParams{Context: context.Background(), Args: args})
		return err
	}

	unconfigured := NewJobClients(zap.NewNop())
	unconfigured.apiKey = ""
	if err := search(unconfigured); err != errJobsUnavailable {
		t.Fatalf("missing key: got %v", err)
	}

	for status, want := range map[int]error{
		http.StatusForbidden:           errJobsUnavailable,
		http.StatusInternalServerError: errJobsUnavailable,
		http.StatusTooManyRequests:     errJobsRateLimited,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		c := NewJobClients(zap.NewNop())
		c.apiKey, c.endpoint = "test", srv.URL
		if err := search(c); err != want {
			t.Errorf("status %d: got %v, want %v", status, err, want)
		}
		srv.Close()
	}
	for _, e := range []error{errJobsUnavailable, errJobsRateLimited} {
		if strings.Contains(e.Error(), "JSEARCH") || strings.Contains(e.Error(), "API key") {
			t.Errorf("message names our configuration: %q", e)
		}
	}
}

package resolvers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/graphql-go/graphql"
)

func TestJobSearchReusesRecentResults(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"status":"OK","data":[{"job_title":"Backend Developer","employer_name":"Acme"}]}`))
	}))
	defer srv.Close()

	clock := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	c := NewJobClients()
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

	c := NewJobClients()
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

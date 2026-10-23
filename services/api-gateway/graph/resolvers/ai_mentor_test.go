package resolvers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"
)

// fakeGemini answers each model with a scripted list of status codes, then 200.
func fakeGemini(t *testing.T, script map[string][]int) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-goog-api-key") != "test-key" {
			t.Errorf("api key not sent in header")
		}
		if r.URL.Query().Get("key") != "" {
			t.Errorf("api key leaked into the query string")
		}
		model := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), ":generateContent")
		mu.Lock()
		calls = append(calls, model)
		var code int
		if len(script[model]) > 0 {
			code, script[model] = script[model][0], script[model][1:]
		}
		mu.Unlock()
		if code != 0 {
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "busy"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []any{map[string]any{
				"content": map[string]any{"parts": []any{
					map[string]any{"text": "thinking...", "thought": true},
					map[string]any{"text": "answer from " + model},
				}},
				"finishReason": "STOP",
			}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func newTestMentor(baseURL string) *AIMentorHandler {
	return &AIMentorHandler{
		Log: zap.NewNop(), apiKey: "test-key", baseURL: baseURL + "/",
		model: "main", fallbackModel: "lite", thinkingLevel: "low", http: http.DefaultClient,
	}
}

var oneTurn = []geminiContent{{Role: "user", Parts: []geminiPart{{Text: "hi"}}}}

func TestMentorRetriesThenFallsBack(t *testing.T) {
	srv, calls := fakeGemini(t, map[string][]int{"main": {503, 503}})
	reply, err := newTestMentor(srv.URL).generate(context.Background(), "sys", oneTurn)
	if err != nil {
		t.Fatal(err)
	}
	if reply != "answer from lite" {
		t.Fatalf("reply = %q, want the fallback's answer without the thought part", reply)
	}
	if got := strings.Join(*calls, ","); got != "main,main,lite" {
		t.Fatalf("calls = %s", got)
	}
}

func TestMentorRetrySucceeds(t *testing.T) {
	srv, calls := fakeGemini(t, map[string][]int{"main": {503}})
	reply, err := newTestMentor(srv.URL).generate(context.Background(), "sys", oneTurn)
	if err != nil || reply != "answer from main" {
		t.Fatalf("reply = %q, err = %v", reply, err)
	}
	if len(*calls) != 2 {
		t.Fatalf("calls = %v", *calls)
	}
}

func TestMentorDoesNotRetryClientErrors(t *testing.T) {
	srv, calls := fakeGemini(t, map[string][]int{"main": {400}})
	if _, err := newTestMentor(srv.URL).generate(context.Background(), "sys", oneTurn); err == nil {
		t.Fatal("want an error")
	}
	if len(*calls) != 1 {
		t.Fatalf("a 400 was retried: %v", *calls)
	}
}

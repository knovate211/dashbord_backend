package resolvers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
)

// AIMentorHandler answers the course pages' AI mentor chat through Gemini.
//
// The browser used to call the model provider itself with a key built into the
// JavaScript bundle, which would publish the key to every visitor. The key now
// stays here, and so does the system prompt: a client that could set its own
// prompt could use the endpoint as a free general-purpose model on our bill.
// The client sends only the conversation.
type AIMentorHandler struct {
	Log *zap.Logger

	apiKey        string
	baseURL       string // Gemini's models endpoint; tests point it at a fake
	model         string
	fallbackModel string
	thinkingLevel string
	http          *http.Client

	perMinute *rateLimiter
	perDay    *rateLimiter
}

const (
	geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models/"

	// Bounds on what one request may carry. Older turns past mentorMaxTurns are
	// dropped rather than rejected, so a long chat keeps working.
	mentorMaxTurns       = 20
	mentorMaxMessageSize = 4000
	mentorMaxCourseSize  = 120
	mentorMaxOutput      = 2048

	// Each model call gets its own deadline so a retry and a fallback still fit
	// in the time a learner will wait.
	mentorAttemptTimeout = 20 * time.Second
)

const mentorSystemPrompt = `You are Lali AI Mentor, a friendly and encouraging AI mentor embedded in an online course about programming and software development.

Your two jobs:
1. EXPLAIN: When a student asks a question, explain concepts clearly, with short code examples where useful. Match your depth to what they seem to already know. Use plain language before jargon.
2. QUIZ: Periodically, or when asked, quiz the student with a short question to check understanding. Ask one question at a time, wait for their answer, then give feedback before moving on. Mix in occasional quick checks even when not explicitly asked, if it fits naturally after an explanation.

Style rules:
- Keep responses concise and scannable. Use short paragraphs and code blocks, not walls of text.
- Be encouraging but honest — if an answer is wrong, say so clearly and explain why, then help them get it right.
- Never do a student's graded assignment for them wholesale; guide them toward the answer with questions and hints instead.
- If asked something outside programming/software topics, gently redirect back to the course material.`

// NewAIMentorHandler reads its configuration from the environment. Without
// GEMINI_API_KEY the route still answers, with a clear "not configured" error,
// so the chat widget can say so instead of failing silently.
func NewAIMentorHandler(log *zap.Logger) *AIMentorHandler {
	model := strings.TrimSpace(os.Getenv("GEMINI_MODEL"))
	if model == "" {
		model = "gemini-3.5-flash"
	}
	thinking := strings.TrimSpace(os.Getenv("GEMINI_THINKING_LEVEL"))
	if thinking == "" {
		// Thinking tokens count against the output budget; a mentor reply does
		// not need deep reasoning, and "low" keeps answers fast and complete.
		thinking = "low"
	}
	fallback := strings.TrimSpace(os.Getenv("GEMINI_FALLBACK_MODEL"))
	if fallback == "" {
		fallback = "gemini-3.5-flash-lite"
	}
	return &AIMentorHandler{
		Log:           log,
		apiKey:        strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
		baseURL:       geminiBaseURL,
		model:         model,
		fallbackModel: fallback,
		thinkingLevel: thinking,
		http:          &http.Client{},
		perMinute:     newRateLimiter(10, time.Minute),
		perDay:        newRateLimiter(200, 24*time.Hour),
	}
}

func (h *AIMentorHandler) Enabled() bool { return h.apiKey != "" }

func (h *AIMentorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		writeErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if strings.TrimRight(r.URL.Path, "/") != "/api/ai-mentor/chat" || r.Method != http.MethodPost {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if !h.Enabled() {
		writeErr(w, http.StatusServiceUnavailable, "The AI mentor is not configured yet.")
		return
	}
	if !h.perMinute.allow(userID) || !h.perDay.allow(userID) {
		writeErr(w, http.StatusTooManyRequests, "You're sending messages quickly — please wait a moment and try again.")
		return
	}

	var body struct {
		Course   string `json:"course"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(body.Messages) == 0 || body.Messages[len(body.Messages)-1].Role != "user" {
		writeErr(w, http.StatusBadRequest, "the conversation must end with your message")
		return
	}
	if len(body.Messages) > mentorMaxTurns {
		body.Messages = body.Messages[len(body.Messages)-mentorMaxTurns:]
	}

	contents := make([]geminiContent, 0, len(body.Messages))
	for _, m := range body.Messages {
		text := strings.TrimSpace(m.Content)
		if text == "" {
			continue
		}
		if len(text) > mentorMaxMessageSize {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("messages are limited to %d characters", mentorMaxMessageSize))
			return
		}
		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}
		contents = append(contents, geminiContent{Role: role, Parts: []geminiPart{{Text: text}}})
	}
	// A trimmed history can begin with a model turn; Gemini expects the user first.
	for len(contents) > 0 && contents[0].Role != "user" {
		contents = contents[1:]
	}
	if len(contents) == 0 {
		writeErr(w, http.StatusBadRequest, "empty message")
		return
	}

	prompt := mentorSystemPrompt
	if course := strings.TrimSpace(body.Course); course != "" {
		if len(course) > mentorMaxCourseSize {
			course = course[:mentorMaxCourseSize]
		}
		prompt += "\n\nThe student is currently studying the course: " + course + "."
	}

	reply, err := h.generate(r.Context(), prompt, contents)
	if err != nil {
		h.Log.Error("ai mentor generate failed", zap.Error(err), zap.String("model", h.model))
		writeErr(w, http.StatusBadGateway, "The mentor couldn't answer just now. Please try again.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"reply": reply})
}

type geminiPart struct {
	Text    string `json:"text"`
	Thought bool   `json:"thought,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

// errMentorBlocked is a reply the model withheld (safety filters and the like).
var errMentorBlocked = errors.New("reply withheld by the model")

// errMentorBusy is Gemini saying it is overloaded or rate-limiting us. It is
// usually brief ("high demand" 503s), so it is worth another attempt.
var errMentorBusy = errors.New("model busy")

// generate asks the configured model, retries it once after a pause when it is
// busy, and then tries the lighter fallback model, which tends to have spare
// capacity when the main one is saturated.
func (h *AIMentorHandler) generate(ctx context.Context, systemPrompt string, contents []geminiContent) (string, error) {
	attempts := []string{h.model, h.model}
	if h.fallbackModel != "" && h.fallbackModel != h.model {
		attempts = append(attempts, h.fallbackModel)
	}
	var err error
	for i, model := range attempts {
		if i == 1 {
			select {
			case <-time.After(1500 * time.Millisecond):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		attemptCtx, cancel := context.WithTimeout(ctx, mentorAttemptTimeout)
		var reply string
		reply, err = h.callModel(attemptCtx, model, systemPrompt, contents)
		cancel()
		if err == nil {
			return reply, nil
		}
		if !errors.Is(err, errMentorBusy) && !errors.Is(err, context.DeadlineExceeded) {
			return "", err
		}
		h.Log.Warn("ai mentor model busy, retrying", zap.String("model", model), zap.Error(err))
	}
	return "", err
}

func (h *AIMentorHandler) callModel(ctx context.Context, model, systemPrompt string, contents []geminiContent) (string, error) {
	payload, err := json.Marshal(map[string]any{
		"systemInstruction": geminiContent{Parts: []geminiPart{{Text: systemPrompt}}},
		"contents":          contents,
		"generationConfig": map[string]any{
			"maxOutputTokens": mentorMaxOutput,
			"thinkingConfig":  map[string]string{"thinkingLevel": h.thinkingLevel},
		},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		h.baseURL+model+":generateContent", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	// In a header, not the ?key= query string, so it never lands in access logs.
	req.Header.Set("x-goog-api-key", h.apiKey)

	resp, err := h.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}

	var out struct {
		Candidates []struct {
			Content      geminiContent `json:"content"`
			FinishReason string        `json:"finishReason"`
		} `json:"candidates"`
		PromptFeedback struct {
			BlockReason string `json:"blockReason"`
		} `json:"promptFeedback"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("gemini %d: unreadable response", resp.StatusCode)
	}
	if out.Error != nil || resp.StatusCode != http.StatusOK {
		msg := ""
		if out.Error != nil {
			msg = out.Error.Message
		}
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusInternalServerError {
			return "", fmt.Errorf("%w: gemini %d: %s", errMentorBusy, resp.StatusCode, msg)
		}
		return "", fmt.Errorf("gemini %d: %s", resp.StatusCode, msg)
	}
	if out.PromptFeedback.BlockReason != "" || len(out.Candidates) == 0 {
		return "", errMentorBlocked
	}

	var sb strings.Builder
	for _, p := range out.Candidates[0].Content.Parts {
		if !p.Thought {
			sb.WriteString(p.Text)
		}
	}
	reply := strings.TrimSpace(sb.String())
	if reply == "" {
		return "", fmt.Errorf("%w (finish reason %s)", errMentorBlocked, out.Candidates[0].FinishReason)
	}
	return reply, nil
}

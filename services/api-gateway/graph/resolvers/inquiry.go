package resolvers

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// InquiryHandler captures leads from the public marketing site and serves them
// back to the admin panel.
//
// POST /api/inquiries is the only unauthenticated write endpoint in the
// gateway — it has to be, since the people filling in a contact form have no
// account. That makes two things non-negotiable: every field is length-capped
// before it reaches the database, and submissions are rate limited per IP.
//
// It talks to Postgres directly rather than through a service, matching the
// AdminHandler pattern. An inquiry is a flat row with no domain logic; routing
// it through a new gRPC service would be plumbing for its own sake.
type InquiryHandler struct {
	Pool         *pgxpool.Pool
	Log          *zap.Logger
	limiter      *rateLimiter
	emailLimiter *rateLimiter
	mailer       *mailer
}

// NewInquiryHandler wires the handler and ensures its table exists.
func NewInquiryHandler(ctx context.Context, pool *pgxpool.Pool, log *zap.Logger) (*InquiryHandler, error) {
	h := &InquiryHandler{
		Pool: pool,
		Log:  log,
		// Deliberately generous, because the audience is students: a whole
		// college campus can sit behind one NAT address, so a tight per-IP cap
		// would lock out real enquiries during a campaign. The honeypot does
		// most of the anti-bot work; this only stops a runaway flood.
		// Per-IP limiting is OFF by default: a whole campus can sit behind one
		// NAT address, and blocking it during a drive loses real enquiries.
		// Abuse is handled by the honeypot and the per-email cap below.
		// INQUIRY_IP_LIMIT > 0 turns an IP cap back on.
		limiter:      newRateLimiter(envInt("INQUIRY_IP_LIMIT", 0), envMinutes("INQUIRY_IP_WINDOW_MIN", 10)),
		emailLimiter: newRateLimiter(envInt("INQUIRY_EMAIL_LIMIT", 5), time.Hour),
		mailer:       newMailer(log),
	}
	if err := h.ensureTable(ctx); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *InquiryHandler) ensureTable(ctx context.Context) error {
	_, err := h.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS inquiries (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name       TEXT NOT NULL,
			email      TEXT NOT NULL,
			phone      TEXT NOT NULL DEFAULT '',
			whatsapp   TEXT NOT NULL DEFAULT '',
			interest   TEXT NOT NULL DEFAULT '',
			message    TEXT NOT NULL DEFAULT '',
			-- Which form it came from: contact | demo | register | other.
			source     TEXT NOT NULL DEFAULT 'other',
			page_url   TEXT NOT NULL DEFAULT '',
			status     TEXT NOT NULL DEFAULT 'new'
			           CHECK (status IN ('new', 'contacted', 'closed', 'spam')),
			notes      TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE INDEX IF NOT EXISTS idx_inquiries_created ON inquiries(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_inquiries_status  ON inquiries(status);

		-- CREATE TABLE IF NOT EXISTS is a no-op once the table exists, so new
		-- columns need their own ALTER to reach databases created earlier.
		ALTER TABLE inquiries ADD COLUMN IF NOT EXISTS whatsapp TEXT NOT NULL DEFAULT '';

		-- Company leads for the hiring-test platform (the /hire page).
		-- Empty for learner enquiries.
		ALTER TABLE inquiries ADD COLUMN IF NOT EXISTS company       TEXT NOT NULL DEFAULT '';
		ALTER TABLE inquiries ADD COLUMN IF NOT EXISTS job_title     TEXT NOT NULL DEFAULT '';
		ALTER TABLE inquiries ADD COLUMN IF NOT EXISTS company_size  TEXT NOT NULL DEFAULT '';
		ALTER TABLE inquiries ADD COLUMN IF NOT EXISTS hiring_volume TEXT NOT NULL DEFAULT '';
	`)
	if err != nil {
		return fmt.Errorf("create inquiries table: %w", err)
	}
	return nil
}

// ─── Public submission ────────────────────────────────────────────────────────

type inquiryInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	WhatsApp string `json:"whatsapp"`
	Interest string `json:"interest"`
	Message  string `json:"message"`
	Source   string `json:"source"`
	PageURL  string `json:"page_url"`
	// Company fields, sent only by the hiring-platform form.
	Company      string `json:"company"`
	JobTitle     string `json:"job_title"`
	CompanySize  string `json:"company_size"`
	HiringVolume string `json:"hiring_volume"`
	// Honeypot: a field hidden from real users. Bots fill every input they
	// find, so anything arriving here is discarded — while still returning
	// success, so the bot has no signal to adapt to.
	Website string `json:"website"`
}

// ServeHTTP handles POST /api/inquiries. This route is public.
func (h *InquiryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		h.fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var in inquiryInput
	// Cap the body before decoding — an unauthenticated endpoint should never
	// read an unbounded request.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if in.Website != "" {
		h.Log.Info("inquiry honeypot triggered", zap.String("ip", clientIP(r)))
		h.ok(w) // look identical to success
		return
	}

	name := clip(in.Name, 120)
	email := strings.ToLower(clip(in.Email, 200))
	if name == "" || !looksLikeEmail(email) {
		h.fail(w, http.StatusBadRequest, "please provide your name and a valid email address")
		return
	}

	if !h.limiter.allow(clientIP(r)) || !h.emailLimiter.allow(email) {
		h.fail(w, http.StatusTooManyRequests, "too many submissions — please try again shortly")
		return
	}

	_, err := h.Pool.Exec(r.Context(), `
		INSERT INTO inquiries (name, email, phone, whatsapp, interest, message, source, page_url,
		                       company, job_title, company_size, hiring_volume)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, name, email, clip(in.Phone, 40), clip(in.WhatsApp, 40), clip(in.Interest, 120),
		clip(in.Message, 4000), normalizeSource(in.Source), clip(in.PageURL, 300),
		clip(in.Company, 160), clip(in.JobTitle, 120), clip(in.CompanySize, 40), clip(in.HiringVolume, 40))
	if err != nil {
		h.Log.Error("save inquiry failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not submit your enquiry, please try again")
		return
	}

	h.Log.Info("inquiry received", zap.String("source", normalizeSource(in.Source)), zap.String("email", email))
	// Fired after the row is committed, and asynchronously — a slow or broken
	// SMTP server must never delay or fail the visitor's submission.
	h.mailer.notify(in, name, email)
	h.ok(w)
}

func normalizeSource(s string) string {
	v := strings.ToLower(strings.TrimSpace(s))
	// Preserve any concise, safe source slug so the marketing site can tag which
	// form/page a lead came from (e.g. "home_hero", "course_full-stack",
	// "pricing", "contact"). Anything unexpected collapses to "other".
	if v == "" {
		return "other"
	}
	if len(v) > 40 || !sourceSlug.MatchString(v) {
		return "other"
	}
	return v
}

var sourceSlug = regexp.MustCompile(`^[a-z0-9_-]+$`)

// looksLikeEmail is a deliberately loose check. Strict validation rejects real
// addresses; the only thing worth catching here is obvious junk.
func looksLikeEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	if at < 1 || at == len(s)-1 {
		return false
	}
	return strings.Contains(s[at+1:], ".") && !strings.ContainsAny(s, " \t\n")
}

func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max]
	}
	return s
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (h *InquiryHandler) ok(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

func (h *InquiryHandler) fail(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ─── Admin listing ────────────────────────────────────────────────────────────

type inquiryRow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	WhatsApp  string `json:"whatsapp"`
	Interest  string `json:"interest"`
	Message   string `json:"message"`
	Source    string `json:"source"`
	PageURL   string `json:"page_url"`
	Status    string `json:"status"`
	Notes     string `json:"notes"`
	CreatedAt string `json:"created_at"`

	Company      string `json:"company"`
	JobTitle     string `json:"job_title"`
	CompanySize  string `json:"company_size"`
	HiringVolume string `json:"hiring_volume"`
}

// ListInquiries backs GET /api/admin/inquiries.
func (h *InquiryHandler) ListInquiries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}

	where, args := inquiryWhere(q)

	var total int
	if err := h.Pool.QueryRow(r.Context(), "SELECT COUNT(*) FROM inquiries "+where, args...).Scan(&total); err != nil {
		h.Log.Error("count inquiries failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load enquiries")
		return
	}

	rows, err := h.Pool.Query(r.Context(), fmt.Sprintf(`
		SELECT id::text, name, email, phone, whatsapp, interest, message, source, page_url, status, notes, created_at,
		       company, job_title, company_size, hiring_volume
		FROM   inquiries %s
		ORDER  BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, len(args)+1, len(args)+2), append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		h.Log.Error("list inquiries failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not load enquiries")
		return
	}
	defer rows.Close()

	out := []inquiryRow{}
	for rows.Next() {
		var i inquiryRow
		var created time.Time
		if err := rows.Scan(&i.ID, &i.Name, &i.Email, &i.Phone, &i.WhatsApp, &i.Interest, &i.Message,
			&i.Source, &i.PageURL, &i.Status, &i.Notes, &created,
			&i.Company, &i.JobTitle, &i.CompanySize, &i.HiringVolume); err != nil {
			h.Log.Error("scan inquiry failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not load enquiries")
			return
		}
		i.CreatedAt = created.Format(time.RFC3339)
		out = append(out, i)
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"inquiries": out, "total": total, "page": page, "pageSize": pageSize,
	})
}

// inquiryWhere builds the filter clause shared by the enquiry list, its CSV
// export and the bulk actions, so "delete all matching" removes exactly the
// rows the admin was looking at.
func inquiryWhere(q url.Values) (string, []any) {
	var clauses []string
	var args []any
	add := func(format string, v any) {
		args = append(args, v)
		clauses = append(clauses, fmt.Sprintf(format, len(args)))
	}
	if s := q.Get("status"); s != "" {
		add("status = $%d", s)
	}
	if s := q.Get("source"); s != "" {
		add("source = $%d", s)
	}
	if s := q.Get("interest"); s != "" {
		add("interest = $%d", s)
	}
	if s := strings.TrimSpace(q.Get("search")); s != "" {
		args = append(args, s)
		clauses = append(clauses, fmt.Sprintf(
			"(name ILIKE '%%' || $%d || '%%' OR email ILIKE '%%' || $%d || '%%' OR phone ILIKE '%%' || $%d || '%%' OR company ILIKE '%%' || $%d || '%%')",
			len(args), len(args), len(args), len(args)))
	}
	if d, err := time.Parse("2006-01-02", q.Get("from")); err == nil {
		add("created_at >= $%d", d)
	}
	if d, err := time.Parse("2006-01-02", q.Get("to")); err == nil {
		add("created_at < $%d", d.AddDate(0, 0, 1))
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

// UpdateInquiry backs PATCH /api/admin/inquiries/{id} — status and notes only.
func (h *InquiryHandler) UpdateInquiry(w http.ResponseWriter, r *http.Request, id string) {
	var body struct {
		Status string `json:"status"`
		Notes  string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	switch body.Status {
	case "new", "contacted", "closed", "spam":
	default:
		h.fail(w, http.StatusBadRequest, "status must be new, contacted, closed or spam")
		return
	}

	tag, err := h.Pool.Exec(r.Context(), `
		UPDATE inquiries SET status = $2, notes = $3, updated_at = now() WHERE id = $1::uuid
	`, id, body.Status, clip(body.Notes, 2000))
	if err != nil {
		h.Log.Error("update inquiry failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not update the enquiry")
		return
	}
	if tag.RowsAffected() == 0 {
		h.fail(w, http.StatusNotFound, "enquiry not found")
		return
	}
	h.ok(w)
}

// ─── Rate limiting ────────────────────────────────────────────────────────────

// rateLimiter is a small in-memory sliding window keyed by IP.
//
// In-memory means it resets on deploy and is per-instance rather than shared —
// acceptable for a contact form, where the goal is stopping casual flooding,
// not defeating a determined attacker. Move it to Redis if the gateway is ever
// run multi-replica.
type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
}

// newRateLimiter returns nil — no limit at all — when limit <= 0, so any cap
// can be switched off from configuration.
func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	if limit <= 0 {
		return nil
	}
	return &rateLimiter{hits: make(map[string][]time.Time), limit: limit, window: window}
}

func (l *rateLimiter) allow(key string) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}

	// Drop empty keys so a long-running process does not accumulate one entry
	// per IP that ever submitted.
	if len(kept) == 0 {
		delete(l.hits, key)
	} else {
		l.hits[key] = kept
	}

	if len(kept) >= l.limit {
		return false
	}
	l.hits[key] = append(l.hits[key], now)
	return true
}

package resolvers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/knovate211/api-gateway/middleware"
	"github.com/knovate211/pkg/ids"
	assessmentv1 "github.com/knovate211/proto/assessment/v1"
)

// IntegrityHandler collects the evidence the browser proctor cannot judge on
// its own, and turns it into a per-candidate risk rating for reviewers.
//
// From the candidate's browser (authenticated, own live attempt only):
//
//	POST /api/integrity/heartbeat   device session + IP; catches a second device
//	POST /api/integrity/snapshots   editor snapshots, for code playback
//
// For recruiters and admins (company-scoped):
//
//	GET /api/integrity/assessments/{id}   whole-test report: risk per candidate,
//	                                      similar code, shared wrong answers,
//	                                      shared IPs
//	GET /api/integrity/attempts/{id}      one candidate: signals, sessions and
//	                                      the snapshots behind code playback
//
// Nothing here fails a candidate. Everything is evidence with a reason
// attached, for a person to weigh — the same stance the browser proctor takes.
type IntegrityHandler struct {
	Pool          *pgxpool.Pool
	AssessmentSvc assessmentv1.AssessmentServiceClient
	Log           *zap.Logger
}

// NewIntegrityHandler wires the handler and ensures its tables exist.
func NewIntegrityHandler(ctx context.Context, pool *pgxpool.Pool, svc assessmentv1.AssessmentServiceClient, log *zap.Logger) (*IntegrityHandler, error) {
	h := &IntegrityHandler{Pool: pool, AssessmentSvc: svc, Log: log}
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS attempt_sessions (
			attempt_id  UUID NOT NULL REFERENCES attempts(id) ON DELETE CASCADE,
			session_id  TEXT NOT NULL,
			ip          TEXT NOT NULL DEFAULT '',
			user_agent  TEXT NOT NULL DEFAULT '',
			screen      TEXT NOT NULL DEFAULT '',
			first_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
			last_seen   TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (attempt_id, session_id)
		);
		CREATE INDEX IF NOT EXISTS idx_attempt_sessions_ip ON attempt_sessions (ip);

		CREATE TABLE IF NOT EXISTS code_snapshots (
			id          BIGSERIAL PRIMARY KEY,
			attempt_id  UUID NOT NULL REFERENCES attempts(id) ON DELETE CASCADE,
			question_id UUID NOT NULL,
			at          TIMESTAMPTZ NOT NULL,
			language    TEXT NOT NULL DEFAULT '',
			code        TEXT NOT NULL,
			-- start | edit | language | run | submit
			reason      TEXT NOT NULL DEFAULT 'edit',
			-- Characters inserted since the previous snapshot of this question.
			chars_added INT NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_code_snapshots_q ON code_snapshots (attempt_id, question_id, at);
	`)
	if err != nil {
		return nil, fmt.Errorf("create integrity tables: %w", err)
	}
	return h, nil
}

func (h *IntegrityHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	role := middleware.RoleFromContext(r.Context())
	userID := middleware.UserIDFromContext(r.Context())
	if userID == "" {
		h.fail(w, http.StatusUnauthorized, "authentication required")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/integrity"), "/")
	seg := strings.Split(path, "/")

	switch {
	case path == "heartbeat" && r.Method == http.MethodPost:
		h.heartbeat(w, r, userID)
	case path == "snapshots" && r.Method == http.MethodPost:
		h.snapshots(w, r, userID)
	case len(seg) == 2 && seg[0] == "assessments" && r.Method == http.MethodGet:
		if !h.mayReview(w, r.Context(), role, userID, seg[1]) {
			return
		}
		rep, err := h.buildReport(r.Context(), seg[1])
		if err != nil {
			h.Log.Error("integrity report failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not build the integrity report")
			return
		}
		h.json(w, http.StatusOK, rep)
	case len(seg) == 2 && seg[0] == "attempts" && r.Method == http.MethodGet:
		h.attemptDetail(w, r, role, userID, seg[1])
	default:
		h.fail(w, http.StatusNotFound, "unknown integrity endpoint")
	}
}

// ─── Candidate side ───────────────────────────────────────────────────────────

// liveAttempt confirms the attempt is the caller's and still running.
func (h *IntegrityHandler) liveAttempt(ctx context.Context, attemptID, userID string) error {
	// Checked here so the queries below — and those after a successful call —
	// can compare uuid columns directly and use their indexes.
	if !ids.IsUUID(attemptID) {
		return errors.New("attempt not found")
	}
	var owner, status string
	err := h.Pool.QueryRow(ctx, `SELECT user_id::text, status FROM attempts WHERE id = $1`, attemptID).Scan(&owner, &status)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && owner != userID) {
		return errors.New("attempt not found")
	}
	if err != nil {
		return err
	}
	if status != "in_progress" {
		return errors.New("attempt is not in progress")
	}
	return nil
}

// heartbeat records which browser session is sitting the attempt, and from
// where. Two sessions alive at once means the test is open on two devices or
// in two tabs; a new IP mid-test means the network changed (or someone else
// took over). Both become proctor events, so they land in the same timeline
// and integrity score as everything else.
func (h *IntegrityHandler) heartbeat(w http.ResponseWriter, r *http.Request, userID string) {
	var body struct {
		AttemptID string `json:"attemptId"`
		SessionID string `json:"sessionId"`
		Screen    string `json:"screen"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil || body.AttemptID == "" || body.SessionID == "" {
		h.fail(w, http.StatusBadRequest, "attemptId and sessionId are required")
		return
	}
	ctx := r.Context()
	if err := h.liveAttempt(ctx, body.AttemptID, userID); err != nil {
		h.fail(w, http.StatusNotFound, err.Error())
		return
	}
	ip := clientIP(r)
	session := clip(body.SessionID, 80)

	var isNew bool
	if err := h.Pool.QueryRow(ctx, `
		INSERT INTO attempt_sessions (attempt_id, session_id, ip, user_agent, screen)
		VALUES ($1::uuid, $2, $3, $4, $5)
		ON CONFLICT (attempt_id, session_id) DO UPDATE
			SET last_seen = now(), ip = EXCLUDED.ip, screen = EXCLUDED.screen
		RETURNING (xmax = 0)
	`, body.AttemptID, session, ip, clip(r.UserAgent(), 300), clip(body.Screen, 60)).Scan(&isNew); err != nil {
		h.Log.Error("record attempt session failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not record session")
		return
	}

	if isNew {
		var others int
		var otherIPs []string
		_ = h.Pool.QueryRow(ctx, `
			SELECT COUNT(*) FILTER (WHERE last_seen > now() - interval '90 seconds'),
			       COALESCE(array_agg(DISTINCT ip) FILTER (WHERE ip <> $3), '{}')
			FROM   attempt_sessions WHERE attempt_id = $1 AND session_id <> $2
		`, body.AttemptID, session, ip).Scan(&others, &otherIPs)
		if others > 0 {
			h.event(ctx, body.AttemptID, userID, "duplicate_session", "test opened in another tab or device")
		}
		if len(otherIPs) > 0 {
			h.event(ctx, body.AttemptID, userID, "ip_change", "network changed to "+ip)
		}
	}
	h.json(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *IntegrityHandler) event(ctx context.Context, attemptID, userID, kind, detail string) {
	if h.AssessmentSvc == nil {
		return
	}
	if _, err := h.AssessmentSvc.RecordProctorEvent(ctx, &assessmentv1.RecordProctorEventRequest{
		AttemptId: attemptID, UserId: userID, Kind: kind, Detail: detail,
	}); err != nil {
		h.Log.Warn("record integrity event failed", zap.String("kind", kind), zap.Error(err))
	}
}

type snapshotIn struct {
	At       int64  `json:"at"` // ms since epoch, client clock
	Language string `json:"language"`
	Code     string `json:"code"`
	Reason   string `json:"reason"`
}

const (
	maxSnapshotsPerCall     = 200
	maxSnapshotsPerQuestion = 3000
	maxSnapshotBytes        = 100 << 10
)

// snapshots stores the editor history behind code playback. Each snapshot
// records how many characters appeared since the previous one, which is what
// the report reads to spot a whole solution landing at once.
func (h *IntegrityHandler) snapshots(w http.ResponseWriter, r *http.Request, userID string) {
	var body struct {
		AttemptID  string       `json:"attemptId"`
		QuestionID string       `json:"questionId"`
		Snapshots  []snapshotIn `json:"snapshots"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20)).Decode(&body); err != nil || body.AttemptID == "" || body.QuestionID == "" {
		h.fail(w, http.StatusBadRequest, "attemptId, questionId and snapshots are required")
		return
	}
	ctx := r.Context()
	if err := h.liveAttempt(ctx, body.AttemptID, userID); err != nil {
		h.fail(w, http.StatusNotFound, err.Error())
		return
	}
	var kind string
	if !ids.IsUUID(body.QuestionID) {
		h.fail(w, http.StatusNotFound, "question not found")
		return
	}
	if err := h.Pool.QueryRow(ctx, `SELECT kind FROM attempt_questions WHERE id = $1 AND attempt_id = $2`,
		body.QuestionID, body.AttemptID).Scan(&kind); err != nil || kind != "coding" {
		h.fail(w, http.StatusNotFound, "question not found")
		return
	}
	if len(body.Snapshots) > maxSnapshotsPerCall {
		body.Snapshots = body.Snapshots[:maxSnapshotsPerCall]
	}

	var prev string
	var stored int
	err := h.Pool.QueryRow(ctx, `
		SELECT COALESCE((SELECT code FROM code_snapshots WHERE attempt_id = $1 AND question_id = $2 ORDER BY at DESC, id DESC LIMIT 1), ''),
		       (SELECT COUNT(*) FROM code_snapshots WHERE attempt_id = $1 AND question_id = $2)
	`, body.AttemptID, body.QuestionID).Scan(&prev, &stored)
	if err != nil {
		h.fail(w, http.StatusInternalServerError, "could not store snapshots")
		return
	}

	now := time.Now().UTC()
	batch := &pgx.Batch{}
	for _, s := range body.Snapshots {
		if stored >= maxSnapshotsPerQuestion {
			break
		}
		code := s.Code
		if len(code) > maxSnapshotBytes {
			code = code[:maxSnapshotBytes]
		}
		at := time.UnixMilli(s.At).UTC()
		// The client clock is only trusted to order its own snapshots; a
		// timestamp far from now is clamped to the server's time.
		if s.At == 0 || at.After(now.Add(time.Minute)) || at.Before(now.Add(-6*time.Hour)) {
			at = now
		}
		reason := s.Reason
		switch reason {
		case "start", "edit", "language", "run", "submit":
		default:
			reason = "edit"
		}
		batch.Queue(`
			INSERT INTO code_snapshots (attempt_id, question_id, at, language, code, reason, chars_added)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7)
		`, body.AttemptID, body.QuestionID, at, clip(s.Language, 20), code, reason, insertedChars(prev, code))
		prev = code
		stored++
	}
	if batch.Len() > 0 {
		if err := h.Pool.SendBatch(ctx, batch).Close(); err != nil {
			h.Log.Error("store code snapshots failed", zap.Error(err))
			h.fail(w, http.StatusInternalServerError, "could not store snapshots")
			return
		}
	}
	h.json(w, http.StatusOK, map[string]any{"stored": batch.Len()})
}

// insertedChars is the size of the edit between two versions: what is left
// after trimming the prefix and suffix they share. A candidate typing adds a
// few characters per snapshot; a pasted or generated solution adds hundreds.
func insertedChars(prev, next string) int {
	a, b := []rune(prev), []rune(next)
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	return len(b) - p - s
}

// trapToken is the per-question marker hidden in the problem statement (see
// CodingQuestionView). Invisible on screen, it rides along when the statement
// is copied into a chatbot, which then tends to use it in the answer. FNV-1a
// on both sides, so the browser and the server agree without sharing a secret
// — the marker is not a secret, only a tell.
func trapToken(attemptID, questionID string) string {
	f := fnv.New32a()
	_, _ = f.Write([]byte(attemptID + ":" + questionID))
	return "kv_" + strconv.FormatUint(uint64(f.Sum32()), 36)
}

// ─── Reviewer side ────────────────────────────────────────────────────────────

// mayReview allows admins any test, and recruiters their own company's.
func (h *IntegrityHandler) mayReview(w http.ResponseWriter, ctx context.Context, role, userID, assessmentID string) bool {
	if role != "admin" && role != "recruiter" {
		h.fail(w, http.StatusForbidden, "reviewer access required")
		return false
	}
	var allowed bool
	err := h.Pool.QueryRow(ctx, `
		SELECT $2 = 'admin' OR EXISTS (
		         SELECT 1 FROM company_members m
		         WHERE  m.company_id = a.company_id AND m.user_id::text = $3)
		FROM   assessments a WHERE a.id::text = $1
	`, assessmentID, role, userID).Scan(&allowed)
	if errors.Is(err, pgx.ErrNoRows) {
		h.fail(w, http.StatusNotFound, "test not found")
		return false
	}
	if err != nil {
		h.fail(w, http.StatusInternalServerError, "could not check access")
		return false
	}
	if !allowed {
		h.fail(w, http.StatusForbidden, "not a member of this company")
		return false
	}
	return true
}

type person struct {
	AttemptID string `json:"attempt_id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
}

type signal struct {
	Severity string `json:"severity"` // high | medium | low
	Text     string `json:"text"`
	Points   int    `json:"-"`
}

type attemptRisk struct {
	person
	Status         string   `json:"status"`
	IntegrityScore float64  `json:"integrity_score"`
	Risk           string   `json:"risk"` // high | medium | low
	RiskPoints     int      `json:"risk_points"`
	Signals        []signal `json:"signals"`
	IPs            []string `json:"ips"`
	Sessions       int      `json:"sessions"`
}

type similarPair struct {
	QuestionTitle string `json:"question_title"`
	Language      string `json:"language"`
	A             person `json:"a"`
	B             person `json:"b"`
	Percent       int    `json:"percent"`
}

type collusionPair struct {
	A           person `json:"a"`
	B           person `json:"b"`
	SharedWrong int    `json:"shared_wrong"`
	BothWrong   int    `json:"both_wrong"`
}

type sharedIP struct {
	IP     string   `json:"ip"`
	People []person `json:"people"`
}

type integrityReport struct {
	Attempts   []*attemptRisk  `json:"attempts"`
	Similarity []similarPair   `json:"similarity"`
	Collusion  []collusionPair `json:"collusion"`
	SharedIPs  []sharedIP      `json:"shared_ips"`
}

// Similarity at or above this is reported; at or above simHigh it is a
// high-severity signal.
const (
	simReport = 60
	simHigh   = 85
)

// A "large insert": at least burstMinChars characters arriving faster than
// burstCharsPerSec since the previous snapshot.
const (
	burstMinChars    = 100
	burstCharsPerSec = 12.0
)

// buildReport assembles the whole-test integrity report.
func (h *IntegrityHandler) buildReport(ctx context.Context, assessmentID string) (*integrityReport, error) {
	rep := &integrityReport{Attempts: []*attemptRisk{}, Similarity: []similarPair{}, Collusion: []collusionPair{}, SharedIPs: []sharedIP{}}

	// Attempts.
	rows, err := h.Pool.Query(ctx, `
		SELECT a.id::text, COALESCE(u.name, ''), COALESCE(u.email, ''), a.status, a.integrity_score::float8
		FROM   attempts a LEFT JOIN users u ON u.id = a.user_id
		WHERE  a.assessment_id::text = $1
		ORDER  BY a.started_at
	`, assessmentID)
	if err != nil {
		return nil, fmt.Errorf("load attempts: %w", err)
	}
	byID := map[string]*attemptRisk{}
	var ids []string
	for rows.Next() {
		ar := &attemptRisk{Signals: []signal{}, IPs: []string{}}
		if err := rows.Scan(&ar.AttemptID, &ar.Name, &ar.Email, &ar.Status, &ar.IntegrityScore); err != nil {
			rows.Close()
			return nil, err
		}
		rep.Attempts = append(rep.Attempts, ar)
		byID[ar.AttemptID] = ar
		ids = append(ids, ar.AttemptID)
	}
	rows.Close()
	if len(ids) == 0 {
		return rep, nil
	}
	add := func(id, sev, text string, pts int) {
		if ar := byID[id]; ar != nil {
			ar.Signals = append(ar.Signals, signal{Severity: sev, Text: text, Points: pts})
		}
	}

	// Browser-proctor tallies and the integrity score.
	evRows, err := h.Pool.Query(ctx, `
		SELECT attempt_id::text, kind, COUNT(*) FROM proctor_events
		WHERE  attempt_id::text = ANY($1) GROUP BY attempt_id, kind
	`, ids)
	if err == nil {
		labels := map[string]string{
			"tab_blur": "left the test tab", "fullscreen_exit": "left full screen", "paste": "tried to paste",
			"copy": "tried to copy", "devtools": "opened developer tools", "multi_monitor": "used a second monitor",
			"window_resized": "shrank the test window", "screenshot": "pressed Print Screen",
			"duplicate_session": "opened the test on a second tab or device", "ip_change": "changed network mid-test",
			"camera_off": "turned the camera off", "camera_denied": "refused the camera", "multi_face": "had more than one face on camera",
			"no_face": "left the camera", "voice_detected": "was talking", "disconnect": "went offline",
		}
		for evRows.Next() {
			var id, kind string
			var n int
			if evRows.Scan(&id, &kind, &n) != nil {
				continue
			}
			label, known := labels[kind]
			if !known {
				continue
			}
			switch kind {
			case "duplicate_session", "devtools", "multi_monitor":
				add(id, "medium", fmt.Sprintf("%s (%d×)", capitalize(label), n), 20)
			case "tab_blur", "fullscreen_exit", "paste", "screenshot", "camera_off", "multi_face":
				if n >= 3 {
					add(id, "medium", fmt.Sprintf("%s %d times", capitalize(label), n), 10)
				} else {
					add(id, "low", fmt.Sprintf("%s %d time%s", capitalize(label), n, plural(n)), 3)
				}
			default:
				add(id, "low", fmt.Sprintf("%s (%d×)", capitalize(label), n), 2)
			}
		}
		evRows.Close()
	}
	for _, ar := range rep.Attempts {
		switch {
		case ar.Status == "disqualified":
			add(ar.AttemptID, "high", "Disqualified by the proctor (tab-switch limit reached)", 60)
		case ar.IntegrityScore < 60:
			add(ar.AttemptID, "medium", fmt.Sprintf("Low integrity score (%.0f/100)", ar.IntegrityScore), 20)
		}
	}

	// Sessions and IPs.
	sRows, err := h.Pool.Query(ctx, `
		SELECT attempt_id::text, ip, COUNT(*) OVER (PARTITION BY attempt_id)
		FROM   attempt_sessions WHERE attempt_id::text = ANY($1)
	`, ids)
	ipPeople := map[string]map[string]bool{}
	if err == nil {
		for sRows.Next() {
			var id, ip string
			var n int
			if sRows.Scan(&id, &ip, &n) != nil {
				continue
			}
			if ar := byID[id]; ar != nil {
				ar.Sessions = n
				if ip != "" && !contains(ar.IPs, ip) {
					ar.IPs = append(ar.IPs, ip)
				}
			}
			if ip != "" {
				if ipPeople[ip] == nil {
					ipPeople[ip] = map[string]bool{}
				}
				ipPeople[ip][id] = true
			}
		}
		sRows.Close()
	}
	for ip, set := range ipPeople {
		if len(set) < 2 {
			continue
		}
		si := sharedIP{IP: ip}
		for id := range set {
			if ar := byID[id]; ar != nil {
				si.People = append(si.People, ar.person)
			}
		}
		sort.Slice(si.People, func(i, j int) bool { return si.People[i].Name < si.People[j].Name })
		rep.SharedIPs = append(rep.SharedIPs, si)
		// A handful sharing one address is a household or a café worth a look;
		// dozens is a campus network and says nothing on its own.
		if len(set) <= 4 {
			for id := range set {
				add(id, "low", fmt.Sprintf("Same network as %d other candidate%s (%s)", len(set)-1, plural(len(set)-1), ip), 8)
			}
		}
	}
	sort.Slice(rep.SharedIPs, func(i, j int) bool { return len(rep.SharedIPs[i].People) > len(rep.SharedIPs[j].People) })

	if err := h.codeSignals(ctx, rep, byID, ids, add); err != nil {
		return nil, err
	}
	if err := h.mcqSignals(ctx, rep, byID, ids, add); err != nil {
		return nil, err
	}

	// Rating: the points from every signal, in three bands.
	for _, ar := range rep.Attempts {
		sort.SliceStable(ar.Signals, func(i, j int) bool { return sevRank(ar.Signals[i].Severity) > sevRank(ar.Signals[j].Severity) })
		for _, s := range ar.Signals {
			ar.RiskPoints += s.Points
		}
		switch {
		case ar.RiskPoints >= 60:
			ar.Risk = "high"
		case ar.RiskPoints >= 25:
			ar.Risk = "medium"
		default:
			ar.Risk = "low"
		}
	}
	sort.SliceStable(rep.Attempts, func(i, j int) bool { return rep.Attempts[i].RiskPoints > rep.Attempts[j].RiskPoints })
	return rep, nil
}

// codeSignals: similar code between candidates, the hidden-marker trap, large
// code insertions (often right after leaving the tab), and suspiciously fast
// solves.
func (h *IntegrityHandler) codeSignals(ctx context.Context, rep *integrityReport, byID map[string]*attemptRisk, ids []string,
	add func(id, sev, text string, pts int)) error {

	type answer struct {
		attempt, question, problem, title, lang, code string
		marks, awarded                                float64
		spentMs                                       int64
	}
	rows, err := h.Pool.Query(ctx, `
		SELECT aq.attempt_id::text, aq.id::text, aq.problem_id::text, COALESCE(p.title, 'Coding question'),
		       COALESCE(aq.language, ''), COALESCE(aq.code, ''), aq.marks::float8,
		       COALESCE(aq.awarded_marks, 0)::float8, COALESCE(aq.time_spent_ms, 0)
		FROM   attempt_questions aq
		LEFT   JOIN problems p ON p.id = aq.problem_id
		WHERE  aq.attempt_id::text = ANY($1) AND aq.kind = 'coding'
	`, ids)
	if err != nil {
		return fmt.Errorf("load coding answers: %w", err)
	}
	var answers []answer
	problems := map[string]bool{}
	for rows.Next() {
		var a answer
		if err := rows.Scan(&a.attempt, &a.question, &a.problem, &a.title, &a.lang, &a.code, &a.marks, &a.awarded, &a.spentMs); err != nil {
			rows.Close()
			return err
		}
		answers = append(answers, a)
		problems[a.problem] = true
	}
	rows.Close()

	// Starter code per problem and language, so the template is not "shared".
	starters := map[string]map[string]string{}
	if len(problems) > 0 {
		pids := make([]string, 0, len(problems))
		for p := range problems {
			pids = append(pids, p)
		}
		sRows, err := h.Pool.Query(ctx, `
			SELECT problem_id::text, COALESCE(javascript, ''), COALESCE(python, ''), COALESCE(java, ''), COALESCE(cpp, ''), COALESCE(go, '')
			FROM   starter_codes WHERE problem_id::text = ANY($1)
		`, pids)
		if err == nil {
			for sRows.Next() {
				var pid, js, py, java, cpp, gol string
				if sRows.Scan(&pid, &js, &py, &java, &cpp, &gol) == nil {
					starters[pid] = map[string]string{"javascript": js, "python": py, "java": java, "cpp": cpp, "go": gol}
				}
			}
			sRows.Close()
		}
	}

	// How each answer was written, from its snapshots: typed in steps, or
	// arriving faster than anyone types. Used below to tell the likely source
	// of a copied answer from the copy.
	type history struct{ steps, bursts int }
	written := map[string]*history{}
	if hRows, err := h.Pool.Query(ctx, `
		WITH s AS (
		  SELECT attempt_id, question_id, reason, chars_added,
		         EXTRACT(EPOCH FROM at - lag(at) OVER (PARTITION BY attempt_id, question_id ORDER BY at, id)) AS gap
		  FROM   code_snapshots WHERE attempt_id::text = ANY($1)
		)
		SELECT attempt_id::text, question_id::text,
		       COUNT(*) FILTER (WHERE reason IN ('edit', 'run', 'submit') AND chars_added > 0),
		       COUNT(*) FILTER (WHERE reason IN ('edit', 'run', 'submit') AND chars_added >= $2
		                          AND (gap IS NULL OR gap = 0 OR chars_added / gap >= $3))
		FROM   s GROUP BY attempt_id, question_id
	`, ids, burstMinChars, burstCharsPerSec); err == nil {
		for hRows.Next() {
			var aid, qid string
			var hs history
			if hRows.Scan(&aid, &qid, &hs.steps, &hs.bursts) == nil {
				written[aid+"|"+qid] = &hs
			}
		}
		hRows.Close()
	}
	// typedByHand: several small steps and no burst.
	typedByHand := func(a answer) bool {
		hs := written[a.attempt+"|"+a.question]
		return hs != nil && hs.bursts == 0 && hs.steps >= 3
	}
	pasted := func(a answer) bool {
		hs := written[a.attempt+"|"+a.question]
		return hs != nil && hs.bursts > 0
	}

	// Similarity, per problem and language.
	type printed struct {
		a  answer
		fp fingerprint
	}
	groups := map[string][]printed{}
	for _, a := range answers {
		if strings.TrimSpace(a.code) == "" {
			continue
		}
		base := simFingerprint(starters[a.problem][a.lang], a.lang)
		key := a.problem + "|" + a.lang
		groups[key] = append(groups[key], printed{a, simFingerprint(a.code, a.lang).without(base)})
	}
	for _, g := range groups {
		for i := 0; i < len(g); i++ {
			for j := i + 1; j < len(g); j++ {
				pct, ok := simScore(g[i].fp, g[j].fp)
				if !ok || pct < simReport {
					continue
				}
				pa, pb := byID[g[i].a.attempt], byID[g[j].a.attempt]
				if pa == nil || pb == nil {
					continue
				}
				rep.Similarity = append(rep.Similarity, similarPair{
					QuestionTitle: g[i].a.title, Language: g[i].a.lang, A: pa.person, B: pb.person, Percent: pct,
				})
				sev, pts := "medium", 30
				if pct >= simHigh {
					sev, pts = "high", 60
				}
				// A match flags both people, but playback can often say who
				// wrote it: the one who typed it in steps is the likely source,
				// the one whose copy landed at once the likely copier. The
				// source still shared their work, so they stay flagged — lower.
				flag := func(self, other printed) {
					switch {
					case typedByHand(self.a) && pasted(other.a):
						add(self.a.attempt, "medium", fmt.Sprintf("Code %d%% similar to %s on “%s” — this candidate typed it step by step, %s's arrived at once: likely the source",
							pct, nameOf(byID[other.a.attempt].person), self.a.title, nameOf(byID[other.a.attempt].person)), 20)
					default:
						add(self.a.attempt, sev, fmt.Sprintf("Code %d%% similar to %s on “%s”", pct, nameOf(byID[other.a.attempt].person), self.a.title), pts)
					}
				}
				flag(g[i], g[j])
				flag(g[j], g[i])
			}
		}
	}
	sort.Slice(rep.Similarity, func(i, j int) bool { return rep.Similarity[i].Percent > rep.Similarity[j].Percent })

	// The hidden marker, in the final code or anywhere in its history.
	trapHit := map[string]bool{}
	for _, a := range answers {
		if strings.Contains(a.code, trapToken(a.attempt, a.question)) {
			trapHit[a.attempt+"|"+a.question] = true
		}
	}
	tRows, err := h.Pool.Query(ctx, `
		SELECT DISTINCT ON (attempt_id, question_id) attempt_id::text, question_id::text, code
		FROM   code_snapshots
		WHERE  attempt_id::text = ANY($1) AND code LIKE '%kv\_%'
	`, ids)
	if err == nil {
		for tRows.Next() {
			var aid, qid, code string
			if tRows.Scan(&aid, &qid, &code) == nil && strings.Contains(code, trapToken(aid, qid)) {
				trapHit[aid+"|"+qid] = true
			}
		}
		tRows.Close()
	}
	titleOf := map[string]string{}
	for _, a := range answers {
		titleOf[a.attempt+"|"+a.question] = a.title
	}
	for k := range trapHit {
		aid := strings.SplitN(k, "|", 2)[0]
		add(aid, "high", fmt.Sprintf("Answer to “%s” contains the hidden marker from the question — it was likely copied into an AI tool", titleOf[k]), 100)
	}

	// Code that appeared faster than anyone types. Judged by rate, not size:
	// a short pasted answer is still a paste, and a long function typed in one
	// go is not. People sustain perhaps 8–10 characters a second; the snapshot
	// gap also includes the pause that triggered the snapshot, so honest
	// typing comes out well under the line.
	bRows, err := h.Pool.Query(ctx, `
		WITH s AS (
		  SELECT attempt_id, question_id, at, reason, chars_added,
		         EXTRACT(EPOCH FROM at - lag(at) OVER (PARTITION BY attempt_id, question_id ORDER BY at, id)) AS gap
		  FROM   code_snapshots WHERE attempt_id::text = ANY($1)
		)
		SELECT s.attempt_id::text, s.question_id::text, s.at, s.chars_added, COALESCE(s.gap, 0)::float8,
		       EXISTS (SELECT 1 FROM proctor_events e
		               WHERE e.attempt_id = s.attempt_id AND e.kind = 'tab_blur'
		                 AND e.occurred_at BETWEEN s.at - interval '90 seconds' AND s.at)
		FROM   s
		WHERE  s.reason IN ('edit', 'run', 'submit') AND s.chars_added >= $2
		ORDER  BY s.at
	`, ids, burstMinChars)
	if err == nil {
		count := map[string]int{}
		for bRows.Next() {
			var aid, qid string
			var at time.Time
			var added int
			var gap float64
			var afterTab bool
			if bRows.Scan(&aid, &qid, &at, &added, &gap, &afterTab) != nil {
				continue
			}
			if gap > 0 && float64(added)/gap < burstCharsPerSec {
				continue
			}
			count[aid]++
			if count[aid] > 3 {
				continue
			}
			if afterTab {
				add(aid, "medium", fmt.Sprintf("%d characters of code appeared at once on “%s”, straight after leaving the tab", added, titleOf[aid+"|"+qid]), 30)
			} else {
				add(aid, "low", fmt.Sprintf("%d characters of code appeared at once on “%s”", added, titleOf[aid+"|"+qid]), 12)
			}
		}
		bRows.Close()
	}

	// Full marks in a fraction of the time the others needed.
	spent := map[string][]int64{}
	for _, a := range answers {
		if a.marks > 0 && a.awarded >= a.marks && a.spentMs > 0 {
			spent[a.problem] = append(spent[a.problem], a.spentMs)
		}
	}
	for _, a := range answers {
		cohort := spent[a.problem]
		if a.marks == 0 || a.awarded < a.marks || a.spentMs == 0 || len(cohort) < 4 {
			continue
		}
		med := median(cohort)
		if med >= 120_000 && a.spentMs*4 < med {
			add(a.attempt, "medium", fmt.Sprintf("Solved “%s” in %s — the others needed about %s", a.title, dur(a.spentMs), dur(med)), 20)
		}
	}
	return nil
}

// mcqSignals: identical wrong answers between pairs of candidates, and
// near-perfect scores at implausible speed.
func (h *IntegrityHandler) mcqSignals(ctx context.Context, rep *integrityReport, byID map[string]*attemptRisk, ids []string,
	add func(id, sev, text string, pts int)) error {

	rows, err := h.Pool.Query(ctx, `
		SELECT attempt_id::text, mcq_question_id::text, COALESCE(array_to_string(selected_options, ','), ''),
		       COALESCE(awarded_marks, 0)::float8, marks::float8, COALESCE(time_spent_ms, 0)
		FROM   attempt_questions
		WHERE  attempt_id::text = ANY($1) AND kind = 'mcq' AND mcq_question_id IS NOT NULL
	`, ids)
	if err != nil {
		return fmt.Errorf("load mcq answers: %w", err)
	}
	type pick struct {
		sel     string
		correct bool
	}
	answers := map[string]map[string]pick{} // attempt -> question -> pick
	type pace struct {
		answered, correct int
		ms                int64
	}
	paces := map[string]*pace{}
	for rows.Next() {
		var aid, qid, sel string
		var awarded, marks float64
		var ms int64
		if rows.Scan(&aid, &qid, &sel, &awarded, &marks, &ms) != nil {
			continue
		}
		if sel == "" {
			continue
		}
		if answers[aid] == nil {
			answers[aid] = map[string]pick{}
		}
		ok := awarded >= marks && marks > 0
		answers[aid][qid] = pick{sel: sortedCSV(sel), correct: ok}
		p := paces[aid]
		if p == nil {
			p = &pace{}
			paces[aid] = p
		}
		p.answered++
		if ok {
			p.correct++
		}
		p.ms += ms
	}
	rows.Close()

	// Shared wrong answers. Two people independently choosing the same wrong
	// option now and then is normal; doing it again and again is not.
	list := make([]string, 0, len(answers))
	for aid := range answers {
		list = append(list, aid)
	}
	sort.Strings(list)
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			a, b := answers[list[i]], answers[list[j]]
			bothWrong, same := 0, 0
			for q, pa := range a {
				pb, ok := b[q]
				if !ok || pa.correct || pb.correct {
					continue
				}
				bothWrong++
				if pa.sel == pb.sel {
					same++
				}
			}
			if same >= 3 && same*10 >= bothWrong*7 {
				pa, pb := byID[list[i]], byID[list[j]]
				if pa == nil || pb == nil {
					continue
				}
				rep.Collusion = append(rep.Collusion, collusionPair{A: pa.person, B: pb.person, SharedWrong: same, BothWrong: bothWrong})
				add(pa.AttemptID, "high", fmt.Sprintf("Chose the same wrong answer as %s on %d of %d questions", nameOf(pb.person), same, bothWrong), 45)
				add(pb.AttemptID, "high", fmt.Sprintf("Chose the same wrong answer as %s on %d of %d questions", nameOf(pa.person), same, bothWrong), 45)
			}
		}
	}
	sort.Slice(rep.Collusion, func(i, j int) bool { return rep.Collusion[i].SharedWrong > rep.Collusion[j].SharedWrong })

	// Speed: average time per answered question against the group's median.
	var avgs []int64
	for _, p := range paces {
		if p.answered >= 5 && p.ms > 0 {
			avgs = append(avgs, p.ms/int64(p.answered))
		}
	}
	if len(avgs) >= 5 {
		med := median(avgs)
		for aid, p := range paces {
			if p.answered < 5 || p.ms == 0 {
				continue
			}
			avg := p.ms / int64(p.answered)
			if avg*4 < med && p.correct*10 >= p.answered*8 {
				add(aid, "medium", fmt.Sprintf("Answered %d/%d multiple-choice questions correctly at %s each — the group took about %s",
					p.correct, p.answered, dur(avg), dur(med)), 20)
			}
		}
	}
	return nil
}

// attemptDetail is one candidate's evidence: their signals from the report,
// their sessions, and the snapshots behind code playback.
func (h *IntegrityHandler) attemptDetail(w http.ResponseWriter, r *http.Request, role, userID, attemptID string) {
	ctx := r.Context()
	var assessmentID string
	if err := h.Pool.QueryRow(ctx, `SELECT assessment_id::text FROM attempts WHERE id::text = $1`, attemptID).Scan(&assessmentID); err != nil {
		h.fail(w, http.StatusNotFound, "attempt not found")
		return
	}
	if !h.mayReview(w, ctx, role, userID, assessmentID) {
		return
	}
	rep, err := h.buildReport(ctx, assessmentID)
	if err != nil {
		h.Log.Error("integrity report failed", zap.Error(err))
		h.fail(w, http.StatusInternalServerError, "could not build the integrity report")
		return
	}
	var mine *attemptRisk
	for _, a := range rep.Attempts {
		if a.AttemptID == attemptID {
			mine = a
		}
	}

	type sessionOut struct {
		IP        string `json:"ip"`
		UserAgent string `json:"user_agent"`
		Screen    string `json:"screen"`
		FirstSeen string `json:"first_seen"`
		LastSeen  string `json:"last_seen"`
	}
	sessions := []sessionOut{}
	if rows, err := h.Pool.Query(ctx, `
		SELECT ip, user_agent, screen, first_seen, last_seen FROM attempt_sessions
		WHERE attempt_id::text = $1 ORDER BY first_seen
	`, attemptID); err == nil {
		for rows.Next() {
			var s sessionOut
			var f, l time.Time
			if rows.Scan(&s.IP, &s.UserAgent, &s.Screen, &f, &l) == nil {
				s.FirstSeen, s.LastSeen = f.UTC().Format(time.RFC3339), l.UTC().Format(time.RFC3339)
				sessions = append(sessions, s)
			}
		}
		rows.Close()
	}

	type snapOut struct {
		At         string `json:"at"`
		Language   string `json:"language"`
		Code       string `json:"code"`
		Reason     string `json:"reason"`
		CharsAdded int    `json:"chars_added"`
	}
	snaps := map[string][]snapOut{}
	if rows, err := h.Pool.Query(ctx, `
		SELECT question_id::text, at, language, code, reason, chars_added
		FROM   code_snapshots WHERE attempt_id::text = $1 ORDER BY question_id, at, id
	`, attemptID); err == nil {
		for rows.Next() {
			var qid string
			var s snapOut
			var at time.Time
			if rows.Scan(&qid, &at, &s.Language, &s.Code, &s.Reason, &s.CharsAdded) == nil {
				s.At = at.UTC().Format(time.RFC3339Nano)
				snaps[qid] = append(snaps[qid], s)
			}
		}
		rows.Close()
	}

	h.json(w, http.StatusOK, map[string]any{
		"risk":      mine,
		"sessions":  sessions,
		"snapshots": snaps,
		// Pairs involving this candidate, for side-by-side review.
		"similarity": filterPairs(rep.Similarity, attemptID),
	})
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func filterPairs(all []similarPair, attemptID string) []similarPair {
	out := []similarPair{}
	for _, p := range all {
		if p.A.AttemptID == attemptID || p.B.AttemptID == attemptID {
			out = append(out, p)
		}
	}
	return out
}

func sevRank(s string) int {
	switch s {
	case "high":
		return 3
	case "medium":
		return 2
	}
	return 1
}

func median(xs []int64) int64 {
	c := append([]int64(nil), xs...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	return c[len(c)/2]
}

func dur(ms int64) string {
	s := ms / 1000
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	return fmt.Sprintf("%dm %02ds", s/60, s%60)
}

func sortedCSV(s string) string {
	parts := strings.Split(s, ",")
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func nameOf(p person) string {
	if p.Name != "" {
		return p.Name
	}
	return p.Email
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (h *IntegrityHandler) json(w http.ResponseWriter, code int, v any) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *IntegrityHandler) fail(w http.ResponseWriter, code int, msg string) {
	h.json(w, code, map[string]string{"error": msg})
}

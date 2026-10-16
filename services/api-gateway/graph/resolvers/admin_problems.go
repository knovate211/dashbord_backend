package resolvers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	executionv1 "github.com/knovate211/proto/execution/v1"
)

// Coding-problem authoring for the admin panel.
//
// A function-mode problem is fully described by its signature and its test
// cases: the judge wraps the learner's function in a driver generated from the
// signature, feeds each test case (one JSON value per parameter, one per line)
// and compares the returned value with the expected JSON. Starter code for all
// five languages is generated from the same signature by the execution
// service, so an author never writes per-language boilerplate.
//
// Only function-mode problems are editable here. The seeded stdio and SQL
// problems keep their own formats and are shown read-only.

var problemTypes = map[string]bool{
	"int": true, "long": true, "double": true, "bool": true, "string": true, "char": true,
	"int[]": true, "long[]": true, "double[]": true, "bool[]": true, "string[]": true,
	"int[][]": true, "string[][]": true, "TreeNode": true, "ListNode": true,
}

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type problemParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type problemSignature struct {
	EntryPoint string         `json:"entry_point"`
	Params     []problemParam `json:"params"`
	ReturnType string         `json:"return_type"`
	Compare    string         `json:"compare"`
}

type problemCase struct {
	Input          string `json:"input"`
	ExpectedOutput string `json:"expected_output"`
	IsHidden       bool   `json:"is_hidden"`
}

type problemInput struct {
	Title       string           `json:"title"`
	Difficulty  string           `json:"difficulty"`
	Topic       string           `json:"topic"`
	CourseID    string           `json:"course_id"`
	Statement   string           `json:"statement"`
	Constraints []string         `json:"constraints"`
	IsPrivate   bool             `json:"is_private"`
	Signature   problemSignature `json:"signature"`
	TestCases   []problemCase    `json:"test_cases"`
}

// validate returns a user-facing error, or "" when the problem can be saved.
func (p *problemInput) validate() string {
	p.Title = strings.TrimSpace(p.Title)
	p.Topic = strings.TrimSpace(p.Topic)
	p.Statement = strings.TrimSpace(p.Statement)
	switch {
	case p.Title == "" || len(p.Title) > 200:
		return "title is required (at most 200 characters)"
	case p.Difficulty != "Easy" && p.Difficulty != "Medium" && p.Difficulty != "Hard":
		return "difficulty must be Easy, Medium or Hard"
	case p.Statement == "":
		return "the problem statement is required"
	}
	if p.Topic == "" {
		p.Topic = "General"
	}

	s := &p.Signature
	s.EntryPoint = strings.TrimSpace(s.EntryPoint)
	if !identRe.MatchString(s.EntryPoint) {
		return "function name must be a valid identifier, e.g. maxProfit"
	}
	if len(s.Params) == 0 {
		return "add at least one parameter"
	}
	seen := map[string]bool{}
	for i := range s.Params {
		s.Params[i].Name = strings.TrimSpace(s.Params[i].Name)
		prm := s.Params[i]
		if !identRe.MatchString(prm.Name) {
			return fmt.Sprintf("parameter %d needs a valid name", i+1)
		}
		if seen[prm.Name] {
			return fmt.Sprintf("parameter name %q is used twice", prm.Name)
		}
		seen[prm.Name] = true
		if !problemTypes[prm.Type] {
			return fmt.Sprintf("parameter %q has an unsupported type", prm.Name)
		}
	}
	if !problemTypes[s.ReturnType] {
		return "choose a supported return type"
	}
	if s.Compare == "" {
		s.Compare = "exact"
	}
	if s.Compare != "exact" && s.Compare != "unordered" && s.Compare != "set" && s.Compare != "float" {
		return "compare must be exact, unordered, set or float"
	}

	if len(p.TestCases) == 0 {
		return "add at least one test case"
	}
	visible := 0
	for i, tc := range p.TestCases {
		lines := strings.Split(strings.TrimRight(tc.Input, "\n"), "\n")
		if len(lines) != len(s.Params) {
			return fmt.Sprintf("test case %d: expected %d input line(s), one per parameter", i+1, len(s.Params))
		}
		for j, line := range lines {
			if !json.Valid([]byte(line)) {
				return fmt.Sprintf("test case %d: %s is not valid JSON (strings need quotes, e.g. \"abc\")", i+1, s.Params[j].Name)
			}
		}
		if !json.Valid([]byte(strings.TrimSpace(tc.ExpectedOutput))) {
			return fmt.Sprintf("test case %d: expected output is not valid JSON", i+1)
		}
		if !tc.IsHidden {
			visible++
		}
	}
	if visible == 0 {
		return "at least one test case must be visible — it is shown to candidates as the example"
	}
	return ""
}

func (h *AdminHandler) generateStarters(ctx context.Context, s problemSignature) (map[string]string, string, error) {
	if h.Exec == nil {
		return nil, "", errors.New("execution service not configured")
	}
	req := &executionv1.GenerateStartersRequest{EntryPoint: s.EntryPoint, ReturnType: s.ReturnType}
	for _, p := range s.Params {
		req.Params = append(req.Params, &executionv1.StarterParam{Name: p.Name, Type: p.Type})
	}
	resp, err := h.Exec.GenerateStarters(ctx, req)
	if err != nil {
		return nil, "", err
	}
	return resp.Starters, resp.Error, nil
}

// POST /api/admin/problems/starters — preview starter code for a signature.
func (h *AdminHandler) handleProblemStarters(w http.ResponseWriter, r *http.Request) {
	var s problemSignature
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		h.jsonErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	starters, msg, err := h.generateStarters(r.Context(), s)
	if err != nil {
		h.Log.Error("generate starters failed", zap.Error(err))
		h.jsonErr(w, http.StatusBadGateway, "could not reach the code generator")
		return
	}
	if msg != "" {
		h.jsonErr(w, http.StatusBadRequest, msg)
		return
	}
	h.json(w, http.StatusOK, map[string]any{"starters": starters})
}

// GET /api/admin/problems?search&course&difficulty&page&page_size
func (h *AdminHandler) handleListProblems(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 1000 {
		pageSize = 50
	}
	conds := []string{"TRUE"}
	args := []any{}
	add := func(format string, v any) {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf(format, len(args)))
	}
	if s := strings.TrimSpace(q.Get("search")); s != "" {
		add("(p.title ILIKE '%%' || $%d || '%%' OR p.topic ILIKE '%%' || $%[1]d || '%%')", s)
	}
	switch c := q.Get("course"); c {
	case "":
	case "__general":
		conds = append(conds, "p.course_id = ''")
	default:
		add("p.course_id = $%d", c)
	}
	if d := q.Get("difficulty"); d != "" {
		add("p.difficulty = $%d", d)
	}
	// "Authored here" = editable function problems; the 500+ seeded practice
	// problems would otherwise bury the handful an admin is working on.
	if q.Get("editable") == "true" {
		conds = append(conds, "p.io_mode = 'function' AND s.problem_id IS NOT NULL AND s.kind = 'function'")
	}
	where := strings.Join(conds, " AND ")

	var total int
	if err := h.Pool.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM problems p LEFT JOIN problem_signatures s ON s.problem_id = p.id WHERE `+where,
		args...).Scan(&total); err != nil {
		h.Log.Error("count problems failed", zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, "failed to load problems")
		return
	}

	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := h.Pool.Query(r.Context(), fmt.Sprintf(`
		SELECT p.id::text, p.title, p.difficulty, p.topic, p.course_id, p.is_private, p.io_mode,
		       (p.io_mode = 'function' AND s.problem_id IS NOT NULL AND s.kind = 'function') AS editable,
		       (SELECT COUNT(*) FROM test_cases t WHERE t.problem_id = p.id)::int,
		       (SELECT COUNT(*) FROM section_questions sq WHERE sq.problem_id = p.id)::int,
		       EXISTS (SELECT 1 FROM reference_solutions rs WHERE rs.problem_id = p.id),
		       p.updated_at
		FROM   problems p
		LEFT   JOIN problem_signatures s ON s.problem_id = p.id
		WHERE  %s
		ORDER  BY p.updated_at DESC, p.title
		LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args)), args...)
	if err != nil {
		h.Log.Error("list problems failed", zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, "failed to load problems")
		return
	}
	defer rows.Close()

	type row struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Difficulty  string `json:"difficulty"`
		Topic       string `json:"topic"`
		CourseID    string `json:"course_id"`
		IsPrivate   bool   `json:"is_private"`
		IoMode      string `json:"io_mode"`
		Editable    bool   `json:"editable"`
		TestCases   int    `json:"test_cases"`
		UsedInTests int    `json:"used_in_tests"`
		Verified    bool   `json:"verified"`
		UpdatedAt   string `json:"updated_at"`
	}
	out := []row{}
	for rows.Next() {
		var x row
		var at time.Time
		if err := rows.Scan(&x.ID, &x.Title, &x.Difficulty, &x.Topic, &x.CourseID, &x.IsPrivate, &x.IoMode,
			&x.Editable, &x.TestCases, &x.UsedInTests, &x.Verified, &at); err != nil {
			h.Log.Error("scan problem failed", zap.Error(err))
			h.jsonErr(w, http.StatusInternalServerError, "failed to load problems")
			return
		}
		x.UpdatedAt = at.Format(time.RFC3339)
		out = append(out, x)
	}
	h.json(w, http.StatusOK, map[string]any{"problems": out, "total": total})
}

// GET /api/admin/problems/{id}
func (h *AdminHandler) handleGetProblem(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	var (
		p                         problemInput
		ioMode                    string
		entry, ret, compare, kind *string
		params                    []byte
		usedIn                    int
	)
	err := h.Pool.QueryRow(ctx, `
		SELECT p.title, p.difficulty, p.topic, p.course_id, p.statement, p.is_private, p.io_mode,
		       s.entry_point, s.params, s.return_type, s.compare, s.kind,
		       (SELECT COUNT(*) FROM section_questions sq WHERE sq.problem_id = p.id)::int
		FROM   problems p LEFT JOIN problem_signatures s ON s.problem_id = p.id
		WHERE  p.id = $1::uuid`, id).Scan(&p.Title, &p.Difficulty, &p.Topic, &p.CourseID, &p.Statement,
		&p.IsPrivate, &ioMode, &entry, &params, &ret, &compare, &kind, &usedIn)
	if errors.Is(err, pgx.ErrNoRows) {
		h.jsonErr(w, http.StatusNotFound, "problem not found")
		return
	}
	if err != nil {
		h.Log.Error("get problem failed", zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, "failed to load the problem")
		return
	}
	if entry != nil {
		p.Signature = problemSignature{EntryPoint: *entry, ReturnType: deref(ret), Compare: deref(compare)}
		_ = json.Unmarshal(params, &p.Signature.Params)
	}
	p.Constraints, _ = h.collectIDs(ctx, `SELECT constraint_text FROM problem_constraints WHERE problem_id = $1::uuid ORDER BY order_index`, id)

	cases, err := h.Pool.Query(ctx, `SELECT input, expected_output, is_hidden FROM test_cases WHERE problem_id = $1::uuid ORDER BY order_index`, id)
	if err == nil {
		for cases.Next() {
			var tc problemCase
			if cases.Scan(&tc.Input, &tc.ExpectedOutput, &tc.IsHidden) == nil {
				p.TestCases = append(p.TestCases, tc)
			}
		}
		cases.Close()
	}

	refs := map[string]string{}
	if rr, err := h.Pool.Query(ctx, `SELECT language, code FROM reference_solutions WHERE problem_id = $1::uuid`, id); err == nil {
		for rr.Next() {
			var lang, code string
			if rr.Scan(&lang, &code) == nil {
				refs[lang] = code
			}
		}
		rr.Close()
	}

	editable := ioMode == "function" && entry != nil && deref(kind) == "function"
	h.json(w, http.StatusOK, map[string]any{
		"problem": p, "id": id, "io_mode": ioMode, "editable": editable,
		"used_in_tests": usedIn, "reference_solutions": refs,
	})
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// POST /api/admin/problems and PUT /api/admin/problems/{id}
func (h *AdminHandler) handleSaveProblem(w http.ResponseWriter, r *http.Request, id string) {
	var p problemInput
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		h.jsonErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := p.validate(); msg != "" {
		h.jsonErr(w, http.StatusBadRequest, msg)
		return
	}
	ctx := r.Context()

	starters, msg, err := h.generateStarters(ctx, p.Signature)
	if err != nil {
		h.Log.Error("generate starters failed", zap.Error(err))
		h.jsonErr(w, http.StatusBadGateway, "could not reach the code generator — the problem was not saved")
		return
	}
	if msg != "" {
		h.jsonErr(w, http.StatusBadRequest, msg)
		return
	}

	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		h.jsonErr(w, http.StatusInternalServerError, "could not save the problem")
		return
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	creating := id == ""
	if creating {
		err = tx.QueryRow(ctx, `
			INSERT INTO problems (slug, title, difficulty, topic, xp, statement, is_private, io_mode, course_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'function', $8)
			RETURNING id::text`,
			uniqueSlug(p.Title), p.Title, p.Difficulty, p.Topic, xpFor(p.Difficulty), p.Statement, p.IsPrivate, p.CourseID).Scan(&id)
	} else {
		// Only editable (function, signature-backed) problems may be rewritten.
		var ok bool
		err = tx.QueryRow(ctx, `
			SELECT p.io_mode = 'function' AND COALESCE(s.kind, 'function') = 'function'
			FROM problems p LEFT JOIN problem_signatures s ON s.problem_id = p.id WHERE p.id = $1::uuid`, id).Scan(&ok)
		if errors.Is(err, pgx.ErrNoRows) {
			h.jsonErr(w, http.StatusNotFound, "problem not found")
			return
		}
		if err == nil && !ok {
			h.jsonErr(w, http.StatusBadRequest, "this problem is not a function-mode problem and cannot be edited here")
			return
		}
		if err == nil {
			_, err = tx.Exec(ctx, `
				UPDATE problems SET title = $2, difficulty = $3, topic = $4, xp = $5, statement = $6,
				       is_private = $7, course_id = $8, updated_at = now()
				WHERE id = $1::uuid`,
				id, p.Title, p.Difficulty, p.Topic, xpFor(p.Difficulty), p.Statement, p.IsPrivate, p.CourseID)
		}
	}
	if err == nil {
		err = h.writeProblemParts(ctx, tx, id, &p, starters)
	}
	if err != nil {
		h.Log.Error("save problem failed", zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, "could not save the problem")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		h.jsonErr(w, http.StatusInternalServerError, "could not save the problem")
		return
	}

	action := "problem.updated"
	if creating {
		action = "problem.created"
	}
	h.audit(ctx, action, "", "", map[string]any{"problem_id": id, "title": p.Title, "test_cases": len(p.TestCases)})
	h.json(w, http.StatusOK, map[string]any{"id": id})
}

// writeProblemParts replaces everything hanging off the problem row. Replacing
// wholesale keeps the example list, the test cases and the starters derived
// from the one submitted definition — they cannot drift apart.
func (h *AdminHandler) writeProblemParts(ctx context.Context, tx pgx.Tx, id string, p *problemInput, starters map[string]string) error {
	// Seeded problems carry hand-written explanations on their examples. The
	// examples are rebuilt from the visible test cases below, so keep those
	// explanations and reattach them by position.
	var explanations []string
	if rows, err := tx.Query(ctx, `SELECT explanation FROM examples WHERE problem_id = $1::uuid ORDER BY order_index`, id); err == nil {
		for rows.Next() {
			var e string
			if rows.Scan(&e) == nil {
				explanations = append(explanations, e)
			}
		}
		rows.Close()
	}
	for _, table := range []string{"problem_constraints", "examples", "test_cases"} {
		if _, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE problem_id = $1::uuid`, id); err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}
	for i, c := range p.Constraints {
		if c = strings.TrimSpace(c); c == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO problem_constraints (problem_id, constraint_text, order_index) VALUES ($1::uuid, $2, $3)`, id, c, i); err != nil {
			return fmt.Errorf("insert constraint: %w", err)
		}
	}
	ex := 0
	for i, tc := range p.TestCases {
		input := strings.TrimRight(tc.Input, "\n")
		expected := strings.TrimSpace(tc.ExpectedOutput)
		if _, err := tx.Exec(ctx, `
			INSERT INTO test_cases (problem_id, input, expected_output, is_hidden, order_index)
			VALUES ($1::uuid, $2, $3, $4, $5)`, id, input, expected, tc.IsHidden, i); err != nil {
			return fmt.Errorf("insert test case: %w", err)
		}
		// Visible cases double as the worked examples; the problem service
		// re-derives their text from these same cases when it serves them.
		if !tc.IsHidden {
			explanation := ""
			if ex < len(explanations) {
				explanation = explanations[ex]
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO examples (problem_id, input, output, explanation, order_index)
				VALUES ($1::uuid, $2, $3, $4, $5)`, id, input, expected, explanation, ex); err != nil {
				return fmt.Errorf("insert example: %w", err)
			}
			ex++
		}
	}

	params, _ := json.Marshal(p.Signature.Params)
	if _, err := tx.Exec(ctx, `
		INSERT INTO problem_signatures (problem_id, entry_point, params, return_type, compare, kind, methods)
		VALUES ($1::uuid, $2, $3::jsonb, $4, $5, 'function', '[]'::jsonb)
		ON CONFLICT (problem_id) DO UPDATE SET entry_point = EXCLUDED.entry_point, params = EXCLUDED.params,
		       return_type = EXCLUDED.return_type, compare = EXCLUDED.compare, updated_at = now()`,
		id, p.Signature.EntryPoint, string(params), p.Signature.ReturnType, p.Signature.Compare); err != nil {
		return fmt.Errorf("upsert signature: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO starter_codes (problem_id, javascript, python, java, cpp, go)
		VALUES ($1::uuid, $2, $3, $4, $5, $6)
		ON CONFLICT (problem_id) DO UPDATE SET javascript = EXCLUDED.javascript, python = EXCLUDED.python,
		       java = EXCLUDED.java, cpp = EXCLUDED.cpp, go = EXCLUDED.go`,
		id, starters["javascript"], starters["python"], starters["java"], starters["cpp"], starters["go"]); err != nil {
		return fmt.Errorf("upsert starters: %w", err)
	}
	return nil
}

// POST /api/admin/problems/{id}/verify {language, code}
//
// Runs a solution against every test case, hidden ones included. When it
// passes them all it is stored as the problem's reference solution, which is
// what marks the problem "verified" in the list.
func (h *AdminHandler) handleVerifyProblem(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Language string `json:"language"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		h.jsonErr(w, http.StatusBadRequest, "language and code are required")
		return
	}
	if h.Exec == nil {
		h.jsonErr(w, http.StatusServiceUnavailable, "execution service not configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	resp, err := h.Exec.VerifySolution(ctx, &executionv1.VerifySolutionRequest{ProblemId: id, Language: req.Language, Code: req.Code})
	if err != nil {
		h.Log.Error("verify solution failed", zap.Error(err))
		h.jsonErr(w, http.StatusBadGateway, "could not run the solution: "+err.Error())
		return
	}
	allPassed := len(resp.TestResults) > 0
	for _, tr := range resp.TestResults {
		if tr.Status != "Accepted" {
			allPassed = false
		}
	}
	if allPassed {
		if _, err := h.Pool.Exec(ctx, `
			INSERT INTO reference_solutions (problem_id, language, code) VALUES ($1::uuid, $2, $3)
			ON CONFLICT (problem_id, language) DO UPDATE SET code = EXCLUDED.code`, id, req.Language, req.Code); err != nil {
			h.Log.Warn("store reference solution failed", zap.Error(err))
		}
	}
	h.json(w, http.StatusOK, map[string]any{"all_passed": allPassed, "result": resp})
}

// DELETE /api/admin/problems/{id} — refused while a test uses the problem or
// an attempt has recorded answers against it, so no paper or result breaks.
func (h *AdminHandler) handleDeleteProblem(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	var used, answered int
	var title string
	if err := h.Pool.QueryRow(ctx, `
		SELECT p.title,
		       (SELECT COUNT(*) FROM section_questions WHERE problem_id = p.id)::int,
		       (SELECT COUNT(*) FROM attempt_questions WHERE problem_id = p.id)::int
		FROM problems p WHERE p.id = $1::uuid`, id).Scan(&title, &used, &answered); errors.Is(err, pgx.ErrNoRows) {
		h.jsonErr(w, http.StatusNotFound, "problem not found")
		return
	} else if err != nil {
		h.jsonErr(w, http.StatusInternalServerError, "could not delete the problem")
		return
	}
	if used > 0 || answered > 0 {
		h.jsonErr(w, http.StatusConflict, fmt.Sprintf(
			"%q is used in %d test section(s) and %d attempt(s) — remove it from those tests first", title, used, answered))
		return
	}
	if _, err := h.Pool.Exec(ctx, `DELETE FROM problems WHERE id = $1::uuid`, id); err != nil {
		h.Log.Error("delete problem failed", zap.Error(err))
		h.jsonErr(w, http.StatusInternalServerError, "could not delete the problem")
		return
	}
	h.audit(ctx, "problem.deleted", "", "", map[string]any{"problem_id": id, "title": title})
	h.json(w, http.StatusOK, map[string]bool{"success": true})
}

var slugStrip = regexp.MustCompile(`[^a-z0-9]+`)

// uniqueSlug derives a URL slug from the title with a short random suffix, so
// two problems with the same title never collide on the UNIQUE slug column.
func uniqueSlug(title string) string {
	base := strings.Trim(slugStrip.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(base) > 60 {
		base = strings.Trim(base[:60], "-")
	}
	if base == "" {
		base = "problem"
	}
	return fmt.Sprintf("%s-%s", base, strconv.FormatInt(time.Now().UnixNano()%1_000_000, 36))
}

func xpFor(difficulty string) int {
	switch difficulty {
	case "Hard":
		return 150
	case "Medium":
		return 50
	}
	return 20
}

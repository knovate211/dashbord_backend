// Package repository provides Postgres-backed data access for the problem service.
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	problemv1 "github.com/knovate211/proto/problem/v1"
)

// ProblemRepository wraps a pgxpool and implements all problem data queries.
type ProblemRepository struct {
	pool *pgxpool.Pool
}

// New constructs a ProblemRepository.
func New(pool *pgxpool.Pool) *ProblemRepository {
	return &ProblemRepository{pool: pool}
}

// ListProblems returns a filtered, paginated list of problems (summary fields only).
func (r *ProblemRepository) ListProblems(ctx context.Context, req *problemv1.ListProblemsRequest) (*problemv1.ListProblemsResponse, error) {
	page := req.Page
	if page < 1 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	// Build dynamic WHERE clause
	var filterClauses []string
	var filterArgs []interface{}

	if req.SetId != "" {
		filterArgs = append(filterArgs, req.SetId)
		filterClauses = append(filterClauses, fmt.Sprintf("p.set_id::text = $%d", len(filterArgs)))
	}
	if req.Topic != "" {
		filterArgs = append(filterArgs, req.Topic)
		filterClauses = append(filterClauses, fmt.Sprintf("p.topic = $%d", len(filterArgs)))
	}
	if req.Difficulty != "" {
		filterArgs = append(filterArgs, req.Difficulty)
		filterClauses = append(filterClauses, fmt.Sprintf("p.difficulty = $%d", len(filterArgs)))
	}
	// Company-owned questions authored for a hiring drive must never appear in
	// the public practice listing. They stay reachable by id for a live attempt.
	filterClauses = append(filterClauses, "p.is_private = false")

	whereSQL := ""
	if len(filterClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(filterClauses, " AND ")
	}

	// Build pagination args
	mainArgs := make([]interface{}, len(filterArgs))
	copy(mainArgs, filterArgs)

	var userIdParamIndex int
	if req.UserId != "" {
		mainArgs = append(mainArgs, req.UserId)
		userIdParamIndex = len(mainArgs)
	}

	limitOffset := fmt.Sprintf("LIMIT $%d OFFSET $%d", len(mainArgs)+1, len(mainArgs)+2)
	paginationArgs := append(mainArgs, pageSize, offset)

	var query string
	if req.UserId != "" {
		query = fmt.Sprintf(`
			SELECT p.id, p.slug, p.title, p.difficulty, p.topic, p.xp,
			       COALESCE(p.set_id::text, '') AS set_id,
			       COALESCE(pus.status, 'Unsolved') AS user_status
			FROM   problems p
			LEFT JOIN problem_user_status pus ON pus.problem_id = p.id AND pus.user_id = $%d
			%s
			ORDER  BY p.created_at DESC
			%s
		`, userIdParamIndex, whereSQL, limitOffset)
	} else {
		query = fmt.Sprintf(`
			SELECT p.id, p.slug, p.title, p.difficulty, p.topic, p.xp,
			       COALESCE(p.set_id::text, '') AS set_id,
			       'Unsolved' AS user_status
			FROM   problems p
			%s
			ORDER  BY p.created_at DESC
			%s
		`, whereSQL, limitOffset)
	}

	rows, err := r.pool.Query(ctx, query, paginationArgs...)
	if err != nil {
		return nil, fmt.Errorf("list problems query: %w", err)
	}
	defer rows.Close()

	var problems []*problemv1.Problem
	for rows.Next() {
		p := &problemv1.Problem{}
		if err := rows.Scan(&p.Id, &p.Slug, &p.Title, &p.Difficulty, &p.Topic, &p.Xp, &p.SetId, &p.UserStatus); err != nil {
			return nil, fmt.Errorf("scan problem row: %w", err)
		}
		problems = append(problems, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate problem rows: %w", err)
	}

	// Count total matching rows
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM problems p %s", whereSQL)
	var total int32
	_ = r.pool.QueryRow(ctx, countQuery, filterArgs...).Scan(&total)

	return &problemv1.ListProblemsResponse{
		Problems: problems,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// GetProblem returns the full detail of a single problem by UUID or slug.
func (r *ProblemRepository) GetProblem(ctx context.Context, req *problemv1.GetProblemRequest) (*problemv1.Problem, error) {
	p := &problemv1.Problem{}
	var ioMode string

	err := r.pool.QueryRow(ctx, `
		SELECT id, slug, title, difficulty, topic, xp, statement,
		       COALESCE(set_id::text, '') AS set_id, io_mode
		FROM   problems
		WHERE  id::text = $1 OR slug = $1
	`, req.Id).Scan(&p.Id, &p.Slug, &p.Title, &p.Difficulty, &p.Topic, &p.Xp, &p.Statement, &p.SetId, &ioMode)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("problem not found: %s", req.Id)
		}
		return nil, fmt.Errorf("get problem: %w", err)
	}

	// Constraints
	cRows, _ := r.pool.Query(ctx,
		"SELECT constraint_text FROM problem_constraints WHERE problem_id = $1 ORDER BY order_index", p.Id)
	if cRows != nil {
		for cRows.Next() {
			var c string
			cRows.Scan(&c) //nolint:errcheck
			p.Constraints = append(p.Constraints, c)
		}
		cRows.Close()
	}

	// Tags
	tRows, _ := r.pool.Query(ctx, "SELECT tag FROM problem_tags WHERE problem_id = $1", p.Id)
	if tRows != nil {
		for tRows.Next() {
			var t string
			tRows.Scan(&t) //nolint:errcheck
			p.Tags = append(p.Tags, t)
		}
		tRows.Close()
	}

	// Examples
	eRows, _ := r.pool.Query(ctx,
		"SELECT input, output, explanation FROM examples WHERE problem_id = $1 ORDER BY order_index", p.Id)
	if eRows != nil {
		for eRows.Next() {
			ex := &problemv1.Example{}
			eRows.Scan(&ex.Input, &ex.Output, &ex.Explanation) //nolint:errcheck
			p.Examples = append(p.Examples, ex)
		}
		eRows.Close()
	}
	if err := r.alignExamplesToTestCases(ctx, p, ioMode); err != nil {
		return nil, err
	}

	// Hints
	hRows, _ := r.pool.Query(ctx,
		"SELECT order_index, title, body FROM hints WHERE problem_id = $1 ORDER BY order_index", p.Id)
	if hRows != nil {
		for hRows.Next() {
			h := &problemv1.Hint{}
			hRows.Scan(&h.Order, &h.Title, &h.Body) //nolint:errcheck
			p.Hints = append(p.Hints, h)
		}
		hRows.Close()
	}

	// Starter codes
	sc := &problemv1.StarterCodes{}
	r.pool.QueryRow(ctx, //nolint:errcheck
		"SELECT javascript, python, java, cpp, go FROM starter_codes WHERE problem_id = $1", p.Id).
		Scan(&sc.Javascript, &sc.Python, &sc.Java, &sc.Cpp, &sc.Go)
	p.StarterCodes = sc
	p.SupportedLanguages = supportedLanguages(ioMode, sc)

	return p, nil
}

// alignExamplesToTestCases replaces each displayed example's input and output
// with the visible test case it corresponds to, keeping the authored
// explanation.
//
// The two were stored independently and had drifted apart everywhere. Two Sum
// displayed "nums = [2,7,11,15], target = 9" while the judge fed it
// "[2,7,11,15]\n9" — so the format shown to a learner was one the runner would
// reject, and the worked example could not be pasted into the custom input box.
// Deriving the display from the graded data means they cannot disagree again.
//
// Only function-mode problems are realigned. stdio problems print free-form
// text and SQL problems carry a fixture document as their input; for both, the
// authored prose is the more useful thing to show.
func (r *ProblemRepository) alignExamplesToTestCases(ctx context.Context, p *problemv1.Problem, ioMode string) error {
	if ioMode != "function" || len(p.Examples) == 0 {
		return nil
	}

	rows, err := r.pool.Query(ctx, `
		SELECT input, expected_output
		FROM   test_cases
		WHERE  problem_id = $1 AND is_hidden = false
		ORDER  BY order_index`, p.Id)
	if err != nil {
		return fmt.Errorf("align examples: %w", err)
	}
	defer rows.Close()

	type visible struct{ input, expected string }
	var cases []visible
	for rows.Next() {
		var v visible
		if err := rows.Scan(&v.input, &v.expected); err != nil {
			return fmt.Errorf("align examples: %w", err)
		}
		cases = append(cases, v)
	}
	if len(cases) == 0 {
		return nil
	}

	// Show one example per visible test case. An authored explanation is kept
	// wherever there is one to keep; extra examples beyond the visible cases
	// described inputs that are never run, so they are dropped rather than left
	// to contradict the ones above them.
	aligned := make([]*problemv1.Example, 0, len(cases))
	for i, c := range cases {
		ex := &problemv1.Example{Input: c.input, Output: c.expected}
		if i < len(p.Examples) {
			ex.Explanation = p.Examples[i].Explanation
		}
		aligned = append(aligned, ex)
	}
	p.Examples = aligned
	return nil
}

// placeholderStarter reports whether a starter template is filler rather than
// a real skeleton for that language.
//
// The stdio problems are Go exercises; their other four columns were never
// authored. What is there instead is a small set of stock bodies repeated
// verbatim across hundreds of unrelated problems — 346 of them carry a starter
// that just prints "Completed", another 71 print "Hello World" or hold nothing
// but a "Write your code here" comment. A body that is byte-identical across
// hundreds of different problems cannot be a real skeleton for any of them.
//
// Offering those languages let a learner open a Go exercise in JavaScript and
// submit a program that could never pass.
func placeholderStarter(code string) bool {
	t := strings.TrimSpace(code)
	if t == "" {
		return true
	}

	// Stock filler bodies, identified by what they print.
	for _, marker := range []string{`"Completed"`, `'Completed'`, `"Hello World"`, `'Hello World'`} {
		if strings.Contains(t, marker) {
			return true
		}
	}
	if t == "# Not applicable" || t == "// Not applicable" {
		return true
	}

	// Nothing but comments, or a body whose only content is the "write your
	// code here" prompt with no statement of any kind around it.
	var stmts []string
	for _, line := range strings.Split(t, "\n") {
		l := strings.TrimSpace(line)
		if l == "" || strings.HasPrefix(l, "//") || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "*") || strings.HasPrefix(l, "/*") {
			continue
		}
		stmts = append(stmts, l)
	}
	return len(stmts) == 0
}

// supportedLanguages lists the languages a problem can genuinely be solved in.
//
// stdio problems are Go exercises, and only Go. Every one of the 412 of them
// ships a complete Go program; the other four columns hold stock filler
// repeated verbatim across hundreds of unrelated problems — 346 carry a body
// that just prints "Completed", the rest print "Hello World" or wrap a lone
// "write your code here" comment in an empty main. Pattern-matching four
// languages' filler shapes was losing to the Java and C++ variants, which have
// real structure around the empty body, so the rule is stated directly instead.
//
// If real multi-language stdio starters are ever authored, this is where that
// changes — and validate-problems will hold them to the same standard.
func supportedLanguages(ioMode string, sc *problemv1.StarterCodes) []string {
	switch ioMode {
	case "sql":
		return []string{"sql"}
	case "stdio":
		return []string{"go"}
	}

	// Function-mode starters are generated from the problem's signature, so
	// every language has a real skeleton unless the problem predates the
	// migration and still carries a hand-authored placeholder.
	candidates := []struct {
		name string
		code string
	}{
		{"javascript", sc.Javascript},
		{"python", sc.Python},
		{"java", sc.Java},
		{"cpp", sc.Cpp},
		{"go", sc.Go},
	}
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if !placeholderStarter(c.code) {
			out = append(out, c.name)
		}
	}
	// Never hand back an empty list — that would leave the editor with no
	// language at all, which is worse than offering one that needs work.
	if len(out) == 0 {
		return []string{"javascript"}
	}
	return out
}

// GetTestCases returns the test cases for a problem.
// When includeHidden is false (Run button), only visible test cases are returned.
func (r *ProblemRepository) GetTestCases(ctx context.Context, req *problemv1.GetTestCasesRequest) (*problemv1.GetTestCasesResponse, error) {
	query := `
		SELECT id, input, expected_output, is_hidden, time_limit_ms, memory_limit_mb, order_index
		FROM   test_cases
		WHERE  problem_id = $1
	`
	args := []interface{}{req.ProblemId}

	if !req.IncludeHidden {
		query += " AND is_hidden = false"
	}
	query += " ORDER BY order_index"

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get test cases: %w", err)
	}
	defer rows.Close()

	var testCases []*problemv1.TestCase
	for rows.Next() {
		tc := &problemv1.TestCase{}
		if err := rows.Scan(&tc.Id, &tc.Input, &tc.ExpectedOutput, &tc.IsHidden,
			&tc.TimeLimitMs, &tc.MemoryLimitMb, &tc.OrderIndex); err != nil {
			return nil, fmt.Errorf("scan test case: %w", err)
		}
		testCases = append(testCases, tc)
	}

	spec, err := r.getExecutionSpec(ctx, req.ProblemId)
	if err != nil {
		return nil, err
	}

	return &problemv1.GetTestCasesResponse{TestCases: testCases, ExecutionSpec: spec}, nil
}

// getExecutionSpec loads how this problem is executed and graded.
//
// io_mode always exists (it defaults to 'function'); the signature row may not,
// because problems are being migrated onto declared signatures in batches. A
// missing signature is not an error — it returns a spec carrying only the mode,
// and the judge keeps its legacy behaviour for that problem.
func (r *ProblemRepository) getExecutionSpec(ctx context.Context, problemID string) (*problemv1.ExecutionSpec, error) {
	const q = `
		SELECT p.io_mode,
		       s.entry_point, s.params, s.return_type, s.compare, s.float_eps,
		       s.kind, s.methods
		FROM   problems p
		LEFT   JOIN problem_signatures s ON s.problem_id = p.id
		WHERE  p.id = $1
	`
	var (
		ioMode     string
		entryPoint *string
		paramsRaw  []byte
		returnType *string
		compare    *string
		floatEps   *float64
		kind       *string
		methodsRaw []byte
	)
	err := r.pool.QueryRow(ctx, q, problemID).
		Scan(&ioMode, &entryPoint, &paramsRaw, &returnType, &compare, &floatEps, &kind, &methodsRaw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get execution spec: %w", err)
	}

	spec := &problemv1.ExecutionSpec{IoMode: ioMode}
	if entryPoint == nil {
		return spec, nil
	}

	spec.EntryPoint = *entryPoint
	if returnType != nil {
		spec.ReturnType = *returnType
	}
	if compare != nil {
		spec.Compare = *compare
	}
	if floatEps != nil {
		spec.FloatEps = *floatEps
	}
	if kind != nil {
		spec.Kind = *kind
	}
	if len(paramsRaw) > 0 {
		if err := json.Unmarshal(paramsRaw, &spec.Params); err != nil {
			return nil, fmt.Errorf("decode signature params for %s: %w", problemID, err)
		}
	}
	if len(methodsRaw) > 0 {
		if err := json.Unmarshal(methodsRaw, &spec.Methods); err != nil {
			return nil, fmt.Errorf("decode signature methods for %s: %w", problemID, err)
		}
	}
	return spec, nil
}

// ListPracticeSets returns all practice sets with optional per-user progress.
func (r *ProblemRepository) ListPracticeSets(ctx context.Context, req *problemv1.ListPracticeSetsRequest) (*problemv1.ListPracticeSetsResponse, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ps.id, ps.title, ps.level, ps.level_color, ps.bg_color,
		       COUNT(p.id) AS total_problems
		FROM   practice_sets ps
		LEFT   JOIN problems p ON p.set_id = ps.id
		GROUP  BY ps.id
		ORDER  BY ps.created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("list practice sets: %w", err)
	}
	defer rows.Close()

	var sets []*problemv1.PracticeSet
	for rows.Next() {
		s := &problemv1.PracticeSet{}
		if err := rows.Scan(&s.Id, &s.Title, &s.Level, &s.LevelColor, &s.BgColor, &s.TotalProblems); err != nil {
			return nil, fmt.Errorf("scan practice set: %w", err)
		}

		// Per-user progress if userId provided
		if req.UserId != "" {
			var solved int32
			r.pool.QueryRow(ctx, `
				SELECT COUNT(*) FROM problem_user_status pus
				JOIN   problems p ON p.id = pus.problem_id
				WHERE  pus.user_id = $1 AND p.set_id = $2 AND pus.status = 'Solved'
			`, req.UserId, s.Id).Scan(&solved) //nolint:errcheck

			if s.TotalProblems > 0 {
				s.Progress = float32(solved) / float32(s.TotalProblems) * 100
			}
		}
		sets = append(sets, s)
	}

	return &problemv1.ListPracticeSetsResponse{PracticeSets: sets}, nil
}

// GetProblemUserStatus returns the submission status for a specific user+problem.
func (r *ProblemRepository) GetProblemUserStatus(ctx context.Context, req *problemv1.GetProblemUserStatusRequest) (*problemv1.ProblemUserStatus, error) {
	pus := &problemv1.ProblemUserStatus{
		UserId:    req.UserId,
		ProblemId: req.ProblemId,
		Status:    "Unsolved",
	}

	err := r.pool.QueryRow(ctx, `
		SELECT status, COALESCE(solved_at::text, ''), attempts
		FROM   problem_user_status
		WHERE  user_id = $1 AND problem_id = $2
	`, req.UserId, req.ProblemId).Scan(&pus.Status, &pus.SolvedAt, &pus.Attempts)

	if err != nil && err != pgx.ErrNoRows {
		return nil, fmt.Errorf("get problem user status: %w", err)
	}

	return pus, nil
}

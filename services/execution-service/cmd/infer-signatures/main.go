// Command infer-signatures derives a problem's execution contract from the data
// that already exists for it, and writes it to problem_signatures.
//
// Two independent sources agree on the answer:
//
//	the JavaScript starter  gives the entry point and the parameter names
//	the test cases          give the parameter and return TYPES
//
// Types come from the data rather than from parsing a type annotation because
// the data is what the judge actually has to handle. If every test case feeds
// a JSON array of integers in the first position, the first parameter is int[]
// whatever a comment claims.
//
// Nothing is written unless every test case agrees on every type and the
// resulting signature validates. A problem the inference cannot settle is left
// without a signature — it keeps the legacy behaviour and gets reported, rather
// than being given a guess that would mis-grade it.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/knovate211/execution-service/internal/codegen"
)

var jsFuncRe = regexp.MustCompile(`function\s+([a-zA-Z0-9_]+)\s*\(([^)]*)\)`)

// paramNamePrefixRe matches the "name = " that LeetCode-style test inputs put in
// front of each value, e.g. `nums = [2,7,11,15], target = 9`.
var paramNamePrefixRe = regexp.MustCompile(`^\s*[a-zA-Z_][a-zA-Z0-9_]*\s*=\s*`)

// linkedStructureParams are parameter names that denote a tree or a linked list
// rather than the array their test data is written as.
//
// `root = [3,9,20,null,null,15,7]` is a level-order *serialisation* of a binary
// tree, not an array of integers. Nothing in these starters says so — they carry
// no type annotations at all, just `function isValidBST(root)` — so the name is
// the only signal available.
//
// Inferring int[] here would produce a driver that hands an array to a function
// expecting a tree: it would compile, run, and grade wrongly. Refusing costs
// nothing by comparison, since the problem simply stays on the legacy path until
// the type language grows TreeNode and ListNode.
var treeParams = map[string]bool{"root": true}

var listParams = map[string]bool{
	"head": true, "l1": true, "l2": true,
	"list1": true, "list2": true, "headA": true, "headB": true,
}

type problem struct {
	ID, Slug, Difficulty, JS string
	Cases                    []testCase
}

type testCase struct {
	ID       string `json:"id"`
	Input    string `json:"input"`
	Expected string `json:"expected"`
}

type skipped struct{ Slug, Reason string }

func main() {
	dsn := flag.String("dsn", envOr("POSTGRES_DSN", "postgres://skillofide:password@localhost:5432/skillofide?sslmode=disable"), "postgres DSN")
	apply := flag.Bool("apply", false, "write signatures; without this the run only reports")
	difficulties := flag.String("difficulties", "Medium,Hard", "comma-separated difficulties to process")
	normalize := flag.Bool("normalize", false, "rewrite test inputs of ALREADY-signed problems to match their stored signature, then exit")
	flag.Parse()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		fatal("connect: %v", err)
	}
	defer pool.Close()

	if *normalize {
		if err := normalizeSignedProblems(ctx, pool, *apply); err != nil {
			fatal("normalize: %v", err)
		}
		return
	}

	problems, err := load(ctx, pool, strings.Split(*difficulties, ","))
	if err != nil {
		fatal("load: %v", err)
	}

	var (
		derived []struct {
			p   problem
			sig codegen.Signature
		}
		skips []skipped
	)

	for _, p := range problems {
		sig, err := infer(p)
		if err != nil {
			skips = append(skips, skipped{p.Slug, err.Error()})
			continue
		}
		derived = append(derived, struct {
			p   problem
			sig codegen.Signature
		}{p, sig})
	}

	for _, d := range derived {
		params := make([]string, len(d.sig.Params))
		for i, prm := range d.sig.Params {
			params[i] = fmt.Sprintf("%s %s", prm.Type, prm.Name)
		}
		fmt.Printf("  %-38s %s(%s) -> %s\n", d.p.Slug, d.sig.EntryPoint, strings.Join(params, ", "), d.sig.ReturnType)
	}

	if len(skips) > 0 {
		fmt.Printf("\nnot inferable (%d) — left on the legacy path:\n", len(skips))
		sort.Slice(skips, func(i, j int) bool { return skips[i].Slug < skips[j].Slug })
		byReason := map[string]int{}
		for _, s := range skips {
			byReason[s.Reason]++
		}
		reasons := make([]string, 0, len(byReason))
		for r := range byReason {
			reasons = append(reasons, r)
		}
		sort.Slice(reasons, func(i, j int) bool { return byReason[reasons[i]] > byReason[reasons[j]] })
		for _, r := range reasons {
			fmt.Printf("  %4d  %s\n", byReason[r], r)
		}
	}

	fmt.Printf("\n%d inferable, %d not\n", len(derived), len(skips))
	if !*apply {
		fmt.Println("dry run — pass -apply to write")
		return
	}

	written, rewritten := 0, 0
	for _, d := range derived {
		raw, err := json.Marshal(d.sig.Params)
		if err != nil {
			fatal("%s: encode params: %v", d.p.Slug, err)
		}

		// Rewriting the test data is not optional. The generated driver reads
		// one JSON value per line, so a signature declaring two parameters
		// against an input stored as "[2,3,6,7], 7" on a single line would feed
		// the whole string to the first parameter and nothing to the second.
		// The signature and the data have to be changed together.
		for _, tc := range d.p.Cases {
			lines, err := normalizeInput(tc.Input, len(d.sig.Params))
			if err != nil {
				fatal("%s: normalise input: %v", d.p.Slug, err)
			}
			canonical := strings.Join(lines, "\n")
			if canonical == strings.TrimRight(tc.Input, "\n") {
				continue
			}
			if _, err := pool.Exec(ctx,
				`UPDATE test_cases SET input = $1 WHERE id = $2`, canonical, tc.ID); err != nil {
				fatal("%s: rewrite test case: %v", d.p.Slug, err)
			}
			rewritten++
		}
		// Rounded expectations need a tolerance wide enough to cover them. The
		// stored values carry about five decimals, so the default 1e-6 would
		// still reject a correct answer.
		eps := 1e-6
		if d.sig.Compare == codegen.CompareFloat {
			eps = 1e-4
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO problem_signatures (problem_id, entry_point, params, return_type, compare, float_eps)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (problem_id) DO UPDATE
			SET entry_point = EXCLUDED.entry_point,
			    params      = EXCLUDED.params,
			    return_type = EXCLUDED.return_type,
			    compare     = EXCLUDED.compare,
			    float_eps   = EXCLUDED.float_eps,
			    updated_at  = now()`,
			d.p.ID, d.sig.EntryPoint, raw, string(d.sig.ReturnType), string(d.sig.Compare), eps); err != nil {
			fatal("%s: write: %v", d.p.Slug, err)
		}
		written++
	}
	fmt.Printf("wrote %d signatures, rewrote %d test-case inputs to one JSON value per line\n", written, rewritten)
}

// infer derives the signature, or explains why it cannot.
func infer(p problem) (codegen.Signature, error) {
	m := jsFuncRe.FindStringSubmatch(p.JS)
	if m == nil {
		return codegen.Signature{}, fmt.Errorf("no function declaration in the JavaScript starter")
	}
	entry := m[1]
	if entry == "solveChallenge" {
		return codegen.Signature{}, fmt.Errorf("placeholder solveChallenge(input) starter — the problem itself needs authoring")
	}

	var names []string
	for _, raw := range strings.Split(m[2], ",") {
		n := strings.TrimSpace(raw)
		if n != "" {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return codegen.Signature{}, fmt.Errorf("entry point takes no parameters — cannot confirm against test data")
	}
	if len(p.Cases) == 0 {
		return codegen.Signature{}, fmt.Errorf("no test cases")
	}

	paramTypes := make([]codegen.Type, len(names))
	var retType codegen.Type

	// A parameter whose name marks it as a linked structure is typed as one
	// rather than as the array its test data is written as. The type language
	// models trees and lists now; what it cannot do is tell them apart from an
	// int[] by looking at "[3,9,20,null,null,15,7]", which is why the name is
	// still what decides.
	linked := make(map[int]codegen.Type, len(names))
	for i, name := range names {
		switch {
		case treeParams[name]:
			linked[i] = codegen.TTreeNode
		case listParams[name]:
			linked[i] = codegen.TListNode
		}
	}

	for _, tc := range p.Cases {
		lines, err := normalizeInput(tc.Input, len(names))
		if err != nil {
			return codegen.Signature{}, err
		}
		for i, line := range lines {
			if lt, ok := linked[i]; ok {
				// The serialised form is an array; the parameter is not.
				if _, err := inferType(line); err != nil {
					return codegen.Signature{}, fmt.Errorf("parameter %q: %v", names[i], err)
				}
				paramTypes[i] = lt
				continue
			}
			t, err := inferType(line)
			if err != nil {
				return codegen.Signature{}, fmt.Errorf("parameter %q: %v", names[i], err)
			}
			merged, err := merge(paramTypes[i], t)
			if err != nil {
				return codegen.Signature{}, fmt.Errorf("parameter %q: %v", names[i], err)
			}
			paramTypes[i] = merged
		}

		t, err := inferType(tc.Expected)
		if err != nil {
			return codegen.Signature{}, fmt.Errorf("expected output: %v", err)
		}
		merged, err := merge(retType, t)
		if err != nil {
			return codegen.Signature{}, fmt.Errorf("expected output: %v", err)
		}
		retType = merged
	}

	// A list problem whose expectation is a flat array is almost certainly
	// returning a ListNode, not the array: "[7,0,8]" is the serialised form of
	// the list the function built. Typing it int[] would make the driver
	// JSON.stringify a node object and emit {"val":7,"next":{...}}.
	//
	// Tree problems are left alone — a tree in, an array out is usually a
	// genuine traversal result, as in verticalTraversal returning int[][].
	for i, prm := range names {
		_ = prm
		if lt, ok := linked[i]; ok && lt == codegen.TListNode && strings.HasSuffix(string(retType), "[]") {
			return codegen.Signature{}, fmt.Errorf("takes a ListNode and returns %s — the return is probably a ListNode too; declare this signature by hand", retType)
		}
	}

	// A double result cannot survive exact comparison. The expected outputs are
	// stored rounded — "30.66667" — while a correct solution computes
	// 30.666666666666664, and string equality calls that wrong.
	compare := codegen.CompareExact
	if retType == codegen.TDouble || retType == codegen.TDblArr {
		compare = codegen.CompareFloat
	}

	sig := codegen.Signature{EntryPoint: entry, ReturnType: retType, Compare: compare}
	for i, n := range names {
		sig.Params = append(sig.Params, codegen.Param{Name: n, Type: paramTypes[i]})
	}
	if err := sig.Validate(); err != nil {
		return codegen.Signature{}, err
	}
	return sig, nil
}

// normalizeInput turns a stored test input into exactly one JSON value per
// parameter.
//
// Three formats are in the data, all meaning the same thing:
//
//	"[2,7,11,15]\n9"                 already canonical
//	"[2,3,6,7], 7"                    values on one line
//	"nums = [2,7,11,15], target = 9"  values on one line, each labelled
//
// Only the first is what the generated driver reads, so the other two are
// rewritten to match when the signature is applied. Splitting is bracket- and
// quote-aware: `[[1,3],[6,9]], [2,5]` is two values, not four, and the comma
// inside "ABC,DEF" is part of the string.
func normalizeInput(raw string, want int) ([]string, error) {
	trimmed := strings.TrimRight(raw, "\n")

	// An entry point that takes nothing is fed nothing. "None" was the
	// placeholder the seed used to mean exactly that.
	if want == 0 {
		if t := strings.TrimSpace(trimmed); t == "" || t == "None" {
			return []string{}, nil
		}
		return nil, fmt.Errorf("entry point takes no parameters but the test input is %q", truncate(trimmed, 40))
	}

	if strings.TrimSpace(trimmed) == "" || strings.TrimSpace(trimmed) == "None" {
		return nil, fmt.Errorf("test input is empty or the literal placeholder \"None\"")
	}

	if lines := strings.Split(trimmed, "\n"); len(lines) == want {
		return stripLabels(lines), nil
	}
	if want == 1 {
		return stripLabels([]string{trimmed}), nil
	}

	parts := splitTopLevel(trimmed)
	if len(parts) != want {
		return nil, fmt.Errorf("test input splits into %d value(s) but the entry point takes %d parameter(s)", len(parts), want)
	}
	return stripLabels(parts), nil
}

// stripLabels removes a leading "name = " from each value.
func stripLabels(parts []string) []string {
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSpace(paramNamePrefixRe.ReplaceAllString(p, ""))
	}
	return out
}

// splitTopLevel splits on commas that are outside brackets, braces and strings.
func splitTopLevel(s string) []string {
	var out []string
	var cur strings.Builder
	depth := 0
	inStr := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' && (i == 0 || s[i-1] != '\\') {
			inStr = !inStr
		}
		if !inStr {
			switch c {
			case '[', '{':
				depth++
			case ']', '}':
				depth--
			case ',':
				if depth == 0 {
					out = append(out, cur.String())
					cur.Reset()
					continue
				}
			}
		}
		cur.WriteByte(c)
	}
	if strings.TrimSpace(cur.String()) != "" {
		out = append(out, cur.String())
	}
	return out
}

// inferType reads one JSON value and reports the narrowest type that holds it.
func inferType(raw string) (codegen.Type, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("empty value")
	}
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return "", fmt.Errorf("not JSON: %q", truncate(s, 40))
	}
	return typeOf(v)
}

func typeOf(v interface{}) (codegen.Type, error) {
	switch t := v.(type) {
	case bool:
		return codegen.TBool, nil
	case string:
		return codegen.TString, nil
	case float64:
		if t == float64(int64(t)) {
			return codegen.TInt, nil
		}
		return codegen.TDouble, nil
	case []interface{}:
		if len(t) == 0 {
			// An empty array cannot say what it holds. Merging with a populated
			// case from another test resolves it.
			return codegen.TIntArr, nil
		}
		var elem codegen.Type
		for _, item := range t {
			et, err := typeOf(item)
			if err != nil {
				return "", err
			}
			merged, err := merge(elem, et)
			if err != nil {
				return "", err
			}
			elem = merged
		}
		switch elem {
		case codegen.TInt:
			return codegen.TIntArr, nil
		case codegen.TDouble:
			return codegen.TDblArr, nil
		case codegen.TBool:
			return codegen.TBoolArr, nil
		case codegen.TString:
			return codegen.TStrArr, nil
		case codegen.TIntArr:
			return codegen.TInt2D, nil
		case codegen.TStrArr:
			return codegen.TStr2D, nil
		}
		return "", fmt.Errorf("unsupported array element type %q", elem)
	case nil:
		return "", fmt.Errorf("null value gives no type")
	}
	return "", fmt.Errorf("unsupported JSON shape %T", v)
}

// merge combines two observations of the same position. int widens to double;
// anything else that disagrees is a genuine conflict and stops the inference.
func merge(a, b codegen.Type) (codegen.Type, error) {
	if a == "" {
		return b, nil
	}
	if b == "" || a == b {
		return a, nil
	}
	switch {
	case a == codegen.TInt && b == codegen.TDouble, a == codegen.TDouble && b == codegen.TInt:
		return codegen.TDouble, nil
	case a == codegen.TIntArr && b == codegen.TDblArr, a == codegen.TDblArr && b == codegen.TIntArr:
		return codegen.TDblArr, nil
	// An empty array defaults to int[]; a later case may reveal what it holds.
	case a == codegen.TIntArr && (b == codegen.TStrArr || b == codegen.TBoolArr || b == codegen.TInt2D):
		return b, nil
	case b == codegen.TIntArr && (a == codegen.TStrArr || a == codegen.TBoolArr || a == codegen.TInt2D):
		return a, nil
	}
	return "", fmt.Errorf("test cases disagree: %s in one, %s in another", a, b)
}

// normalizeSignedProblems brings every signed problem's test data into the one
// JSON value per line form its signature implies.
//
// Inference rewrites the data as it goes, but a signature declared by hand — as
// the four ListNode problems had to be, since no rule can tell "[7,0,8] is a
// list" from "[7,0,8] is an int[]" — leaves the data behind. The driver then
// reads the whole of "l1 = [2,4,3], l2 = [5,6,4]" as the first argument, throws
// on the JSON parse, and prints nothing. Running this closes that gap wherever
// it exists rather than only where it has been noticed.
func normalizeSignedProblems(ctx context.Context, pool *pgxpool.Pool, apply bool) error {
	rows, err := pool.Query(ctx, `
		SELECT p.id, p.slug, jsonb_array_length(s.params)
		FROM   problems p
		JOIN   problem_signatures s ON s.problem_id = p.id
		WHERE  p.io_mode = 'function'
		ORDER  BY p.slug`)
	if err != nil {
		return err
	}
	type target struct {
		id, slug string
		nparams  int
	}
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.slug, &t.nparams); err != nil {
			return err
		}
		targets = append(targets, t)
	}
	rows.Close()

	changed, broken := 0, 0
	for _, t := range targets {
		tcRows, err := pool.Query(ctx,
			`SELECT id, input FROM test_cases WHERE problem_id=$1 ORDER BY order_index`, t.id)
		if err != nil {
			return err
		}
		type tc struct{ id, input string }
		var cases []tc
		for tcRows.Next() {
			var c tc
			if err := tcRows.Scan(&c.id, &c.input); err != nil {
				return err
			}
			cases = append(cases, c)
		}
		tcRows.Close()

		for _, c := range cases {
			lines, err := normalizeInput(c.input, t.nparams)
			if err != nil {
				fmt.Printf("  UNFIXABLE  %-38s %v\n", t.slug, err)
				broken++
				continue
			}
			canonical := strings.Join(lines, "\n")
			if canonical == strings.TrimRight(c.input, "\n") {
				continue
			}
			fmt.Printf("  rewrite    %-38s %q -> %q\n", t.slug, truncate(c.input, 44), truncate(canonical, 44))
			changed++
			if apply {
				if _, err := pool.Exec(ctx, `UPDATE test_cases SET input=$1 WHERE id=$2`, canonical, c.id); err != nil {
					return err
				}
			}
		}
	}

	fmt.Printf("\n%d test input(s) need rewriting, %d cannot be normalised\n", changed, broken)
	if !apply {
		fmt.Println("dry run — pass -apply to write")
	}
	return nil
}

func load(ctx context.Context, pool *pgxpool.Pool, difficulties []string) ([]problem, error) {
	rows, err := pool.Query(ctx, `
		SELECT p.id, p.slug, p.difficulty, sc.javascript
		FROM   problems p
		JOIN   starter_codes sc ON sc.problem_id = p.id
		LEFT   JOIN problem_signatures s ON s.problem_id = p.id
		WHERE  p.io_mode = 'function' AND s.problem_id IS NULL AND p.difficulty = ANY($1)
		ORDER  BY p.slug`, difficulties)
	if err != nil {
		return nil, err
	}
	var out []problem
	for rows.Next() {
		var p problem
		if err := rows.Scan(&p.ID, &p.Slug, &p.Difficulty, &p.JS); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	rows.Close()

	for i := range out {
		p := &out[i]
		tcRows, err := pool.Query(ctx,
			`SELECT id, input, expected_output FROM test_cases WHERE problem_id=$1 ORDER BY order_index`, p.ID)
		if err != nil {
			return nil, err
		}
		for tcRows.Next() {
			var tc testCase
			if err := tcRows.Scan(&tc.ID, &tc.Input, &tc.Expected); err != nil {
				return nil, err
			}
			p.Cases = append(p.Cases, tc)
		}
		tcRows.Close()
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func fatal(f string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "infer-signatures: "+f+"\n", a...)
	os.Exit(1)
}

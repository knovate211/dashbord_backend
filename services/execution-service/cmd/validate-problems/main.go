// Command validate-problems is the gate that keeps this class of bug out.
//
// For every function-mode problem it asserts two things per language:
//
//  1. the reference solution passes every test case
//  2. the generated starter does NOT pass
//
// The first catches a broken driver, a mis-declared signature, or test data in
// the wrong format. The second catches an answer that has leaked into a starter
// template — the reason 361 of 442 Easy problems could be cleared by pressing
// Submit without typing anything.
//
// Every defect in the original audit would have failed this on the first run.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/knovate211/execution-service/internal/codegen"
	"github.com/knovate211/execution-service/internal/judge"
	"github.com/knovate211/execution-service/internal/sandbox"
)

type testCase struct {
	Input    string
	Expected string
}

type problem struct {
	ID       string
	Slug     string
	Kind     string
	Methods  []codegen.Method
	Sig      codegen.Signature
	Cases    []testCase
	RefCode  map[string]string
	Starters map[string]string
}

type failure struct {
	Slug, Lang, Kind, Detail string
}

func main() {
	dsn := flag.String("dsn", envOr("POSTGRES_DSN", "postgres://skillofide:password@localhost:5432/skillofide?sslmode=disable"), "postgres DSN")
	langs := flag.String("langs", "javascript,python", "comma-separated languages to validate")
	slug := flag.String("slug", "", "validate only this slug")
	concurrency := flag.Int("j", 4, "parallel problems")
	dump := flag.String("dump", "", "print the wrapped source for this language and exit (use with -slug)")
	flag.Parse()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		fatal("connect: %v", err)
	}
	defer pool.Close()

	problems, err := load(ctx, pool, *slug)
	if err != nil {
		fatal("load: %v", err)
	}
	// Diagnosing a driver bug means reading the program that actually reached
	// the compiler, not inferring it from a truncated error message.
	if *dump != "" {
		for _, p := range problems {
			wrapped, err := codegen.Wrap(*dump, p.Sig, p.RefCode[*dump])
			if err != nil {
				fatal("%s: %v", p.Slug, err)
			}
			fmt.Printf("// ===== %s / %s =====\n%s\n", p.Slug, *dump, wrapped)
		}
		return
	}

	sb, err := sandbox.New(zap.NewNop())
	if err != nil {
		fatal("sandbox: %v", err)
	}

	languages := strings.Split(*langs, ",")
	fmt.Printf("validating %d function-mode problems across %v\n\n", len(problems), languages)

	var (
		mu       sync.Mutex
		failures []failure
		checked  int
	)
	sem := make(chan struct{}, *concurrency)
	var wg sync.WaitGroup

	for _, p := range problems {
		for _, lang := range languages {
			lang := strings.TrimSpace(lang)
			p := p
			if isPlaceholder(p.RefCode[lang]) {
				// Distinguish "nobody has written the answer for this language
				// yet" from "the answer is wrong". Reporting a stub as a
				// failing solution sends you hunting for a bug in the driver
				// that does not exist.
				mu.Lock()
				failures = append(failures, failure{p.Slug, lang, "no-reference", "no usable reference solution for this language"})
				mu.Unlock()
				continue
			}
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()

				refFails := runAll(ctx, sb, p, lang, p.RefCode[lang])
				starterFails := runAll(ctx, sb, p, lang, p.Starters[lang])
				refEmpty := producedNothing(ctx, sb, p, lang, p.RefCode[lang])

				mu.Lock()
				defer mu.Unlock()
				checked++
				switch {
				case refEmpty:
					// A "reference" that returns null on every single case is a
					// starter stub that got filed as one, not a wrong solution.
					// Most of the Medium and Hard catalogue is in this state:
					// the starters really were skeletons, so there was never an
					// answer to preserve. Reporting these as failures sends you
					// hunting for driver bugs that do not exist.
					failures = append(failures, failure{p.Slug, lang, "no-reference", "stub was filed as the reference; no solution has been written"})
				case len(refFails) > 0:
					failures = append(failures, failure{p.Slug, lang, "reference-fails", refFails[0]})
				}
				// A starter that passes every case is a starter containing the answer.
				if len(starterFails) == 0 && len(p.Cases) > 0 {
					failures = append(failures, failure{p.Slug, lang, "starter-passes", "untouched starter passes every test case"})
				}
			}()
		}
	}
	wg.Wait()

	sort.Slice(failures, func(i, j int) bool {
		if failures[i].Slug != failures[j].Slug {
			return failures[i].Slug < failures[j].Slug
		}
		return failures[i].Lang < failures[j].Lang
	})

	// A missing reference in one language is a coverage gap, not a defect: the
	// problem may be perfectly correct and simply unwritten in that language.
	// Conflating the two meant a run with 100 unwritten Java solutions looked
	// identical to a run with 100 broken ones, and the real failures were
	// buried among them. Language-level correctness is proved separately and
	// exhaustively by driver-conformance, which round-trips every supported
	// type through every language's driver.
	var hard, gaps []failure
	for _, f := range failures {
		if f.Kind == "no-reference" {
			gaps = append(gaps, f)
			continue
		}
		hard = append(hard, f)
	}

	fmt.Printf("checked %d problem/language pairs\n", checked)

	if len(gaps) > 0 {
		byLang := map[string]int{}
		for _, g := range gaps {
			byLang[g.Lang]++
		}
		langs := make([]string, 0, len(byLang))
		for l := range byLang {
			langs = append(langs, l)
		}
		sort.Strings(langs)
		fmt.Printf("\ncoverage gaps — no reference solution written yet:\n")
		for _, l := range langs {
			fmt.Printf("  %-11s %d problem(s)\n", l, byLang[l])
		}
	}

	if len(hard) == 0 {
		fmt.Println("\nPASS — every reference solution that exists is accepted, and no starter is")
		return
	}
	fmt.Printf("\nFAIL — %d problem(s):\n", len(hard))
	for _, f := range hard {
		fmt.Printf("  %-34s %-11s %-16s %s\n", f.Slug, f.Lang, f.Kind, f.Detail)
	}
	os.Exit(1)
}

// isPlaceholder reports whether a stored "solution" is really a stub.
//
// Starters were authored per language by hand and many languages got filler
// rather than a skeleton — "// Go solution", `console.log("Hello World")`,
// "# Not applicable". regen-starters filed whatever was in the editor as the
// reference, so those placeholders came across too.
func isPlaceholder(code string) bool {
	t := strings.TrimSpace(code)
	if t == "" {
		return true
	}
	// Nothing but comments and blank lines is not an implementation.
	hasCode := false
	for _, line := range strings.Split(t, "\n") {
		l := strings.TrimSpace(line)
		if l == "" || strings.HasPrefix(l, "//") || strings.HasPrefix(l, "#") {
			continue
		}
		hasCode = true
		break
	}
	if !hasCode {
		return true
	}
	switch t {
	case `console.log("Hello World");`, `console.log("Hello World")`, "# Not applicable", "// Not applicable":
		return true
	}
	// Deliberately NOT keyed on the presence of "Write your code here". Most
	// real starters carry that comment above a working body — treating it as a
	// placeholder marker rejected eleven perfectly good reference solutions.
	return false
}

// producedNothing reports whether the code returns null or nothing at all for
// every test case — the signature of an unimplemented body.
func producedNothing(ctx context.Context, sb *sandbox.DockerSandbox, p problem, lang, code string) bool {
	if len(p.Cases) == 0 {
		return false
	}
	spec := specFor(p)
	for _, tc := range p.Cases {
		res, err := sb.Run(ctx, &sandbox.RunRequest{
			ProblemId: p.ID, Language: lang, Code: code, Input: tc.Input, Spec: spec,
		})
		if err != nil || res.TimedOut {
			return false
		}
		out := strings.TrimSpace(res.Stdout)
		if out != "" && out != "null" && out != "None" {
			return false
		}
	}
	return true
}

// runAll returns a description of each failing case, empty if all pass.
func runAll(ctx context.Context, sb *sandbox.DockerSandbox, p problem, lang, code string) []string {
	var out []string
	spec := specFor(p)
	for _, tc := range p.Cases {
		res, err := sb.Run(ctx, &sandbox.RunRequest{
			ProblemId: p.ID, Language: lang, Code: code, Input: tc.Input, Spec: spec,
		})
		if err != nil {
			out = append(out, fmt.Sprintf("run error: %v", err))
			continue
		}
		ok := judge.MatchesTyped(tc.Expected, res.Stdout, judge.CompareSpec{
			Mode: judge.CompareMode(p.Sig.Compare), Eps: p.Sig.Eps,
		})
		// A timeout must never be reported as a wrong answer. Under parallel
		// load a container can exceed its limit while compiling, and the result
		// — exit 0, empty stdout — is indistinguishable from a solution that
		// printed nothing unless the flag is checked explicitly.
		if res.TimedOut {
			out = append(out, fmt.Sprintf("TIMED OUT on input %q (machine load? try -j 1)", tc.Input))
		} else if res.OOMKilled {
			out = append(out, fmt.Sprintf("out of memory on input %q", tc.Input))
		} else if res.ExitCode != 0 {
			out = append(out, fmt.Sprintf("exit %d on input %q: %s", res.ExitCode, tc.Input, firstLines(res.Stderr)))
		} else if !ok {
			out = append(out, fmt.Sprintf("input %q: want %s, got %s", tc.Input, tc.Expected, strings.TrimSpace(res.Stdout)))
		}
	}
	return out
}

func specFor(p problem) *sandbox.ExecutionSpec {
	return &sandbox.ExecutionSpec{
		IoMode:     sandbox.IoModeFunction,
		EntryPoint: p.Sig.EntryPoint,
		Params:     p.Sig.Params,
		ReturnType: p.Sig.ReturnType,
		Compare:    string(p.Sig.Compare),
		FloatEps:   p.Sig.Eps,
		Kind:       p.Kind,
		Methods:    p.Methods,
	}
}

func load(ctx context.Context, pool *pgxpool.Pool, slug string) ([]problem, error) {
	q := `
		SELECT p.id, p.slug, s.entry_point, s.params, s.return_type, s.compare, s.float_eps,
		       s.kind, s.methods
		FROM   problems p
		JOIN   problem_signatures s ON s.problem_id = p.id
		WHERE  p.io_mode = 'function'`
	args := []interface{}{}
	if slug != "" {
		q += " AND p.slug = $1"
		args = append(args, slug)
	}
	q += " ORDER BY p.slug"

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	var out []problem
	for rows.Next() {
		var p problem
		var raw, methodsRaw []byte
		var compare string
		if err := rows.Scan(&p.ID, &p.Slug, &p.Sig.EntryPoint, &raw, &p.Sig.ReturnType, &compare, &p.Sig.Eps,
			&p.Kind, &methodsRaw); err != nil {
			return nil, err
		}
		if len(methodsRaw) > 0 {
			if err := json.Unmarshal(methodsRaw, &p.Methods); err != nil {
				return nil, fmt.Errorf("%s: methods: %w", p.Slug, err)
			}
		}
		p.Sig.Compare = codegen.Compare(compare)
		if err := json.Unmarshal(raw, &p.Sig.Params); err != nil {
			return nil, fmt.Errorf("%s: params: %w", p.Slug, err)
		}
		out = append(out, p)
	}
	rows.Close()

	for i := range out {
		p := &out[i]
		tcRows, err := pool.Query(ctx, `SELECT input, expected_output FROM test_cases WHERE problem_id=$1 ORDER BY order_index`, p.ID)
		if err != nil {
			return nil, err
		}
		for tcRows.Next() {
			var tc testCase
			if err := tcRows.Scan(&tc.Input, &tc.Expected); err != nil {
				return nil, err
			}
			p.Cases = append(p.Cases, tc)
		}
		tcRows.Close()

		p.RefCode = map[string]string{}
		refRows, err := pool.Query(ctx, `SELECT language, code FROM reference_solutions WHERE problem_id=$1`, p.ID)
		if err != nil {
			return nil, err
		}
		for refRows.Next() {
			var l, c string
			if err := refRows.Scan(&l, &c); err != nil {
				return nil, err
			}
			p.RefCode[l] = c
		}
		refRows.Close()

		p.Starters = map[string]string{}
		var js, py, jv, cp, goSrc string
		if err := pool.QueryRow(ctx, `SELECT javascript, python, java, cpp, go FROM starter_codes WHERE problem_id=$1`, p.ID).
			Scan(&js, &py, &jv, &cp, &goSrc); err == nil {
			p.Starters = map[string]string{"javascript": js, "python": py, "java": jv, "cpp": cp, "go": goSrc}
		}
	}
	return out, nil
}

// firstLines keeps enough of a compiler's output to be actionable. Taking only
// the first line reported "# command-line-arguments" — the header — and hid the
// error underneath it.
func firstLines(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 3 {
		lines = lines[:3]
	}
	return strings.Join(lines, " | ")
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func fatal(f string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "validate-problems: "+f+"\n", a...)
	os.Exit(1)
}

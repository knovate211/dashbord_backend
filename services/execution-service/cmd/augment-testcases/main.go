// Command augment-testcases adds test cases to problems that have too few.
//
// A problem with one visible case and no hidden case is not really graded: a
// learner reads the expected output and returns it as a constant. jump-game
// shipped a single case, [2,3,1,1,4] -> true, which `return true` satisfies.
//
// Inputs are authored by hand — knowing that an empty array, a single element
// and a duplicate-heavy case are the interesting shapes is judgement, not
// something to generate. Expected outputs are NOT authored: they are computed
// by running the problem's own reference solution, which validate-problems has
// already checked against the existing cases. Hand-computing them is how you
// get a test that asserts the wrong answer, and a wrong expectation is worse
// than a missing one — it fails correct submissions.
//
// Everything added is hidden. The visible cases are what the statement's
// examples show; these exist to stop the answer being read off them.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/knovate211/execution-service/internal/codegen"
	"github.com/knovate211/execution-service/internal/sandbox"
)

// spec is the authored half: which problem, and what inputs to add.
type spec struct {
	Slug   string   `json:"slug"`
	Inputs []string `json:"inputs"`
}

func main() {
	dsn := flag.String("dsn", envOr("POSTGRES_DSN", "postgres://skillofide:password@localhost:5432/skillofide?sslmode=disable"), "postgres DSN")
	file := flag.String("f", "", "JSON file of {slug, inputs[]} entries")
	apply := flag.Bool("apply", false, "write the generated cases")
	flag.Parse()

	if *file == "" {
		fatal("-f is required")
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		fatal("read %s: %v", *file, err)
	}
	var specs []spec
	if err := json.Unmarshal(raw, &specs); err != nil {
		fatal("decode %s: %v", *file, err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		fatal("connect: %v", err)
	}
	defer pool.Close()

	sb, err := sandbox.New(zap.NewNop())
	if err != nil {
		fatal("sandbox: %v", err)
	}

	added, skipped := 0, 0
	for _, sp := range specs {
		var (
			problemID  string
			entryPoint string
			paramsRaw  []byte
			returnType string
			compare    string
			eps        float64
			refCode    string
			nextIndex  int
		)
		err := pool.QueryRow(ctx, `
			SELECT p.id, s.entry_point, s.params, s.return_type, s.compare, s.float_eps,
			       coalesce(r.code, ''),
			       coalesce((SELECT max(order_index) + 1 FROM test_cases WHERE problem_id = p.id), 0)
			FROM   problems p
			JOIN   problem_signatures s ON s.problem_id = p.id
			LEFT   JOIN reference_solutions r ON r.problem_id = p.id AND r.language = 'javascript'
			WHERE  p.slug = $1`, sp.Slug).
			Scan(&problemID, &entryPoint, &paramsRaw, &returnType, &compare, &eps, &refCode, &nextIndex)
		if err != nil {
			fmt.Printf("  SKIP  %-42s %v\n", sp.Slug, err)
			skipped++
			continue
		}
		if strings.TrimSpace(refCode) == "" {
			fmt.Printf("  SKIP  %-42s no reference solution to compute expectations from\n", sp.Slug)
			skipped++
			continue
		}

		var params []codegen.Param
		if err := json.Unmarshal(paramsRaw, &params); err != nil {
			fatal("%s: params: %v", sp.Slug, err)
		}
		spec := &sandbox.ExecutionSpec{
			IoMode:     sandbox.IoModeFunction,
			EntryPoint: entryPoint,
			Params:     params,
			ReturnType: codegen.Type(returnType),
			Compare:    compare,
			FloatEps:   eps,
		}

		for _, in := range sp.Inputs {
			res, err := sb.Run(ctx, &sandbox.RunRequest{
				ProblemId: problemID, Language: "javascript", Code: refCode, Input: in, Spec: spec,
			})
			if err != nil || res.TimedOut || res.ExitCode != 0 {
				detail := "run error"
				if res != nil && res.TimedOut {
					detail = "timed out"
				} else if res != nil && res.ExitCode != 0 {
					detail = strings.TrimSpace(firstLine(res.Stderr))
				}
				fmt.Printf("  SKIP  %-42s input %q: %s\n", sp.Slug, truncate(in, 30), detail)
				skipped++
				continue
			}
			expected := strings.TrimSpace(res.Stdout)
			if expected == "" {
				fmt.Printf("  SKIP  %-42s input %q produced no output\n", sp.Slug, truncate(in, 30))
				skipped++
				continue
			}

			fmt.Printf("  add   %-42s %q -> %s\n", sp.Slug, truncate(in, 30), truncate(expected, 40))
			added++
			if *apply {
				if _, err := pool.Exec(ctx, `
					INSERT INTO test_cases (problem_id, input, expected_output, is_hidden, order_index)
					VALUES ($1, $2, $3, true, $4)`, problemID, in, expected, nextIndex); err != nil {
					fatal("%s: insert: %v", sp.Slug, err)
				}
				nextIndex++
			}
		}
	}

	fmt.Printf("\n%d case(s) to add, %d skipped\n", added, skipped)
	if !*apply {
		fmt.Println("dry run — pass -apply to write")
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
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
	fmt.Fprintf(os.Stderr, "augment-testcases: "+f+"\n", a...)
	os.Exit(1)
}

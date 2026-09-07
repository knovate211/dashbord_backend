// Command regen-starters rewrites starter_codes for every function-mode problem
// from its declared signature, and files the previous contents away as the
// reference solution.
//
// It exists because starters were authored by hand per language, which is how
// 361 of the 442 Easy problems ended up shipping a working solution in the
// editor while 66 others shipped `console.log("Hello World")`. Generating them
// means a starter cannot drift from the signature the judge enforces, and
// cannot contain the answer.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/knovate211/execution-service/internal/codegen"
)

type row struct {
	id         string
	slug       string
	entryPoint string
	paramsRaw  []byte
	returnType string
	compare    string
	kind       string
	methodsRaw []byte
	starters   map[string]string
}

func main() {
	dsn := flag.String("dsn", envOr("POSTGRES_DSN", "postgres://skillofide:password@localhost:5432/skillofide?sslmode=disable"), "postgres DSN")
	apply := flag.Bool("apply", false, "write changes; without this the run is a dry run")
	flag.Parse()

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		fatal("connect: %v", err)
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `
		SELECT p.id, p.slug, s.entry_point, s.params, s.return_type, s.compare,
		       s.kind, s.methods,
		       sc.javascript, sc.python, sc.java, sc.cpp, sc.go
		FROM   problems p
		JOIN   problem_signatures s ON s.problem_id = p.id
		LEFT   JOIN starter_codes sc ON sc.problem_id = p.id
		WHERE  p.io_mode = 'function'
		ORDER  BY p.slug`)
	if err != nil {
		fatal("query: %v", err)
	}

	var all []row
	for rows.Next() {
		var r row
		var js, py, jv, cp, goSrc *string
		if err := rows.Scan(&r.id, &r.slug, &r.entryPoint, &r.paramsRaw, &r.returnType, &r.compare,
			&r.kind, &r.methodsRaw,
			&js, &py, &jv, &cp, &goSrc); err != nil {
			fatal("scan: %v", err)
		}
		r.starters = map[string]string{
			"javascript": deref(js), "python": deref(py),
			"java": deref(jv), "cpp": deref(cp), "go": deref(goSrc),
		}
		all = append(all, r)
	}
	rows.Close()

	langs := []string{"javascript", "python", "java", "cpp", "go"}
	changed := 0

	for _, r := range all {
		var params []codegen.Param
		if err := json.Unmarshal(r.paramsRaw, &params); err != nil {
			fatal("%s: decode params: %v", r.slug, err)
		}
		sig := codegen.Signature{
			EntryPoint: r.entryPoint,
			Params:     params,
			ReturnType: codegen.Type(r.returnType),
			Compare:    codegen.Compare(r.compare),
		}

		// Class-design problems are generated from a different shape: a
		// constructor plus methods, not a single entry point. Running them
		// through the function generator would emit a starter with no methods
		// at all and quietly replace the real one.
		var classSig codegen.ClassSignature
		isClass := r.kind == "class"
		if isClass {
			var methods []codegen.Method
			if err := json.Unmarshal(r.methodsRaw, &methods); err != nil {
				fatal("%s: decode methods: %v", r.slug, err)
			}
			classSig = codegen.ClassSignature{ClassName: r.entryPoint, Ctor: params, Methods: methods}
		}

		// A language that cannot model this signature keeps whatever starter it
		// already had. Aborting the whole run instead — which is what this used
		// to do — meant one tree problem stopped every other problem from being
		// regenerated, and the failure came alphabetically first so nothing ran
		// at all.
		generated := make(map[string]string, len(langs))
		var unsupported []string
		for _, lang := range langs {
			var out string
			var err error
			if isClass {
				out, err = codegen.StarterClass(lang, classSig)
			} else {
				out, err = codegen.Starter(lang, sig)
			}
			if err != nil {
				unsupported = append(unsupported, lang)
				generated[lang] = r.starters[lang]
				continue
			}
			generated[lang] = out
		}

		note := ""
		if len(unsupported) > 0 {
			note = fmt.Sprintf("   [kept existing starter for: %s]", strings.Join(unsupported, ", "))
		}
		if isClass {
			fmt.Printf("%-34s class %s (%d methods)%s\n", r.slug, classSig.ClassName, len(classSig.Methods), note)
		} else {
			fmt.Printf("%-34s %s(%d params) -> %s%s\n", r.slug, sig.EntryPoint, len(params), sig.ReturnType, note)
		}
		if !*apply {
			continue
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			fatal("%s: begin: %v", r.slug, err)
		}
		// Preserve whatever was in the editor as the reference solution before
		// overwriting it. For most problems that text IS the answer, which is
		// exactly what the CI gate needs to run against the test cases.
		for _, lang := range langs {
			prev := r.starters[lang]
			if prev == "" || generated[lang] == prev {
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO reference_solutions (problem_id, language, code)
				VALUES ($1, $2, $3)
				ON CONFLICT (problem_id, language) DO NOTHING`, r.id, lang, prev); err != nil {
				tx.Rollback(ctx) //nolint:errcheck
				fatal("%s/%s: save reference: %v", r.slug, lang, err)
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE starter_codes
			SET javascript=$2, python=$3, java=$4, cpp=$5, go=$6
			WHERE problem_id=$1`,
			r.id, generated["javascript"], generated["python"],
			generated["java"], generated["cpp"], generated["go"]); err != nil {
			tx.Rollback(ctx) //nolint:errcheck
			fatal("%s: update starters: %v", r.slug, err)
		}
		if err := tx.Commit(ctx); err != nil {
			fatal("%s: commit: %v", r.slug, err)
		}
		changed++
	}

	if *apply {
		fmt.Printf("\nregenerated starters for %d problems\n", changed)
	} else {
		fmt.Printf("\ndry run: %d problems would be regenerated (pass -apply to write)\n", len(all))
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func fatal(f string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "regen-starters: "+f+"\n", a...)
	os.Exit(1)
}

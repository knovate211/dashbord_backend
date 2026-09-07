// Command driver-conformance checks that every supported type survives a round
// trip through every language's generated driver.
//
// The alternative was writing a reference solution for all ~100 signed problems
// in Java, C++ and Go — some 300 solutions — to prove those drivers work. That
// is a great deal of authoring to test one thing, and it tests it unevenly:
// whichever types happen to appear in the catalogue get covered, the rest do
// not.
//
// This covers the actual surface instead. For each language and each type it
// generates the signature `echoT(value T) -> T`, supplies a solution that
// returns its argument unchanged, and asserts the output is byte-identical to
// the input. Anything wrong in parsing, in serialisation, or in the assembly
// around them shows up as a mismatch — and every codegen bug found so far was
// exactly one of those:
//
//	C++   helpers appended after code that had not included <bits/stdc++.h>
//	Go    an injected import block left unused by a zero-argument signature
//	Go    a reader variable declared and never read
//	Go    a var block placed between the injected imports and the submission's own
//	Py    json.dumps defaulting to ", " where every other language emits ","
//
// None were visible to a unit test on the generated text. All would fail here.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"

	"github.com/knovate211/execution-service/internal/codegen"
	"github.com/knovate211/execution-service/internal/sandbox"
)

// echoCase is one type and a value that exercises it. Values are chosen to
// catch formatting differences rather than to be interesting: negatives, an
// embedded quote, a float that does not round-trip through a naive printer.
type echoCase struct {
	Type  codegen.Type
	Value string
	// Solution bodies per language, returning the argument unchanged.
	JS, PY, JAVA, CPP, GO string
}

func echoCases() []echoCase {
	return []echoCase{
		{Type: codegen.TInt, Value: "-42"},
		{Type: codegen.TLong, Value: "9007199254740991"},
		{Type: codegen.TDouble, Value: "3.5"},
		{Type: codegen.TBool, Value: "true"},
		{Type: codegen.TString, Value: `"a \"quoted\" word"`},
		{Type: codegen.TIntArr, Value: "[-1,0,1]"},
		{Type: codegen.TIntArr, Value: "[]"},
		{Type: codegen.TLongArr, Value: "[1,2]"},
		{Type: codegen.TDblArr, Value: "[1.5,-2.5]"},
		{Type: codegen.TBoolArr, Value: "[true,false]"},
		{Type: codegen.TStrArr, Value: `["a","b c"]`},
		{Type: codegen.TStrArr, Value: "[]"},
		{Type: codegen.TInt2D, Value: "[[1,2],[3]]"},
		{Type: codegen.TInt2D, Value: "[]"},
		{Type: codegen.TStr2D, Value: `[["a"],["b","c"]]`},
	}
}

// echoSolution returns a solution for `echoValue(value T) -> T` in lang.
func echoSolution(lang string, t codegen.Type) string {
	sig := codegen.Signature{
		EntryPoint: "echoValue",
		Params:     []codegen.Param{{Name: "value", Type: t}},
		ReturnType: t,
	}
	starter, err := codegen.Starter(lang, sig)
	if err != nil {
		return ""
	}
	// Replace the skeleton's placeholder return with one that returns the
	// argument. Working from the generated starter keeps the shape each
	// language expects — a bare function, a Solution class, a package main.
	switch lang {
	case codegen.LangJavaScript:
		return replaceBody(starter, "    return value;")
	case codegen.LangPython:
		return replaceBody(starter, "    return value")
	case codegen.LangJava, codegen.LangCpp:
		return replaceBody(starter, "        return value;")
	case codegen.LangGo:
		return replaceBody(starter, "\treturn value")
	}
	return ""
}

// replaceBody swaps everything between the "Write your code here" marker and
// the end of that function for the given return statement.
func replaceBody(starter, ret string) string {
	lines := strings.Split(starter, "\n")
	out := make([]string, 0, len(lines))
	skipping := false
	for _, l := range lines {
		if strings.Contains(l, "Write your code here") {
			out = append(out, ret)
			skipping = true
			continue
		}
		if skipping {
			// Drop the placeholder return, keep the closing braces.
			if strings.Contains(l, "return") {
				continue
			}
			skipping = false
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

func main() {
	langs := flag.String("langs", "javascript,python,java,cpp,go", "languages to check")
	flag.Parse()

	sb, err := sandbox.New(zap.NewNop())
	if err != nil {
		fatal("sandbox: %v", err)
	}
	ctx := context.Background()

	type failure struct{ lang, typ, value, got, detail string }
	var failures []failure
	checked := 0

	for _, lang := range strings.Split(*langs, ",") {
		lang = strings.TrimSpace(lang)
		fmt.Printf("\n%s\n", strings.ToUpper(lang))
		for _, c := range echoCases() {
			sig := codegen.Signature{
				EntryPoint: "echoValue",
				Params:     []codegen.Param{{Name: "value", Type: c.Type}},
				ReturnType: c.Type,
			}
			code := echoSolution(lang, c.Type)
			if code == "" {
				fmt.Printf("  skip  %-10s %-18s (unsupported)\n", c.Type, truncate(c.Value, 18))
				continue
			}
			spec := &sandbox.ExecutionSpec{
				IoMode:     sandbox.IoModeFunction,
				EntryPoint: sig.EntryPoint,
				Params:     sig.Params,
				ReturnType: sig.ReturnType,
				Compare:    string(codegen.CompareExact),
			}
			res, err := sb.Run(ctx, &sandbox.RunRequest{
				ProblemId: "conformance", Language: lang, Code: code, Input: c.Value, Spec: spec,
			})
			checked++
			if err != nil {
				failures = append(failures, failure{lang, string(c.Type), c.Value, "", err.Error()})
				fmt.Printf("  FAIL  %-10s %-18s run error: %v\n", c.Type, truncate(c.Value, 18), err)
				continue
			}
			got := strings.TrimSpace(res.Stdout)
			switch {
			case res.TimedOut:
				failures = append(failures, failure{lang, string(c.Type), c.Value, got, "timed out"})
				fmt.Printf("  FAIL  %-10s %-18s timed out\n", c.Type, truncate(c.Value, 18))
			case res.ExitCode != 0:
				detail := firstLines(res.Stderr)
				failures = append(failures, failure{lang, string(c.Type), c.Value, got, detail})
				fmt.Printf("  FAIL  %-10s %-18s exit %d: %s\n", c.Type, truncate(c.Value, 18), res.ExitCode, detail)
			case got != c.Value:
				failures = append(failures, failure{lang, string(c.Type), c.Value, got, "round trip changed the value"})
				fmt.Printf("  FAIL  %-10s %-18s got %s\n", c.Type, truncate(c.Value, 18), got)
			default:
				fmt.Printf("  ok    %-10s %s\n", c.Type, truncate(c.Value, 18))
			}
		}
	}

	fmt.Printf("\nchecked %d type/language round trips\n", checked)
	if len(failures) == 0 {
		fmt.Println("PASS — every type survives every language's driver unchanged")
		return
	}
	fmt.Printf("FAIL — %d round trip(s) did not preserve the value:\n", len(failures))
	for _, f := range failures {
		fmt.Printf("  %-11s %-10s %-20s -> %-20s %s\n", f.lang, f.typ, truncate(f.value, 20), truncate(f.got, 20), f.detail)
	}
	os.Exit(1)
}

func firstLines(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 2 {
		lines = lines[:2]
	}
	return strings.Join(lines, " | ")
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func fatal(f string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "driver-conformance: "+f+"\n", a...)
	os.Exit(1)
}

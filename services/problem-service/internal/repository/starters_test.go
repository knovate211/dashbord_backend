package repository

import (
	"testing"

	problemv1 "github.com/knovate211/proto/problem/v1"
)

func starterCodesFor(js, py, java, cpp, goSrc string) problemv1.StarterCodes {
	return problemv1.StarterCodes{Javascript: js, Python: py, Java: java, Cpp: cpp, Go: goSrc}
}

func TestPlaceholderStarter(t *testing.T) {
	filler := map[string]string{
		"js completed":   `console.log("Completed");`,
		"py completed":   `print("Completed")`,
		"java completed": "public class Solution {\n    public static void main(String[] args) {\n        System.out.println(\"Completed\");\n    }\n}",
		"cpp completed":  "#include <iostream>\nusing namespace std;\nint main() {\n    cout << \"Completed\" << endl;\n    return 0;\n}",
		"js hello world": `console.log("Hello World");`,
		"py write here":  "# Write your code here",
		"not applicable": "# Not applicable",
		"empty":          "   \n  ",
		"comments only":  "// nothing here\n// really nothing",
		"jsdoc only":     "/**\n * @param {int} n\n */",
	}
	for name, code := range filler {
		if !placeholderStarter(code) {
			t.Errorf("%s: expected placeholder, got real starter:\n%s", name, code)
		}
	}

	real := map[string]string{
		"generated js":   "/**\n * @param {number[]} nums\n * @return {number[]}\n */\nfunction twoSum(nums) {\n    // Write your code here\n    return [];\n}",
		"generated py":   "def twoSum(nums: list[int]) -> list[int]:\n    # Write your code here\n    return []",
		"generated java": "public class Solution {\n    public int[] twoSum(int[] nums) {\n        // Write your code here\n        return null;\n    }\n}",
		"go stdio":       "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tvar a, b int\n\tfmt.Scan(&a, &b)\n\tfmt.Println(a + b)\n}",
		"sql skeleton":   "-- Table: World(name, area, population)\nSELECT\nFROM World\n;",
	}
	for name, code := range real {
		if placeholderStarter(code) {
			t.Errorf("%s: expected real starter, got placeholder:\n%s", name, code)
		}
	}
}

func TestSupportedLanguages(t *testing.T) {
	sqlOnly := func() []string {
		sc := starterCodesFor("x", "x", "x", "x", "x")
		return supportedLanguages("sql", &sc)
	}()
	if len(sqlOnly) != 1 || sqlOnly[0] != "sql" {
		t.Errorf("sql problems must offer only sql, got %v", sqlOnly)
	}

	// stdio problems are Go exercises regardless of what the other columns
	// happen to contain — including the Java and C++ filler that wraps an empty
	// body in a real class and a real main, which no content heuristic catches.
	sc := starterCodesFor(
		`console.log("Hello World");`,
		"# Write your code here",
		"public class Solution {\n    public static void main(String[] args) {\n        // Write your code here\n    }\n}",
		"#include <iostream>\nusing namespace std;\nint main() {\n    // Write your code here\n    return 0;\n}",
		"package main\n\nimport \"fmt\"\n\nfunc main() {\n\tvar a, b int\n\tfmt.Scan(&a, &b)\n\tfmt.Println(a + b)\n}",
	)
	got := supportedLanguages("stdio", &sc)
	if len(got) != 1 || got[0] != "go" {
		t.Errorf("stdio Go exercise should offer only go, got %v", got)
	}

	// Never return an empty list — the editor would have no language at all.
	empty := starterCodesFor("", "", "", "", "")
	if len(supportedLanguages("function", &empty)) == 0 {
		t.Error("supportedLanguages must never return an empty list")
	}
}

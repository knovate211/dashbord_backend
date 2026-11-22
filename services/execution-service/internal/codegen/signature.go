// Package codegen turns a problem's declared signature into the two artefacts
// that must agree about it: the starter template the learner edits, and the
// driver the judge wraps around what they submit.
//
// Generating both from one source is the point. The previous design inferred
// the entry point with a regex over the submission and guessed the argument
// list by trying to JSON-decode the whole test input as an array, which broke
// for multi-line inputs, string returns, and any file whose first function was
// not the answer.
package codegen

import (
	"fmt"
	"strings"
)

// Type is the small language used to describe parameters and return values.
// It is deliberately narrow: every type here has an unambiguous JSON encoding
// and a natural spelling in all five supported languages.
type Type string

const (
	TInt     Type = "int"
	TLong    Type = "long"
	TDouble  Type = "double"
	TBool    Type = "bool"
	TString  Type = "string"
	TChar    Type = "char"
	TVoid    Type = "void"
	TIntArr  Type = "int[]"
	TLongArr Type = "long[]"
	TDblArr  Type = "double[]"
	TBoolArr Type = "bool[]"
	TStrArr  Type = "string[]"
	TInt2D   Type = "int[][]"
	TStr2D   Type = "string[][]"

	// TTreeNode and TListNode are linked structures, serialised in test data as
	// a flat array: a binary tree in level order with null for absent children
	// ([3,9,20,null,null,15,7]), a list as its values in order ([1,2,3,4]).
	//
	// They are distinct types rather than int[] because the driver has to build
	// the structure before the entry point sees it. Typing them as arrays — the
	// obvious reading of the test data — produces a driver that hands an array
	// to a function expecting a tree: it compiles, runs, and grades wrongly.
	TTreeNode Type = "TreeNode"
	TListNode Type = "ListNode"
)

// Linked returns whether t is a structure the driver must build from its
// serialised form rather than decode directly.
func (t Type) Linked() bool { return t == TTreeNode || t == TListNode }

var supported = map[Type]bool{
	TInt: true, TLong: true, TDouble: true, TBool: true, TString: true,
	TChar: true, TVoid: true, TIntArr: true, TLongArr: true, TDblArr: true,
	TBoolArr: true, TStrArr: true, TInt2D: true, TStr2D: true,
	TTreeNode: true, TListNode: true,
}

// Valid reports whether t is a type the generators know how to emit.
func (t Type) Valid() bool { return supported[t] }

// Param is one argument of the entry point, in declaration order. That order
// is also the order of the lines in a test case's input.
type Param struct {
	Name string `json:"name"`
	Type Type   `json:"type"`
}

// Compare selects how the judge decides whether output matches expectation.
type Compare string

const (
	// CompareExact is deep equality after JSON decoding. Formatting differences
	// — "[0, 1]" versus "[0,1]", Python's True versus JSON's true — do not
	// matter, which is what string comparison got wrong.
	CompareExact Compare = "exact"
	// CompareUnordered sorts the top level before comparing, for the many
	// problems whose statement says "in any order" but whose judge did not.
	CompareUnordered Compare = "unordered"
	// CompareSet ignores duplicates as well as order.
	CompareSet Compare = "set"
	// CompareFloat compares numerically within Eps.
	CompareFloat Compare = "float"
)

// Signature is the full contract for one problem.
type Signature struct {
	EntryPoint string  `json:"entryPoint"`
	Params     []Param `json:"params"`
	ReturnType Type    `json:"returnType"`
	Compare    Compare `json:"compare"`
	Eps        float64 `json:"floatEps"`
}

// Validate rejects a signature the generators cannot faithfully emit. It is
// called before generation rather than letting a bad signature produce code
// that fails to compile inside the sandbox, where the learner would see the
// compiler error and reasonably assume it was their fault.
func (s Signature) Validate() error {
	if strings.TrimSpace(s.EntryPoint) == "" {
		return fmt.Errorf("codegen: entry point is empty")
	}
	if !s.ReturnType.Valid() {
		return fmt.Errorf("codegen: unsupported return type %q", s.ReturnType)
	}
	seen := make(map[string]bool, len(s.Params))
	for i, p := range s.Params {
		if strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("codegen: parameter %d has no name", i)
		}
		if seen[p.Name] {
			return fmt.Errorf("codegen: duplicate parameter name %q", p.Name)
		}
		seen[p.Name] = true
		if !p.Type.Valid() {
			return fmt.Errorf("codegen: parameter %q has unsupported type %q", p.Name, p.Type)
		}
		if p.Type == TVoid {
			return fmt.Errorf("codegen: parameter %q cannot be void", p.Name)
		}
	}
	switch s.Compare {
	case "", CompareExact, CompareUnordered, CompareSet, CompareFloat:
	default:
		return fmt.Errorf("codegen: unknown compare mode %q", s.Compare)
	}
	return nil
}

package sandbox

import (
	"strings"

	"github.com/knovate211/execution-service/internal/codegen"
)

// ExecutionSpec is the sandbox-side view of a problem's execution contract.
type ExecutionSpec struct {
	IoMode     string
	EntryPoint string
	Params     []codegen.Param
	ReturnType codegen.Type
	Compare    string
	FloatEps   float64

	// Kind is "function" or "class"; Methods is populated only for "class".
	Kind    string
	Methods []codegen.Method
}

// Signature kinds.
const (
	KindFunction = "function"
	KindClass    = "class"
)

// Execution modes.
const (
	IoModeFunction = "function"
	IoModeStdio    = "stdio"
	IoModeSQL      = "sql"
)

// Signature converts the spec into the form the generators take.
func (s *ExecutionSpec) Signature() codegen.Signature {
	return codegen.Signature{
		EntryPoint: s.EntryPoint,
		Params:     s.Params,
		ReturnType: s.ReturnType,
		Compare:    codegen.Compare(s.Compare),
		Eps:        s.FloatEps,
	}
}

// ClassSignature converts the spec into the class-mode generator's form.
func (s *ExecutionSpec) ClassSignature() codegen.ClassSignature {
	return codegen.ClassSignature{
		ClassName: s.EntryPoint,
		Ctor:      s.Params,
		Methods:   s.Methods,
	}
}

// wrapForSpec decides how a submission becomes a runnable program.
//
// Three shapes, and the mode says which rather than the code being sniffed for
// clues:
//
//   - stdio and sql submissions are complete programs and run verbatim. The Go
//     basics problems are all stdio; generating a main() for them produced a
//     second main and a duplicated import block, so every one of them failed to
//     compile regardless of what the learner wrote.
//   - function submissions with a declared signature go through codegen.
//   - anything without a spec keeps the legacy slug-keyed behaviour, so
//     problems that have not been migrated yet are unaffected.
func wrapForSpec(spec *ExecutionSpec, problemID, language, code string) (string, error) {
	if spec == nil {
		return wrapUserCode(problemID, language, code), nil
	}

	switch strings.ToLower(spec.IoMode) {
	case IoModeStdio, IoModeSQL:
		return code, nil
	case IoModeFunction:
		if spec.EntryPoint == "" {
			// Declared function-mode but no signature recorded yet.
			return wrapUserCode(problemID, language, code), nil
		}
		if spec.Kind == KindClass {
			return codegen.WrapClass(language, spec.ClassSignature(), code)
		}
		return codegen.Wrap(language, spec.Signature(), code)
	}
	return wrapUserCode(problemID, language, code), nil
}

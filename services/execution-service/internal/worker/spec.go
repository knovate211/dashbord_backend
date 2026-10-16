package worker

import (
	"github.com/knovate211/execution-service/internal/codegen"
	"github.com/knovate211/execution-service/internal/sandbox"
	problemv1 "github.com/knovate211/proto/problem/v1"
)

// toSandboxSpec converts the wire form of a problem's execution contract into
// the sandbox's. The submit path must apply exactly the same contract as the
// run path, or a learner sees one verdict from Run and a different one from
// Submit on identical code.
func toSandboxSpec(in *problemv1.ExecutionSpec) *sandbox.ExecutionSpec {
	if in == nil {
		return nil
	}
	out := &sandbox.ExecutionSpec{
		IoMode:     in.IoMode,
		EntryPoint: in.EntryPoint,
		ReturnType: codegen.Type(in.ReturnType),
		Compare:    in.Compare,
		FloatEps:   in.FloatEps,
		Kind:       in.Kind,
	}
	for _, p := range in.Params {
		out.Params = append(out.Params, codegen.Param{Name: p.Name, Type: codegen.Type(p.Type)})
	}
	for _, m := range in.Methods {
		method := codegen.Method{Name: m.Name, ReturnType: codegen.Type(m.ReturnType)}
		for _, mp := range m.Params {
			method.Params = append(method.Params, codegen.Param{Name: mp.Name, Type: codegen.Type(mp.Type)})
		}
		out.Methods = append(out.Methods, method)
	}
	return out
}

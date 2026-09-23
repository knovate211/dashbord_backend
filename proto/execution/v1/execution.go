// Package executionv1 contains the Execution service types and gRPC service definitions.
package executionv1

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ─── Message Types ────────────────────────────────────────────────────────────

// RunCodeRequest is used for the "Run" button — executes against visible test cases only.
type RunCodeRequest struct {
	ProblemId string `json:"problem_id"`
	Language  string `json:"language"` // python | javascript | java | cpp | go
	Code      string `json:"code"`
	UserId    string `json:"user_id"`
}

// TestResult holds the outcome for a single test case.
type TestResult struct {
	TestCaseId     string `json:"test_case_id"`
	Input          string `json:"input"`
	ExpectedOutput string `json:"expected_output"`
	ActualOutput   string `json:"actual_output"`
	// Status: Accepted | WrongAnswer | TimeLimitExceeded | MemoryLimitExceeded | RuntimeError | CompileError
	Status      string `json:"status"`
	ExecutionMs int64  `json:"execution_ms"`
	MemoryKb    int64  `json:"memory_kb"`
	Error       string `json:"error,omitempty"`
}

type RunCodeResponse struct {
	JobId         string        `json:"job_id"`
	OverallStatus string        `json:"overall_status"`
	TestResults   []*TestResult `json:"test_results"`
	CompileError  string        `json:"compile_error,omitempty"`
	Runtime       int64         `json:"runtime_ms"`
	Memory        int64         `json:"memory_kb"`
}

// RunScratchpadRequest executes code verbatim against supplied stdin, with no
// test cases and no grading. It backs the in-lesson code editor used by module
// assignments, where the learner writes a complete program rather than filling
// in a graded function body.
type RunScratchpadRequest struct {
	Language string `json:"language"` // python | javascript | java | cpp | go | sql
	Code     string `json:"code"`
	Stdin    string `json:"stdin"`
	UserId   string `json:"user_id"`
}

type RunScratchpadResponse struct {
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
	ExitCode    int64  `json:"exit_code"`
	ExecutionMs int64  `json:"execution_ms"`
	TimedOut    bool   `json:"timed_out"`
}

// SubmitCodeRequest is used for the "Submit" button — executes against ALL test cases.
type SubmitCodeRequest struct {
	SubmissionId string `json:"submission_id"`
	ProblemId    string `json:"problem_id"`
	Language     string `json:"language"`
	Code         string `json:"code"`
	UserId       string `json:"user_id"`
}

type SubmitCodeResponse struct {
	JobId string `json:"job_id"`
}

// ExecutionResult is the async result published to NATS after SubmitCode.
type ExecutionResult struct {
	SubmissionId  string        `json:"submission_id"`
	UserId        string        `json:"user_id"`
	ProblemId     string        `json:"problem_id"`
	OverallStatus string        `json:"overall_status"`
	TestResults   []*TestResult `json:"test_results"`
	CompileError  string        `json:"compile_error,omitempty"`
	Runtime       int64         `json:"runtime_ms"`
	Memory        int64         `json:"memory_kb"`
}

// GenerateStartersRequest describes a function-mode signature. The execution
// service owns the code generator, so the admin authoring screen asks it for
// the starter templates rather than duplicating the generator.
type GenerateStartersRequest struct {
	EntryPoint string          `json:"entry_point"`
	Params     []*StarterParam `json:"params"`
	ReturnType string          `json:"return_type"`
}

type StarterParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// GenerateStartersResponse maps language → starter. Error is set (and
// Starters empty) when the signature cannot be generated, e.g. an unknown type.
type GenerateStartersResponse struct {
	Starters map[string]string `json:"starters"`
	Error    string            `json:"error,omitempty"`
}

// VerifySolutionRequest runs code against ALL of a problem's test cases,
// hidden ones included, and returns per-case results synchronously. It is for
// problem authors checking a reference solution — never exposed to learners,
// because it reveals hidden test data.
type VerifySolutionRequest struct {
	ProblemId string `json:"problem_id"`
	Language  string `json:"language"`
	Code      string `json:"code"`
}

// ─── Server Interface ─────────────────────────────────────────────────────────

type ExecutionServiceServer interface {
	RunCode(context.Context, *RunCodeRequest) (*RunCodeResponse, error)
	RunScratchpad(context.Context, *RunScratchpadRequest) (*RunScratchpadResponse, error)
	SubmitCode(context.Context, *SubmitCodeRequest) (*SubmitCodeResponse, error)
	GenerateStarters(context.Context, *GenerateStartersRequest) (*GenerateStartersResponse, error)
	VerifySolution(context.Context, *VerifySolutionRequest) (*RunCodeResponse, error)
}

type UnimplementedExecutionServiceServer struct{}

func (UnimplementedExecutionServiceServer) GenerateStarters(context.Context, *GenerateStartersRequest) (*GenerateStartersResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GenerateStarters not implemented")
}
func (UnimplementedExecutionServiceServer) VerifySolution(context.Context, *VerifySolutionRequest) (*RunCodeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method VerifySolution not implemented")
}

func (UnimplementedExecutionServiceServer) RunCode(context.Context, *RunCodeRequest) (*RunCodeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RunCode not implemented")
}
func (UnimplementedExecutionServiceServer) RunScratchpad(context.Context, *RunScratchpadRequest) (*RunScratchpadResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method RunScratchpad not implemented")
}
func (UnimplementedExecutionServiceServer) SubmitCode(context.Context, *SubmitCodeRequest) (*SubmitCodeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SubmitCode not implemented")
}

// ─── Client Interface & Implementation ───────────────────────────────────────

type ExecutionServiceClient interface {
	RunCode(ctx context.Context, in *RunCodeRequest, opts ...grpc.CallOption) (*RunCodeResponse, error)
	RunScratchpad(ctx context.Context, in *RunScratchpadRequest, opts ...grpc.CallOption) (*RunScratchpadResponse, error)
	SubmitCode(ctx context.Context, in *SubmitCodeRequest, opts ...grpc.CallOption) (*SubmitCodeResponse, error)
	GenerateStarters(ctx context.Context, in *GenerateStartersRequest, opts ...grpc.CallOption) (*GenerateStartersResponse, error)
	VerifySolution(ctx context.Context, in *VerifySolutionRequest, opts ...grpc.CallOption) (*RunCodeResponse, error)
}

type executionServiceClient struct{ cc grpc.ClientConnInterface }

func NewExecutionServiceClient(cc grpc.ClientConnInterface) ExecutionServiceClient {
	return &executionServiceClient{cc}
}

func (c *executionServiceClient) RunCode(ctx context.Context, in *RunCodeRequest, opts ...grpc.CallOption) (*RunCodeResponse, error) {
	out := new(RunCodeResponse)
	if err := c.cc.Invoke(ctx, "/execution.v1.ExecutionService/RunCode", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *executionServiceClient) RunScratchpad(ctx context.Context, in *RunScratchpadRequest, opts ...grpc.CallOption) (*RunScratchpadResponse, error) {
	out := new(RunScratchpadResponse)
	if err := c.cc.Invoke(ctx, "/execution.v1.ExecutionService/RunScratchpad", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *executionServiceClient) SubmitCode(ctx context.Context, in *SubmitCodeRequest, opts ...grpc.CallOption) (*SubmitCodeResponse, error) {
	out := new(SubmitCodeResponse)
	if err := c.cc.Invoke(ctx, "/execution.v1.ExecutionService/SubmitCode", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *executionServiceClient) GenerateStarters(ctx context.Context, in *GenerateStartersRequest, opts ...grpc.CallOption) (*GenerateStartersResponse, error) {
	out := new(GenerateStartersResponse)
	if err := c.cc.Invoke(ctx, "/execution.v1.ExecutionService/GenerateStarters", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *executionServiceClient) VerifySolution(ctx context.Context, in *VerifySolutionRequest, opts ...grpc.CallOption) (*RunCodeResponse, error) {
	out := new(RunCodeResponse)
	if err := c.cc.Invoke(ctx, "/execution.v1.ExecutionService/VerifySolution", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

// ─── Service Registration & Descriptor ───────────────────────────────────────

func RegisterExecutionServiceServer(s grpc.ServiceRegistrar, srv ExecutionServiceServer) {
	s.RegisterService(&ExecutionService_ServiceDesc, srv)
}

var ExecutionService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "execution.v1.ExecutionService",
	HandlerType: (*ExecutionServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "RunCode", Handler: _ExecutionService_RunCode_Handler},
		{MethodName: "RunScratchpad", Handler: _ExecutionService_RunScratchpad_Handler},
		{MethodName: "SubmitCode", Handler: _ExecutionService_SubmitCode_Handler},
		{MethodName: "GenerateStarters", Handler: _ExecutionService_GenerateStarters_Handler},
		{MethodName: "VerifySolution", Handler: _ExecutionService_VerifySolution_Handler},
	},
	Streams: []grpc.StreamDesc{},
}

func _ExecutionService_RunCode_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RunCodeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExecutionServiceServer).RunCode(ctx, in)
	}
	return interceptor(ctx, in, &grpc.UnaryServerInfo{Server: srv, FullMethod: "/execution.v1.ExecutionService/RunCode"},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return srv.(ExecutionServiceServer).RunCode(ctx, req.(*RunCodeRequest))
		})
}

func _ExecutionService_RunScratchpad_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RunScratchpadRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExecutionServiceServer).RunScratchpad(ctx, in)
	}
	return interceptor(ctx, in, &grpc.UnaryServerInfo{Server: srv, FullMethod: "/execution.v1.ExecutionService/RunScratchpad"},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return srv.(ExecutionServiceServer).RunScratchpad(ctx, req.(*RunScratchpadRequest))
		})
}

func _ExecutionService_SubmitCode_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SubmitCodeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExecutionServiceServer).SubmitCode(ctx, in)
	}
	return interceptor(ctx, in, &grpc.UnaryServerInfo{Server: srv, FullMethod: "/execution.v1.ExecutionService/SubmitCode"},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return srv.(ExecutionServiceServer).SubmitCode(ctx, req.(*SubmitCodeRequest))
		})
}

func _ExecutionService_GenerateStarters_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GenerateStartersRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExecutionServiceServer).GenerateStarters(ctx, in)
	}
	return interceptor(ctx, in, &grpc.UnaryServerInfo{Server: srv, FullMethod: "/execution.v1.ExecutionService/GenerateStarters"},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return srv.(ExecutionServiceServer).GenerateStarters(ctx, req.(*GenerateStartersRequest))
		})
}

func _ExecutionService_VerifySolution_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(VerifySolutionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExecutionServiceServer).VerifySolution(ctx, in)
	}
	return interceptor(ctx, in, &grpc.UnaryServerInfo{Server: srv, FullMethod: "/execution.v1.ExecutionService/VerifySolution"},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return srv.(ExecutionServiceServer).VerifySolution(ctx, req.(*VerifySolutionRequest))
		})
}

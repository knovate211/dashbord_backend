// Package submissionv1 contains the Submission service types and gRPC service definitions.
package submissionv1

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ─── Message Types ────────────────────────────────────────────────────────────

type SubmitRequest struct {
	ProblemId string `json:"problem_id"`
	Language  string `json:"language"`
	Code      string `json:"code"`
	UserId    string `json:"user_id"`
	// Source is "assessment" when a timed test submits on the candidate's
	// behalf. Such submissions are scored by the test, not by practice XP.
	Source string `json:"source,omitempty"`
}

type SubmitResponse struct {
	SubmissionId string `json:"submission_id"`
}

type GetSubmissionRequest struct {
	Id string `json:"id"`
}

type TestResult struct {
	TestCaseId     string `json:"test_case_id"`
	Input          string `json:"input"`
	ExpectedOutput string `json:"expected_output"`
	ActualOutput   string `json:"actual_output"`
	Status         string `json:"status"`
	ExecutionMs    int64  `json:"execution_ms"`
	MemoryKb       int64  `json:"memory_kb"`
	Error          string `json:"error,omitempty"`
	IsHidden       bool   `json:"is_hidden,omitempty"`
}

type Submission struct {
	Id           string        `json:"id"`
	UserId       string        `json:"user_id"`
	ProblemId    string        `json:"problem_id"`
	Language     string        `json:"language"`
	Code         string        `json:"code"`
	// Status: Pending | Running | Accepted | WrongAnswer | TimeLimitExceeded | MemoryLimitExceeded | RuntimeError | CompileError
	Status       string        `json:"status"`
	RuntimeMs    int64         `json:"runtime_ms"`
	MemoryKb     int64         `json:"memory_kb"`
	TestResults  []*TestResult `json:"test_results"`
	CompileError string        `json:"compile_error,omitempty"`
	SubmittedAt  string        `json:"submitted_at"`
	CompletedAt  string        `json:"completed_at,omitempty"`
}

type ListSubmissionsRequest struct {
	UserId    string `json:"user_id"`
	ProblemId string `json:"problem_id,omitempty"`
	Page      int32  `json:"page"`
	PageSize  int32  `json:"page_size"`
}

type ListSubmissionsResponse struct {
	Submissions []*Submission `json:"submissions"`
	Total       int32         `json:"total"`
	Page        int32         `json:"page"`
	PageSize    int32         `json:"page_size"`
}

// ─── Server Interface ─────────────────────────────────────────────────────────

type SubmissionServiceServer interface {
	Submit(context.Context, *SubmitRequest) (*SubmitResponse, error)
	GetSubmission(context.Context, *GetSubmissionRequest) (*Submission, error)
	ListSubmissions(context.Context, *ListSubmissionsRequest) (*ListSubmissionsResponse, error)
}

type UnimplementedSubmissionServiceServer struct{}

func (UnimplementedSubmissionServiceServer) Submit(context.Context, *SubmitRequest) (*SubmitResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Submit not implemented")
}
func (UnimplementedSubmissionServiceServer) GetSubmission(context.Context, *GetSubmissionRequest) (*Submission, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetSubmission not implemented")
}
func (UnimplementedSubmissionServiceServer) ListSubmissions(context.Context, *ListSubmissionsRequest) (*ListSubmissionsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListSubmissions not implemented")
}

// ─── Client Interface & Implementation ───────────────────────────────────────

type SubmissionServiceClient interface {
	Submit(ctx context.Context, in *SubmitRequest, opts ...grpc.CallOption) (*SubmitResponse, error)
	GetSubmission(ctx context.Context, in *GetSubmissionRequest, opts ...grpc.CallOption) (*Submission, error)
	ListSubmissions(ctx context.Context, in *ListSubmissionsRequest, opts ...grpc.CallOption) (*ListSubmissionsResponse, error)
}

type submissionServiceClient struct{ cc grpc.ClientConnInterface }

func NewSubmissionServiceClient(cc grpc.ClientConnInterface) SubmissionServiceClient {
	return &submissionServiceClient{cc}
}

func (c *submissionServiceClient) Submit(ctx context.Context, in *SubmitRequest, opts ...grpc.CallOption) (*SubmitResponse, error) {
	out := new(SubmitResponse)
	if err := c.cc.Invoke(ctx, "/submission.v1.SubmissionService/Submit", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *submissionServiceClient) GetSubmission(ctx context.Context, in *GetSubmissionRequest, opts ...grpc.CallOption) (*Submission, error) {
	out := new(Submission)
	if err := c.cc.Invoke(ctx, "/submission.v1.SubmissionService/GetSubmission", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *submissionServiceClient) ListSubmissions(ctx context.Context, in *ListSubmissionsRequest, opts ...grpc.CallOption) (*ListSubmissionsResponse, error) {
	out := new(ListSubmissionsResponse)
	if err := c.cc.Invoke(ctx, "/submission.v1.SubmissionService/ListSubmissions", in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

// ─── Service Registration & Descriptor ───────────────────────────────────────

func RegisterSubmissionServiceServer(s grpc.ServiceRegistrar, srv SubmissionServiceServer) {
	s.RegisterService(&SubmissionService_ServiceDesc, srv)
}

var SubmissionService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "submission.v1.SubmissionService",
	HandlerType: (*SubmissionServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "Submit", Handler: _SubmissionService_Submit_Handler},
		{MethodName: "GetSubmission", Handler: _SubmissionService_GetSubmission_Handler},
		{MethodName: "ListSubmissions", Handler: _SubmissionService_ListSubmissions_Handler},
	},
	Streams: []grpc.StreamDesc{},
}

func _SubmissionService_Submit_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SubmitRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SubmissionServiceServer).Submit(ctx, in)
	}
	return interceptor(ctx, in, &grpc.UnaryServerInfo{Server: srv, FullMethod: "/submission.v1.SubmissionService/Submit"},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return srv.(SubmissionServiceServer).Submit(ctx, req.(*SubmitRequest))
		})
}

func _SubmissionService_GetSubmission_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetSubmissionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SubmissionServiceServer).GetSubmission(ctx, in)
	}
	return interceptor(ctx, in, &grpc.UnaryServerInfo{Server: srv, FullMethod: "/submission.v1.SubmissionService/GetSubmission"},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return srv.(SubmissionServiceServer).GetSubmission(ctx, req.(*GetSubmissionRequest))
		})
}

func _SubmissionService_ListSubmissions_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListSubmissionsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(SubmissionServiceServer).ListSubmissions(ctx, in)
	}
	return interceptor(ctx, in, &grpc.UnaryServerInfo{Server: srv, FullMethod: "/submission.v1.SubmissionService/ListSubmissions"},
		func(ctx context.Context, req interface{}) (interface{}, error) {
			return srv.(SubmissionServiceServer).ListSubmissions(ctx, req.(*ListSubmissionsRequest))
		})
}

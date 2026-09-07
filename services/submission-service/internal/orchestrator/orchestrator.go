// Package orchestrator coordinates the submission lifecycle:
// Submit → persist → publish to NATS → await result → update → publish graded event.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	executionv1 "github.com/knovate211/proto/execution/v1"
	progressv1 "github.com/knovate211/proto/progress/v1"
	submissionv1 "github.com/knovate211/proto/submission/v1"
	"github.com/knovate211/submission-service/internal/repository"
)

const (
	subjectExecutionRun     = "execution.run"
	subjectExecutionResult  = "execution.result"
	subjectSubmissionGraded = "submission.graded"
)

// GradedEvent is published to submission.graded after a submission is evaluated.
type GradedEvent struct {
	SubmissionId  string `json:"submission_id"`
	UserId        string `json:"user_id"`
	ProblemId     string `json:"problem_id"`
	OverallStatus string `json:"overall_status"`
	RuntimeMs     int64  `json:"runtime_ms"`
	MemoryKb      int64  `json:"memory_kb"`
	PassedCount   int    `json:"passed_count"`
	TotalCount    int    `json:"total_count"`
}

// Orchestrator manages submission lifecycle.
type Orchestrator struct {
	repo        *repository.SubmissionRepository
	js          nats.JetStreamContext
	progressCli progressv1.ProgressServiceClient
	log         *zap.Logger
}

// New constructs an Orchestrator.
func New(repo *repository.SubmissionRepository, nc *nats.Conn, progressCli progressv1.ProgressServiceClient, log *zap.Logger) (*Orchestrator, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("jetstream context: %w", err)
	}

	o := &Orchestrator{repo: repo, js: js, progressCli: progressCli, log: log}

	// Start listening for execution results
	go o.listenForResults()

	return o, nil
}

// Submit creates a submission record and enqueues it for execution.
func (o *Orchestrator) Submit(ctx context.Context, req *submissionv1.SubmitRequest) (string, error) {
	// Persist with Pending status
	submissionId, err := o.repo.CreateSubmission(ctx, req)
	if err != nil {
		return "", fmt.Errorf("create submission: %w", err)
	}

	// Enqueue the execution job
	execReq := &executionv1.SubmitCodeRequest{
		SubmissionId: submissionId,
		ProblemId:    req.ProblemId,
		Language:     req.Language,
		Code:         req.Code,
		UserId:       req.UserId,
	}

	data, err := json.Marshal(execReq)
	if err != nil {
		return "", fmt.Errorf("marshal exec request: %w", err)
	}

	if _, err := o.js.Publish(subjectExecutionRun, data); err != nil {
		// Mark as failed if we can't enqueue
		_ = o.repo.UpdateSubmissionResult(ctx, submissionId, "RuntimeError", 0, 0, "failed to enqueue job", nil)
		return "", fmt.Errorf("publish execution job: %w", err)
	}

	o.log.Info("submission enqueued",
		zap.String("submission_id", submissionId),
		zap.String("problem_id", req.ProblemId),
		zap.String("language", req.Language),
	)

	return submissionId, nil
}

// listenForResults subscribes to execution.result and updates submission records.
func (o *Orchestrator) listenForResults() {
	sub, err := o.js.Subscribe(subjectExecutionResult, func(msg *nats.Msg) {
		var result executionv1.ExecutionResult
		if err := json.Unmarshal(msg.Data, &result); err != nil {
			o.log.Error("unmarshal execution result", zap.Error(err))
			msg.Nak() //nolint:errcheck
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Convert execution test results to submission test results
		var trResults []*submissionv1.TestResult
		passedCount := 0
		for _, tr := range result.TestResults {
			subTR := &submissionv1.TestResult{
				TestCaseId:     tr.TestCaseId,
				Input:          tr.Input,
				ExpectedOutput: tr.ExpectedOutput,
				ActualOutput:   tr.ActualOutput,
				Status:         tr.Status,
				ExecutionMs:    tr.ExecutionMs,
				MemoryKb:       tr.MemoryKb,
				Error:          tr.Error,
			}
			trResults = append(trResults, subTR)
			if tr.Status == "Accepted" {
				passedCount++
			}
		}

		// Update submission record
		if err := o.repo.UpdateSubmissionResult(
			ctx,
			result.SubmissionId,
			result.OverallStatus,
			result.Runtime,
			result.Memory,
			result.CompileError,
			trResults,
		); err != nil {
			o.log.Error("update submission result", zap.Error(err))
			msg.Nak() //nolint:errcheck
			return
		}

		// Fetch submission details to get the language
		var language string
		if subRec, err := o.repo.GetSubmission(ctx, result.SubmissionId); err == nil && subRec != nil {
			language = subRec.Language
		} else {
			o.log.Warn("could not load submission details for language info", zap.Error(err))
		}

		// Update progress service
		if o.progressCli != nil && result.OverallStatus == "Accepted" {
			_, err := o.progressCli.UpdateProblemStatus(ctx, &progressv1.UpdateProblemStatusRequest{
				UserId:    result.UserId,
				ProblemId: result.ProblemId,
				Status:    "Solved",
				IsCorrect: true,
				RuntimeMs: result.Runtime,
				MemoryKb:  result.Memory,
				Language:  language,
			})
			if err != nil {
				o.log.Warn("update progress failed", zap.Error(err))
			}
		} else if o.progressCli != nil {
			_, err := o.progressCli.UpdateProblemStatus(ctx, &progressv1.UpdateProblemStatusRequest{
				UserId:    result.UserId,
				ProblemId: result.ProblemId,
				Status:    "InProgress",
				IsCorrect: false,
				Language:  language,
			})
			if err != nil {
				o.log.Warn("update progress (in-progress) failed", zap.Error(err))
			}
		}

		// Publish graded event for the notification service
		event := &GradedEvent{
			SubmissionId:  result.SubmissionId,
			UserId:        result.UserId,
			ProblemId:     result.ProblemId,
			OverallStatus: result.OverallStatus,
			RuntimeMs:     result.Runtime,
			MemoryKb:      result.Memory,
			PassedCount:   passedCount,
			TotalCount:    len(trResults),
		}
		eventData, _ := json.Marshal(event)
		if _, err := o.js.Publish(subjectSubmissionGraded, eventData); err != nil {
			o.log.Warn("publish graded event failed", zap.Error(err))
		}

		msg.Ack() //nolint:errcheck
		o.log.Info("submission graded",
			zap.String("submission_id", result.SubmissionId),
			zap.String("status", result.OverallStatus),
		)
	},
		nats.Durable("submission-result-consumer"),
		nats.ManualAck(),
		nats.AckWait(5*time.Minute),
	)

	if err != nil {
		o.log.Error("subscribe execution.result failed", zap.Error(err))
		return
	}
	defer sub.Unsubscribe() //nolint:errcheck

	// Block forever — goroutine lives for service lifetime
	select {}
}

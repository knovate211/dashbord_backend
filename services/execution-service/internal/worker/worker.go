// Package worker subscribes to NATS JetStream and processes async submission executions.
package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/knovate211/execution-service/internal/judge"
	"github.com/knovate211/execution-service/internal/sandbox"
	executionv1 "github.com/knovate211/proto/execution/v1"
	problemv1 "github.com/knovate211/proto/problem/v1"
)

const (
	streamName    = "EXECUTION"
	subjectRun    = "execution.run"
	subjectResult = "execution.result"
	consumerName  = "execution-worker"

	// ackWait is how long NATS waits for an ack before redelivering a job.
	// keepAliveEvery must stay well under it: every job, waiting or running,
	// reports progress at that interval so a backlog is never redelivered and
	// run twice just because it queued behind other submissions.
	ackWait        = 5 * time.Minute
	keepAliveEvery = time.Minute
)

// ProblemClient is the interface the worker uses to fetch test cases.
type ProblemClient interface {
	GetTestCases(ctx context.Context, in *problemv1.GetTestCasesRequest, opts ...interface{}) (*problemv1.GetTestCasesResponse, error)
}

// Worker consumes execution jobs from NATS JetStream.
type Worker struct {
	js      nats.JetStreamContext
	sb      *sandbox.DockerSandbox
	judge   *judge.Judge
	probCli problemv1.ProblemServiceClient
	log     *zap.Logger
	// jobs bounds how many submissions are graded at once.
	jobs chan struct{}
}

// New creates and initialises a Worker, setting up the JetStream stream if needed.
func New(nc *nats.Conn, sb *sandbox.DockerSandbox, j *judge.Judge, probCli problemv1.ProblemServiceClient, log *zap.Logger) (*Worker, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("jetstream context: %w", err)
	}

	// Create or get the stream
	if _, err := js.AddStream(&nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{"execution.>"},
		Retention: nats.WorkQueuePolicy,
		MaxAge:    24 * time.Hour,
		Storage:   nats.FileStorage,
		Replicas:  1,
	}); err != nil {
		// ErrStreamNameAlreadyInUse means it exists — that's fine
		log.Info("stream already exists or created", zap.Error(err))
	}

	n := workerConcurrency(sb)
	log.Info("execution worker configured", zap.Int("concurrent_jobs", n))
	return &Worker{js: js, sb: sb, judge: j, probCli: probCli, log: log, jobs: make(chan struct{}, n)}, nil
}

// workerConcurrency is how many submissions are graded in parallel. It defaults
// to the sandbox's container limit: each job runs its cases one after another,
// so one job per container slot keeps every slot busy without piling jobs up
// behind the slot queue, where their time limits would start running out.
// EXEC_WORKER_CONCURRENCY overrides it.
func workerConcurrency(sb *sandbox.DockerSandbox) int {
	if v := os.Getenv("EXEC_WORKER_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return sb.Capacity()
}

// Start begins consuming from execution.run. Blocks until ctx is cancelled.
func (w *Worker) Start(ctx context.Context) error {
	// The callback only hands the job off. A NATS subscription delivers one
	// message at a time, so grading inside it would grade one submission at a
	// time however many container slots are free.
	sub, err := w.js.QueueSubscribe(subjectRun, consumerName,
		func(msg *nats.Msg) { go w.dispatch(ctx, msg) },
		nats.Durable(consumerName),
		nats.ManualAck(),
		nats.AckWait(ackWait),
		nats.MaxDeliver(3),
	)
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", subjectRun, err)
	}
	defer sub.Unsubscribe() //nolint:errcheck

	w.log.Info("execution worker started", zap.String("subject", subjectRun))
	<-ctx.Done()
	w.log.Info("execution worker stopping")
	return nil
}

// dispatch waits for a free job slot, then grades the job. The job reports
// progress to NATS the whole time, so neither the wait nor a long run is
// mistaken for a dead worker.
func (w *Worker) dispatch(ctx context.Context, msg *nats.Msg) {
	stop := keepAlive(msg)
	defer stop()

	select {
	case w.jobs <- struct{}{}:
		defer func() { <-w.jobs }()
	case <-ctx.Done():
		// Shutting down before it started: hand it straight back so the next
		// worker picks it up instead of waiting out the ack deadline.
		msg.Nak() //nolint:errcheck
		return
	}
	w.handleMessage(msg)
}

// keepAlive tells NATS the job is still being worked on until stop is called.
func keepAlive(msg *nats.Msg) (stop func()) {
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(keepAliveEvery)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				msg.InProgress() //nolint:errcheck
			case <-done:
				return
			}
		}
	}()
	return func() { close(done) }
}

// handleMessage processes a single execution job.
func (w *Worker) handleMessage(msg *nats.Msg) {
	var req executionv1.SubmitCodeRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		w.log.Error("unmarshal execution request", zap.Error(err))
		msg.Nak() //nolint:errcheck
		return
	}

	w.log.Info("processing execution job",
		zap.String("submission_id", req.SubmissionId),
		zap.String("language", req.Language),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	result := w.execute(ctx, &req)

	// Publish result to execution.result
	data, err := json.Marshal(result)
	if err != nil {
		w.log.Error("marshal execution result", zap.Error(err))
		msg.Nak() //nolint:errcheck
		return
	}

	if _, err := w.js.Publish(subjectResult, data); err != nil {
		w.log.Error("publish execution result", zap.Error(err))
		msg.Nak() //nolint:errcheck
		return
	}

	msg.Ack() //nolint:errcheck

	w.log.Info("execution job complete",
		zap.String("submission_id", req.SubmissionId),
		zap.String("status", result.OverallStatus),
	)
}

// execute runs the code against all test cases (including hidden) and returns the aggregate result.
func (w *Worker) execute(ctx context.Context, req *executionv1.SubmitCodeRequest) *executionv1.ExecutionResult {
	result := &executionv1.ExecutionResult{
		SubmissionId: req.SubmissionId,
		UserId:       req.UserId,
		ProblemId:    req.ProblemId,
		Source:       req.Source,
	}

	// Fetch all test cases (including hidden — this is a Submit operation)
	tcResp, err := w.probCli.GetTestCases(ctx, &problemv1.GetTestCasesRequest{
		ProblemId:     req.ProblemId,
		IncludeHidden: true,
	})
	if err != nil {
		result.OverallStatus = "RuntimeError"
		result.CompileError = fmt.Sprintf("failed to fetch test cases: %v", err)
		return result
	}
	result.TotalCases = len(tcResp.TestCases)
	// With nothing to check against, every submission would be Accepted.
	if len(tcResp.TestCases) == 0 {
		result.OverallStatus = judge.StatusRuntimeError
		result.CompileError = "this problem has no test cases yet"
		return result
	}
	// A timed test awards marks per case passed, so it must run them all.
	// Practice only needs the first failure.
	runAll := req.Source == "assessment"

	var testResults []*executionv1.TestResult
	var maxRuntime int64
	var maxMemory int64

	for _, tc := range tcResp.TestCases {
		sbResult, err := w.sb.Run(ctx, &sandbox.RunRequest{
			ProblemId:     req.ProblemId,
			Language:      req.Language,
			Code:          req.Code,
			Input:         tc.Input,
			TimeLimitMs:   tc.TimeLimitMs,
			MemoryLimitMb: tc.MemoryLimitMb,
			Spec:          toSandboxSpec(tcResp.ExecutionSpec),
		})
		if err != nil {
			w.log.Error("sandbox run failed", zap.Error(err), zap.String("tc_id", tc.Id))
			tr := &executionv1.TestResult{
				TestCaseId: tc.Id,
				Status:     "RuntimeError",
				Error:      err.Error(),
				IsHidden:   tc.IsHidden,
			}
			testResults = append(testResults, tr)
			continue
		}

		tr := w.judge.EvaluateTestCaseWithSpec(tc, sbResult, tcResp.ExecutionSpec)
		testResults = append(testResults, tr)

		if sbResult.ExecutionMs > maxRuntime {
			maxRuntime = sbResult.ExecutionMs
		}
		if sbResult.MemoryKb > maxMemory {
			maxMemory = sbResult.MemoryKb
		}

		// Short-circuit on first non-accepted result for efficiency
		if !runAll && tr.Status != judge.StatusAccepted {
			break
		}
	}

	result.TestResults = testResults
	result.OverallStatus = judge.OverallStatus(testResults)
	result.Runtime = maxRuntime
	result.Memory = maxMemory
	return result
}

// PublishRunJob publishes a SubmitCodeRequest to NATS for async processing.
func (w *Worker) PublishRunJob(req *executionv1.SubmitCodeRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal run job: %w", err)
	}
	_, err = w.js.Publish(subjectRun, data)
	return err
}

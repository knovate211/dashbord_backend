// Package consumer bridges the existing code-grading pipeline into assessment
// scoring. When the judge finishes a submission, submission-service publishes
// submission.graded; this consumer routes that verdict back to the attempt
// question that produced it.
//
// It uses its own durable consumer name so it receives every event
// independently of notification-service, which listens on the same subject.
package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/knovate211/assessment-service/internal/repository"
)

const (
	subjectGraded = "submission.graded"
	durableName   = "assessment-submission-graded"
)

// gradedEvent mirrors the payload published by submission-service.
type gradedEvent struct {
	SubmissionId  string `json:"submission_id"`
	UserId        string `json:"user_id"`
	ProblemId     string `json:"problem_id"`
	OverallStatus string `json:"overall_status"`
	PassedCount   int    `json:"passed_count"`
	TotalCount    int    `json:"total_count"`
	Source        string `json:"source"`
}

// maxLinkRetries bounds how long a test verdict waits for its attempt link.
// RecordCodeSubmission runs right after Submit returns, so a few seconds is
// plenty; the bound only stops a genuinely orphaned event looping forever.
const maxLinkRetries = 10

// Consumer subscribes to graded submissions.
type Consumer struct {
	js   nats.JetStreamContext
	repo *repository.Repo
	log  *zap.Logger
	sub  *nats.Subscription
}

// Start wires the subscription and begins consuming.
func Start(nc *nats.Conn, repo *repository.Repo, log *zap.Logger) (*Consumer, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("jetstream context: %w", err)
	}

	// Idempotent — submission-service and notification-service create the same
	// stream, whichever starts first wins.
	if _, err := js.AddStream(&nats.StreamConfig{
		Name:     "SUBMISSIONS",
		Subjects: []string{"submission.>"},
		MaxAge:   24 * time.Hour,
		Storage:  nats.FileStorage,
		Replicas: 1,
	}); err != nil {
		log.Info("SUBMISSIONS stream already exists or created", zap.Error(err))
	}

	c := &Consumer{js: js, repo: repo, log: log}
	sub, err := js.Subscribe(subjectGraded, c.handle, nats.Durable(durableName), nats.ManualAck())
	if err != nil {
		return nil, fmt.Errorf("subscribe %s: %w", subjectGraded, err)
	}
	c.sub = sub

	log.Info("graded-submission consumer started", zap.String("durable", durableName))
	return c, nil
}

func (c *Consumer) handle(msg *nats.Msg) {
	var ev gradedEvent
	if err := json.Unmarshal(msg.Data, &ev); err != nil {
		// A payload we cannot parse will never parse — acking avoids an
		// infinite redelivery loop on one poison message.
		c.log.Error("unmarshal submission.graded", zap.Error(err))
		msg.Ack() //nolint:errcheck
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := c.repo.ApplyGradedSubmission(ctx, ev.SubmissionId, ev.OverallStatus,
		int32(ev.PassedCount), int32(ev.TotalCount))
	if errors.Is(err, repository.ErrSubmissionNotLinked) {
		// Practice submissions never link to an attempt. A test submission
		// can be graded before its attempt link is written — wait for it.
		if ev.Source == "assessment" {
			if meta, mErr := msg.Metadata(); mErr == nil && meta.NumDelivered < maxLinkRetries {
				msg.NakWithDelay(time.Second) //nolint:errcheck
				return
			}
			c.log.Error("test submission never linked to an attempt",
				zap.String("submission_id", ev.SubmissionId))
		}
		msg.Ack() //nolint:errcheck
		return
	}
	if err != nil {
		// A transient database failure is worth retrying: leave it unacked.
		c.log.Error("apply graded submission",
			zap.String("submission_id", ev.SubmissionId), zap.Error(err))
		msg.Nak() //nolint:errcheck
		return
	}

	msg.Ack() //nolint:errcheck
}

// Stop unsubscribes on shutdown.
func (c *Consumer) Stop() {
	if c.sub != nil {
		_ = c.sub.Drain()
	}
}

// Package subscriber subscribes to NATS events and forwards them to the WebSocket hub.
package subscriber

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/knovate211/notification-service/internal/handler"
	notificationv1 "github.com/knovate211/proto/notification/v1"
)

// NATSSubscriber subscribes to NATS topics and forwards events to the hub.
type NATSSubscriber struct {
	js  nats.JetStreamContext
	hub *handler.Hub
	log *zap.Logger
}

// New creates a NATSSubscriber and sets up the subscriptions.
func New(nc *nats.Conn, hub *handler.Hub, log *zap.Logger) (*NATSSubscriber, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("jetstream context: %w", err)
	}

	// Ensure the SUBMISSIONS stream exists before subscribing.
	// This is idempotent — safe to call even if the stream was already created
	// by another service (e.g., submission-service).
	if _, err := js.AddStream(&nats.StreamConfig{
		Name:     "SUBMISSIONS",
		Subjects: []string{"submission.>"},
		MaxAge:   24 * time.Hour,
		Storage:  nats.FileStorage,
		Replicas: 1,
	}); err != nil {
		// ErrStreamNameAlreadyInUse means it already exists — that's fine.
		log.Info("SUBMISSIONS stream already exists or created", zap.Error(err))
	}

	s := &NATSSubscriber{js: js, hub: hub, log: log}

	if err := s.subscribeSubmissionGraded(); err != nil {
		return nil, fmt.Errorf("subscribe submission.graded: %w", err)
	}

	log.Info("NATS subscriber started")
	return s, nil
}

// subscribeSubmissionGraded listens for graded submission events.
func (s *NATSSubscriber) subscribeSubmissionGraded() error {
	_, err := s.js.Subscribe("submission.graded", func(msg *nats.Msg) {
		var payload notificationv1.SubmissionGradedPayload
		if err := json.Unmarshal(msg.Data, &payload); err != nil {
			s.log.Error("unmarshal submission.graded", zap.Error(err))
			msg.Nak() //nolint:errcheck
			return
		}

		event := &notificationv1.WebSocketEvent{
			Type:    notificationv1.EventSubmissionGraded,
			Payload: payload,
		}

		s.hub.Broadcast(payload.UserId, event)

		s.log.Info("broadcast submission graded",
			zap.String("user_id", payload.UserId),
			zap.String("submission_id", payload.SubmissionId),
			zap.String("status", payload.OverallStatus),
		)

		msg.Ack() //nolint:errcheck
	},
		nats.Durable("notification-submission-graded"),
		nats.ManualAck(),
	)
	return err
}

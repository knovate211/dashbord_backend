package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	pkgdb "github.com/knovate211/pkg/database"
	pkglog "github.com/knovate211/pkg/logger"
	assessmentv1 "github.com/knovate211/proto/assessment/v1"
	"github.com/knovate211/proto/codec" // register JSON codec
	executionv1 "github.com/knovate211/proto/execution/v1"
	submissionv1 "github.com/knovate211/proto/submission/v1"

	"github.com/knovate211/assessment-service/internal/consumer"
	"github.com/knovate211/assessment-service/internal/handler"
	"github.com/knovate211/assessment-service/internal/repository"
	"github.com/knovate211/assessment-service/internal/sweeper"
)

func main() {
	codec.Register()
	cfg := loadConfig()
	log := pkglog.New(cfg.logLevel)
	defer log.Sync() //nolint:errcheck

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── PostgreSQL ───────────────────────────────────────────────────────────
	pool, err := pkgdb.NewPostgresPool(ctx, cfg.postgresDSN)
	if err != nil {
		log.Fatal("connect postgres", zap.Error(err))
	}
	defer pool.Close()
	log.Info("postgres connected")

	repo := repository.New(pool)

	// ── Downstream services ──────────────────────────────────────────────────
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	subConn := mustDial(cfg.submissionServiceAddr, dialOpts, log)
	defer subConn.Close()

	execConn := mustDial(cfg.executionServiceAddr, dialOpts, log)
	defer execConn.Close()

	h := handler.New(repo,
		submissionv1.NewSubmissionServiceClient(subConn),
		executionv1.NewExecutionServiceClient(execConn),
		log)

	// ── Graded-submission consumer ───────────────────────────────────────────
	// Without NATS the service still serves MCQ tests; coding questions simply
	// stay pending, so a broker outage degrades rather than fails.
	nc, err := nats.Connect(cfg.natsURL,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		log.Error("connect nats — coding submissions will not be scored until it recovers", zap.Error(err))
	} else {
		defer nc.Close()
		gradedConsumer, err := consumer.Start(nc, repo, log)
		if err != nil {
			log.Error("start graded consumer", zap.Error(err))
		} else {
			defer gradedConsumer.Stop()
		}
	}

	// ── Expiry sweeper ───────────────────────────────────────────────────────
	go sweeper.Run(ctx, repo, cfg.sweepInterval, log)

	// ── gRPC server ──────────────────────────────────────────────────────────
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.grpcPort))
	if err != nil {
		log.Fatal("listen", zap.Error(err))
	}

	srv := grpc.NewServer()
	assessmentv1.RegisterAssessmentServiceServer(srv, h)
	reflection.Register(srv)

	log.Info("assessment-service starting", zap.String("port", cfg.grpcPort))

	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Error("serve failed", zap.Error(err))
		}
	}()

	// ── Graceful shutdown ────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down assessment-service...")
	cancel()
	srv.GracefulStop()
	log.Info("assessment-service stopped")
}

func mustDial(addr string, opts []grpc.DialOption, log *zap.Logger) *grpc.ClientConn {
	conn, err := grpc.Dial(addr, opts...)
	if err != nil {
		log.Fatal("dial gRPC service", zap.String("addr", addr), zap.Error(err))
	}
	log.Info("gRPC connection established", zap.String("addr", addr))
	return conn
}

type config struct {
	postgresDSN           string
	natsURL               string
	submissionServiceAddr string
	executionServiceAddr  string
	grpcPort              string
	sweepInterval         time.Duration
	logLevel              string
}

func loadConfig() config {
	return config{
		postgresDSN:           env("POSTGRES_DSN", "postgres://skillofide:password@localhost:5432/skillofide?sslmode=disable"),
		natsURL:               env("NATS_URL", "nats://localhost:4222"),
		submissionServiceAddr: env("SUBMISSION_SERVICE_ADDR", "localhost:50053"),
		executionServiceAddr:  env("EXECUTION_SERVICE_ADDR", "localhost:50052"),
		grpcPort:              env("GRPC_PORT", "50056"),
		sweepInterval:         envDuration("SWEEP_INTERVAL_SECONDS", 30*time.Second),
		logLevel:              env("LOG_LEVEL", "info"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs <= 0 {
		return fallback
	}
	return time.Duration(secs) * time.Second
}

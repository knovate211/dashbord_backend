package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	pkgdb "github.com/knovate211/pkg/database"
	pkglog "github.com/knovate211/pkg/logger"
	"github.com/knovate211/proto/codec"
	progressv1 "github.com/knovate211/proto/progress/v1"
	submissionv1 "github.com/knovate211/proto/submission/v1"

	"github.com/knovate211/submission-service/internal/handler"
	"github.com/knovate211/submission-service/internal/orchestrator"
	"github.com/knovate211/submission-service/internal/repository"
)

func main() {
	codec.Register()
	cfg := loadConfig()
	log := pkglog.New(cfg.logLevel)
	defer log.Sync() //nolint:errcheck

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── PostgreSQL ────────────────────────────────────────────────────────────
	pool, err := pkgdb.NewPostgresPool(ctx, cfg.postgresDSN)
	if err != nil {
		log.Fatal("connect postgres", zap.Error(err))
	}
	defer pool.Close()

	// ── NATS ─────────────────────────────────────────────────────────────────
	nc, err := nats.Connect(cfg.natsURL)
	if err != nil {
		log.Fatal("connect nats", zap.Error(err))
	}
	defer nc.Close()

	// ── Progress service gRPC client ─────────────────────────────────────────
	progConn, err := grpc.Dial(cfg.progressServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Warn("dial progress-service failed — progress updates disabled", zap.Error(err))
	}
	var progressCli progressv1.ProgressServiceClient
	if progConn != nil {
		defer progConn.Close()
		progressCli = progressv1.NewProgressServiceClient(progConn)
	}

	// ── Build layers ──────────────────────────────────────────────────────────
	repo := repository.New(pool)

	orch, err := orchestrator.New(repo, nc, progressCli, log)
	if err != nil {
		log.Fatal("init orchestrator", zap.Error(err))
	}

	h := handler.New(orch, repo, log)

	// ── gRPC server ───────────────────────────────────────────────────────────
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.grpcPort))
	if err != nil {
		log.Fatal("listen", zap.Error(err))
	}

	srv := grpc.NewServer()
	submissionv1.RegisterSubmissionServiceServer(srv, h)
	reflection.Register(srv)

	log.Info("submission-service starting", zap.String("port", cfg.grpcPort))

	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Error("grpc serve failed", zap.Error(err))
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down submission-service...")
	cancel()
	srv.GracefulStop()
	log.Info("submission-service stopped")
}

type config struct {
	postgresDSN         string
	natsURL             string
	progressServiceAddr string
	grpcPort            string
	logLevel            string
}

func loadConfig() config {
	return config{
		postgresDSN:         env("POSTGRES_DSN", "postgres://skillofide:password@localhost:5432/skillofide?sslmode=disable"),
		natsURL:             env("NATS_URL", nats.DefaultURL),
		progressServiceAddr: env("PROGRESS_SERVICE_ADDR", "localhost:50054"),
		grpcPort:            env("GRPC_PORT", "50053"),
		logLevel:            env("LOG_LEVEL", "info"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

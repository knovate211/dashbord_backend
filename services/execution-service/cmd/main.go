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

	pkglog "github.com/knovate211/pkg/logger"
	"github.com/knovate211/proto/codec"
	executionv1 "github.com/knovate211/proto/execution/v1"
	problemv1 "github.com/knovate211/proto/problem/v1"

	"github.com/knovate211/execution-service/internal/handler"
	"github.com/knovate211/execution-service/internal/judge"
	"github.com/knovate211/execution-service/internal/sandbox"
	"github.com/knovate211/execution-service/internal/worker"
)

func main() {
	codec.Register()
	cfg := loadConfig()
	log := pkglog.New(cfg.logLevel)
	defer log.Sync() //nolint:errcheck

	// ── Docker sandbox ────────────────────────────────────────────────────────
	sb, err := sandbox.New(log)
	if err != nil {
		log.Fatal("init docker sandbox", zap.Error(err))
	}

	// ── Problem service gRPC client ───────────────────────────────────────────
	probConn, err := grpc.Dial(cfg.problemServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal("dial problem-service", zap.Error(err))
	}
	defer probConn.Close()
	probCli := problemv1.NewProblemServiceClient(probConn)

	// ── NATS ──────────────────────────────────────────────────────────────────
	nc, err := nats.Connect(cfg.natsURL)
	if err != nil {
		log.Fatal("connect nats", zap.Error(err))
	}
	defer nc.Close()
	log.Info("nats connected", zap.String("url", cfg.natsURL))

	// ── Judge + Worker ────────────────────────────────────────────────────────
	j := judge.New()
	w, err := worker.New(nc, sb, j, probCli, log)
	if err != nil {
		log.Fatal("init worker", zap.Error(err))
	}

	// ── gRPC server ───────────────────────────────────────────────────────────
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.grpcPort))
	if err != nil {
		log.Fatal("listen", zap.Error(err))
	}

	h := handler.New(sb, j, w, probCli, log)

	srv := grpc.NewServer()
	executionv1.RegisterExecutionServiceServer(srv, h)
	reflection.Register(srv)

	log.Info("execution-service starting", zap.String("port", cfg.grpcPort))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Worker goroutine
	go func() {
		if err := w.Start(ctx); err != nil {
			log.Error("worker error", zap.Error(err))
		}
	}()

	// gRPC goroutine
	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Error("grpc serve error", zap.Error(err))
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down execution-service...")
	cancel()
	srv.GracefulStop()
	log.Info("execution-service stopped")
}

type config struct {
	grpcPort           string
	problemServiceAddr string
	natsURL            string
	logLevel           string
}

func loadConfig() config {
	return config{
		grpcPort:           env("GRPC_PORT", "50052"),
		problemServiceAddr: env("PROBLEM_SERVICE_ADDR", "localhost:50051"),
		natsURL:            env("NATS_URL", nats.DefaultURL),
		logLevel:           env("LOG_LEVEL", "info"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

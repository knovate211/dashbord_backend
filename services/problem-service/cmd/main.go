package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/knovate211/proto/codec" // register JSON codec
	problemv1 "github.com/knovate211/proto/problem/v1"
	pkgdb "github.com/knovate211/pkg/database"
	pkglog "github.com/knovate211/pkg/logger"

	"github.com/knovate211/problem-service/internal/cache"
	"github.com/knovate211/problem-service/internal/handler"
	"github.com/knovate211/problem-service/internal/repository"
)

func main() {
	codec.Register()
	cfg := loadConfig()
	log := pkglog.New(cfg.logLevel)
	defer log.Sync() //nolint:errcheck

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── PostgreSQL ──────────────────────────────────────────────────────────
	pool, err := pkgdb.NewPostgresPool(ctx, cfg.postgresDSN)
	if err != nil {
		log.Fatal("connect postgres", zap.Error(err))
	}
	defer pool.Close()
	log.Info("postgres connected")

	// ── Redis (optional — graceful degradation) ─────────────────────────────
	redisClient, err := pkgdb.NewRedisClient(cfg.redisAddr, cfg.redisPassword, 0)
	if err != nil {
		log.Warn("redis unavailable — cache disabled", zap.Error(err))
	}

	// ── Build layers ─────────────────────────────────────────────────────────
	repo := repository.New(pool)
	cacheLayer := cache.New(redisClient, log)
	h := handler.New(repo, cacheLayer, log)

	// ── gRPC server ──────────────────────────────────────────────────────────
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.grpcPort))
	if err != nil {
		log.Fatal("listen", zap.Error(err))
	}

	srv := grpc.NewServer()
	problemv1.RegisterProblemServiceServer(srv, h)
	reflection.Register(srv) // for grpcurl / Postman

	log.Info("problem-service starting", zap.String("port", cfg.grpcPort))

	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Error("serve failed", zap.Error(err))
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down problem-service...")
	srv.GracefulStop()
	log.Info("problem-service stopped")
}

type config struct {
	postgresDSN   string
	redisAddr     string
	redisPassword string
	grpcPort      string
	logLevel      string
}

func loadConfig() config {
	return config{
		postgresDSN:   env("POSTGRES_DSN", "postgres://skillofide:password@localhost:5432/skillofide?sslmode=disable"),
		redisAddr:     env("REDIS_ADDR", "localhost:6379"),
		redisPassword: env("REDIS_PASSWORD", ""),
		grpcPort:      env("GRPC_PORT", "50051"),
		logLevel:      env("LOG_LEVEL", "info"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

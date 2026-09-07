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

	"github.com/knovate211/proto/codec"
	progressv1 "github.com/knovate211/proto/progress/v1"
	pkgdb "github.com/knovate211/pkg/database"
	pkglog "github.com/knovate211/pkg/logger"

	"github.com/knovate211/progress-service/internal/cache"
	"github.com/knovate211/progress-service/internal/handler"
	"github.com/knovate211/progress-service/internal/repository"
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

	// ── Redis (optional) ──────────────────────────────────────────────────────
	redisClient, err := pkgdb.NewRedisClient(cfg.redisAddr, cfg.redisPassword, 1)
	if err != nil {
		log.Warn("redis unavailable — progress cache disabled", zap.Error(err))
	}

	// ── Build layers ──────────────────────────────────────────────────────────
	repo := repository.New(pool)
	cacheLayer := cache.New(redisClient)
	h := handler.New(repo, cacheLayer, log)

	// ── gRPC server ───────────────────────────────────────────────────────────
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.grpcPort))
	if err != nil {
		log.Fatal("listen", zap.Error(err))
	}

	srv := grpc.NewServer()
	progressv1.RegisterProgressServiceServer(srv, h)
	reflection.Register(srv)

	log.Info("progress-service starting", zap.String("port", cfg.grpcPort))

	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Error("grpc serve failed", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down progress-service...")
	srv.GracefulStop()
	log.Info("progress-service stopped")
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
		grpcPort:      env("GRPC_PORT", "50054"),
		logLevel:      env("LOG_LEVEL", "info"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

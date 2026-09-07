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

	pkgdb "github.com/knovate211/pkg/database"
	pkglog "github.com/knovate211/pkg/logger"
	"github.com/knovate211/proto/codec" // register JSON codec
	userv1 "github.com/knovate211/proto/user/v1"

	"github.com/knovate211/user-service/internal/handler"
	"github.com/knovate211/user-service/internal/repository"
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

	// ── Build layers ─────────────────────────────────────────────────────────
	repo := repository.New(pool)
	
	// Ensure users table and seed data
	if err := repo.EnsureUsersTable(ctx); err != nil {
		log.Fatal("ensure users table failed", zap.Error(err))
	}
	log.Info("users table verified and seeded successfully")

	// Ensure user_profiles table
	if err := repo.EnsureProfileTable(ctx); err != nil {
		log.Fatal("failed to ensure user_profiles table", zap.Error(err))
	}

	// Ensure user_courses table
	if err := repo.EnsureUserCoursesTable(ctx); err != nil {
		log.Fatal("failed to ensure user_courses table", zap.Error(err))
	}
	log.Info("user_profiles table verified successfully")

	// Ensure quiz tables and seed keys
	if err := repo.EnsureQuizTables(ctx); err != nil {
		log.Fatal("failed to ensure quiz tables", zap.Error(err))
	}
	log.Info("quiz tables verified and seeded successfully")


	h := handler.New(repo, log)

	// ── gRPC server ──────────────────────────────────────────────────────────
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.grpcPort))
	if err != nil {
		log.Fatal("listen", zap.Error(err))
	}

	srv := grpc.NewServer()
	userv1.RegisterUserServiceServer(srv, h)
	reflection.Register(srv) // for grpcurl / Postman

	log.Info("user-service starting", zap.String("port", cfg.grpcPort))

	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Error("serve failed", zap.Error(err))
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down user-service...")
	srv.GracefulStop()
	log.Info("user-service stopped")
}

type config struct {
	postgresDSN string
	grpcPort    string
	logLevel    string
}

func loadConfig() config {
	return config{
		postgresDSN: env("POSTGRES_DSN", "postgres://skillofide:password@localhost:5432/skillofide?sslmode=disable"),
		grpcPort:    env("GRPC_PORT", "50055"),
		logLevel:    env("LOG_LEVEL", "info"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

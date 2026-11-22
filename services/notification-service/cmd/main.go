package main

import (
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/knovate211/notification-service/internal/handler"
	"github.com/knovate211/notification-service/internal/subscriber"
	pkgconfig "github.com/knovate211/pkg/config"
	pkglog "github.com/knovate211/pkg/logger"
)

func main() {
	cfg := loadConfig()
	log := pkglog.New(cfg.logLevel)
	defer log.Sync() //nolint:errcheck

	// ── Hub (WebSocket connections) ────────────────────────────────────────────
	hub := handler.NewHub(log)

	// ── NATS ──────────────────────────────────────────────────────────────────
	nc, err := nats.Connect(cfg.natsURL,
		nats.ReconnectWait(2),
		nats.MaxReconnects(-1), // reconnect forever
	)
	if err != nil {
		log.Fatal("connect nats", zap.Error(err))
	}
	defer nc.Close()

	if _, err := subscriber.New(nc, hub, log); err != nil {
		log.Fatal("init nats subscriber", zap.Error(err))
	}

	// ── HTTP server ───────────────────────────────────────────────────────────
	mux := http.NewServeMux()
	mux.Handle("/ws", hub)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok")) //nolint:errcheck
	})

	server := &http.Server{
		Addr:    ":" + cfg.httpPort,
		Handler: mux,
	}

	log.Info("notification-service starting", zap.String("port", cfg.httpPort))

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("http server error", zap.Error(err))
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down notification-service...")
	log.Info("notification-service stopped")
}

type config struct {
	natsURL  string
	httpPort string
	logLevel string
}

func loadConfig() config {
	return config{
		natsURL:  pkgconfig.Env("NATS_URL", nats.DefaultURL),
		httpPort: pkgconfig.Env("HTTP_PORT", "8081"),
		logLevel: pkgconfig.Env("LOG_LEVEL", "info"),
	}
}

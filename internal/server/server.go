package server

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"github.com/takutakahashi/capps/internal/config"
	"github.com/takutakahashi/capps/internal/gateway"
)

// Server is the capps HTTP server.
type Server struct {
	cfg    *config.Config
	client gateway.GatewayClient
	logger *zap.Logger
	http   *http.Server
}

// New creates a new Server, initialises the gateway client, and registers routes.
func New(cfg *config.Config) (*Server, error) {
	logger, err := buildLogger(cfg.LogLevel)
	if err != nil {
		return nil, err
	}

	client := gateway.NewReconnectingClient(cfg)

	h := &handler{
		client:     client,
		gatewayURL: cfg.GatewayURL,
	}

	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(loggingMiddleware(logger))
	r.Use(recoveryMiddleware(logger))
	r.Use(chimiddleware.Compress(5))

	// Routes
	r.Get("/healthz", h.handleHealthz)
	r.Get("/status", h.handleStatus)

	// Generic gateway call endpoint.
	// The wildcard `{*}` captures everything after /call/ including slashes.
	r.Post("/call/*", h.handleCall)

	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: r,
	}

	return &Server{
		cfg:    cfg,
		client: client,
		logger: logger,
		http:   httpServer,
	}, nil
}

// Start connects to the gateway and begins serving HTTP requests.
// It blocks until the context is cancelled or a fatal error occurs.
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("connecting to gateway", zap.String("url", s.cfg.GatewayURL))
	if err := s.client.Connect(ctx); err != nil {
		return err
	}

	s.logger.Info("starting HTTP server", zap.String("addr", s.cfg.ListenAddr))

	errCh := make(chan error, 1)
	go func() {
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		s.logger.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*1e9) // 10 seconds
		defer cancel()
		_ = s.http.Shutdown(shutdownCtx)
		_ = s.client.Close()
		return nil
	case err := <-errCh:
		return err
	}
}

// buildLogger creates a zap logger configured to the requested level.
func buildLogger(level string) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	if err := cfg.Level.UnmarshalText([]byte(level)); err != nil {
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}
	return cfg.Build()
}

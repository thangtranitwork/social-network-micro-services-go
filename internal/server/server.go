package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"social-network-go/logger"
)

// RunWithGracefulShutdown runs an http.Server and handles SIGINT / SIGTERM for clean shutdown
func RunWithGracefulShutdown(srv *http.Server, shutdownTimeout time.Duration, cleanup ...func()) {
	if shutdownTimeout <= 0 {
		shutdownTimeout = 10 * time.Second
	}

	serverErrors := make(chan error, 1)

	go func() {
		logger.Info("🚀 Server is listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		logger.Err(err).Fatal("Error starting server")

	case sig := <-shutdown:
		logger.Info("Shutdown signal caught (%v), gracefully stopping server (timeout: %v)...", sig, shutdownTimeout)

		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			logger.Err(err).Error("Graceful shutdown timed out, forcing server close")
			_ = srv.Close()
		}

		for _, fn := range cleanup {
			if fn != nil {
				fn()
			}
		}

		logger.Info("Server stopped cleanly")
	}
}

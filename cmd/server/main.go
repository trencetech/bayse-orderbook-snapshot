package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/trencetech/bayse-orderbook-snapshot/internal/collector"
	"github.com/trencetech/bayse-orderbook-snapshot/internal/config"
	"github.com/trencetech/bayse-orderbook-snapshot/internal/database"
	apphttp "github.com/trencetech/bayse-orderbook-snapshot/internal/http"
	"github.com/trencetech/bayse-orderbook-snapshot/internal/logger"
	"github.com/trencetech/bayse-orderbook-snapshot/internal/models"
	"github.com/trencetech/bayse-orderbook-snapshot/internal/repository"
	"github.com/trencetech/bayse-orderbook-snapshot/internal/version"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(fmt.Errorf("failed to load config: %w", err))
	}

	zapLogger, err := logger.New(cfg.Env)
	if err != nil {
		log.Fatal(fmt.Errorf("failed to create logger: %w", err))
	}
	defer zapLogger.Sync()

	zapLogger.Info("starting bayse-orderbook-snapshot",
		zap.String("env", cfg.Env),
		zap.String("commit", version.CommitSHA),
	)

	// Database
	db := database.New(cfg.DSN())
	defer db.Close()

	if err := db.Ping(); err != nil {
		zapLogger.Fatal("failed to connect to database", zap.Error(err))
	}
	zapLogger.Info("connected to database")

	repo := repository.NewSnapshotRepository(db)

	// Snapshot write channel
	writeCh := make(chan models.OrderbookSnapshot, 1000)

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Batch writer — runs until writeCh is closed
	writer := collector.NewBatchWriter(repo, zapLogger, writeCh)
	writerDone := make(chan struct{})
	go func() { writer.Run(ctx); close(writerDone) }()

	// Market discovery
	discovery := collector.NewDiscovery(cfg.BayseRelayURL, cfg.DiscoveryInterval, zapLogger)

	// WebSocket collector
	wsCollector := collector.NewWSCollector(cfg.BayseWSURL, zapLogger, writeCh)
	discovery.OnUpdate(func(markets []models.TrackedMarket) {
		wsCollector.UpdateMarkets(markets)
	})

	// REST poller
	poller := collector.NewPoller(cfg.BayseRelayURL, cfg.PollInterval, zapLogger, writeCh)
	discovery.OnUpdate(func(markets []models.TrackedMarket) {
		poller.UpdateMarkets(markets)
	})

	// Track producers so we can wait for them before closing writeCh
	var producerWG sync.WaitGroup
	producerWG.Add(2)

	go discovery.Run(ctx)
	go func() { defer producerWG.Done(); wsCollector.Run(ctx) }()
	go func() { defer producerWG.Done(); poller.Run(ctx) }()

	// HTTP server
	routerResult := apphttp.NewRouter(repo, zapLogger, cfg)

	srv := &http.Server{
		Addr:              ":" + cfg.ServerPort,
		Handler:           routerResult.Engine,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		zapLogger.Info("HTTP server listening", zap.String("port", cfg.ServerPort))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zapLogger.Fatal("HTTP server error", zap.Error(err))
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zapLogger.Info("shutting down...")

	// Stop collectors first
	cancel()

	// Wait for producers to finish before closing the write channel
	producerWG.Wait()
	close(writeCh)
	<-writerDone

	// Shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		zapLogger.Error("HTTP server shutdown error", zap.Error(err))
	}

	routerResult.Shutdown()

	zapLogger.Info("shutdown complete")
}

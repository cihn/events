package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"events/internal/config"
	"events/internal/dedup"
	"events/internal/handler"
	"events/internal/pipeline"
	"events/internal/repository"
	"events/internal/server"
	"events/internal/service"
)

// @title           Events
// @version         1.0
// @description     High-throughput event ingestion and analytics service backed by ClickHouse.
// @description     Supports single and bulk event ingestion with async batched writes,
// @description     in-memory deduplication, and aggregated metrics queries.
// @host            localhost:8080
// @BasePath        /
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg := config.Load()

	chCfg := repository.ClickHouseConfig{
		Addr:     cfg.ClickHouse.Addr,
		Database: cfg.ClickHouse.Database,
		Username: cfg.ClickHouse.Username,
		Password: cfg.ClickHouse.Password,
		Protocol: cfg.ClickHouse.Protocol,
	}

	var conn driver.Conn
	var connErr error
	for attempt := 1; attempt <= 30; attempt++ {
		conn, connErr = repository.NewClickHouseConn(chCfg, logger)
		if connErr == nil {
			break
		}
		logger.Warn("clickhouse not ready, retrying",
			"attempt", attempt, "error", connErr)
		time.Sleep(2 * time.Second)
	}
	if connErr != nil {
		logger.Error("failed to connect to clickhouse", "error", connErr)
		os.Exit(1)
	}

	if err := repository.RunMigrations(context.Background(), conn, cfg.ClickHouse.Database, logger); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}

	repo := repository.NewEventRepository(conn, cfg.ClickHouse.Database)
	dedupCache := dedup.NewCache(cfg.Dedup.CacheSize)

	pipe := pipeline.New(
		repo,
		cfg.Pipeline.BufferSize,
		cfg.Pipeline.Workers,
		cfg.Pipeline.BatchSize,
		cfg.Pipeline.FlushInterval,
		logger,
	)
	pipe.Start()

	eventSvc := service.NewEventService(pipe, dedupCache)
	metricsSvc := service.NewMetricsService(repo)

	eventH := handler.NewEventHandler(eventSvc)
	metricsH := handler.NewMetricsHandler(metricsSvc)
	healthH := handler.NewHealthHandler(conn, pipe)

	router := server.NewRouter(eventH, metricsH, healthH, logger)

	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		logger.Info("server starting", "port", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown error", "error", err)
	}

	pipe.Stop()
	logger.Info("shutdown complete")
}

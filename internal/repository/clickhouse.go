package repository

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

type ClickHouseConfig struct {
	Addr     string
	Database string
	Username string
	Password string
	Protocol string
}

func NewClickHouseConn(cfg ClickHouseConfig, logger *slog.Logger) (driver.Conn, error) {
	opts := &clickhouse.Options{
		Addr: []string{cfg.Addr},
		Auth: clickhouse.Auth{
			Username: cfg.Username,
			Password: cfg.Password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout:     10 * time.Second,
		MaxOpenConns:    20,
		MaxIdleConns:    10,
		ConnMaxLifetime: 10 * time.Minute,
	}

	if cfg.Protocol == "http" {
		opts.Protocol = clickhouse.HTTP
	}

	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("clickhouse open: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("clickhouse ping: %w", err)
	}

	logger.Info("connected to clickhouse", "addr", cfg.Addr, "protocol", cfg.Protocol)
	return conn, nil
}

// RunMigrations creates the database and events table if they don't exist.
// Called automatically on startup so the service is ready to ingest immediately.
func RunMigrations(ctx context.Context, conn driver.Conn, database string, logger *slog.Logger) error {
	if err := conn.Exec(ctx, "CREATE DATABASE IF NOT EXISTS "+database); err != nil {
		return fmt.Errorf("create database: %w", err)
	}

	createTable := `
		CREATE TABLE IF NOT EXISTS ` + database + `.events (
			event_id     String,
			event_name   LowCardinality(String),
			user_id      String,
			timestamp    DateTime64(3, 'UTC'),
			channel      LowCardinality(String) DEFAULT '',
			campaign_id  String DEFAULT '',
			tags         Array(String),
			metadata     String DEFAULT '{}',
			ingested_at  DateTime64(3, 'UTC') DEFAULT now64(3)
		) ENGINE = ReplacingMergeTree(ingested_at)
		PARTITION BY toYYYYMM(timestamp)
		ORDER BY (event_name, user_id, timestamp, event_id)
		TTL toDateTime(timestamp) + INTERVAL 1 YEAR
		SETTINGS index_granularity = 8192
	`

	if err := conn.Exec(ctx, createTable); err != nil {
		return fmt.Errorf("create events table: %w", err)
	}

	logger.Info("migrations completed", "database", database)
	return nil
}

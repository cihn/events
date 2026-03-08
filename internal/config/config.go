package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server     ServerConfig
	ClickHouse ClickHouseConfig
	Pipeline   PipelineConfig
	Dedup      DedupConfig
}

type ServerConfig struct {
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

type ClickHouseConfig struct {
	Addr     string
	Database string
	Username string
	Password string
	Protocol string // "native" or "http"
}

type PipelineConfig struct {
	BufferSize    int
	Workers       int
	BatchSize     int
	FlushInterval time.Duration
}

type DedupConfig struct {
	CacheSize int
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:            getEnv("SERVER_PORT", "8080"),
			ReadTimeout:     getDuration("SERVER_READ_TIMEOUT", 5*time.Second),
			WriteTimeout:    getDuration("SERVER_WRITE_TIMEOUT", 10*time.Second),
			ShutdownTimeout: getDuration("SERVER_SHUTDOWN_TIMEOUT", 30*time.Second),
		},
		ClickHouse: ClickHouseConfig{
			Addr:     getEnv("CLICKHOUSE_ADDR", "localhost:8123"),
			Database: getEnv("CLICKHOUSE_DATABASE", "events_db"),
			Username: getEnv("CLICKHOUSE_USERNAME", "default"),
			Password: getEnv("CLICKHOUSE_PASSWORD", ""),
			Protocol: getEnv("CLICKHOUSE_PROTOCOL", "http"),
		},
		Pipeline: PipelineConfig{
			BufferSize:    getInt("PIPELINE_BUFFER_SIZE", 100000),
			Workers:       getInt("PIPELINE_WORKERS", 4),
			BatchSize:     getInt("PIPELINE_BATCH_SIZE", 5000),
			FlushInterval: getDuration("PIPELINE_FLUSH_INTERVAL", 500*time.Millisecond),
		},
		Dedup: DedupConfig{
			CacheSize: getInt("DEDUP_CACHE_SIZE", 1000000),
		},
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

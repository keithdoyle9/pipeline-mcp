package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ServerName          string
	Version             string
	LogLevel            slog.Level
	GitHubAPIBaseURL    string
	GitHubReadToken     string
	GitHubWriteToken    string
	DisableMutations    bool
	AuditLogPath        string
	MetricsExportPath   string
	MaxLogBytes         int64
	DefaultLookbackDays int
	MaxHistoricalRuns   int
	HTTPTimeout         time.Duration
	UserAgent           string
	Actor               string
}

func Load() (*Config, error) {
	level, err := parseLogLevel(getEnv("LOG_LEVEL", "info"))
	if err != nil {
		return nil, err
	}

	maxLogBytes, err := getEnvInt64("MAX_LOG_BYTES", 20*1024*1024)
	if err != nil {
		return nil, fmt.Errorf("parse MAX_LOG_BYTES: %w", err)
	}

	lookback, err := getEnvInt("DEFAULT_LOOKBACK_DAYS", 14)
	if err != nil {
		return nil, fmt.Errorf("parse DEFAULT_LOOKBACK_DAYS: %w", err)
	}

	maxRuns, err := getEnvInt("MAX_HISTORICAL_RUNS", 100)
	if err != nil {
		return nil, fmt.Errorf("parse MAX_HISTORICAL_RUNS: %w", err)
	}

	httpTimeoutSeconds, err := getEnvInt("HTTP_TIMEOUT_SECONDS", 25)
	if err != nil {
		return nil, fmt.Errorf("parse HTTP_TIMEOUT_SECONDS: %w", err)
	}

	disableMutations, err := getEnvBool("DISABLE_MUTATIONS", true)
	if err != nil {
		return nil, fmt.Errorf("parse DISABLE_MUTATIONS: %w", err)
	}

	cfg := &Config{
		ServerName:          getEnv("SERVER_NAME", "pipeline-mcp"),
		Version:             getEnv("VERSION", "v0.1.0"),
		LogLevel:            level,
		GitHubAPIBaseURL:    strings.TrimRight(getEnv("GITHUB_API_BASE_URL", "https://api.github.com"), "/"),
		GitHubReadToken:     firstEnv("GITHUB_READ_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"),
		GitHubWriteToken:    firstEnv("GITHUB_WRITE_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"),
		DisableMutations:    disableMutations,
		AuditLogPath:        getEnv("AUDIT_LOG_PATH", "var/audit-events.jsonl"),
		MetricsExportPath:   strings.TrimSpace(os.Getenv("METRICS_EXPORT_PATH")),
		MaxLogBytes:         maxLogBytes,
		DefaultLookbackDays: lookback,
		MaxHistoricalRuns:   maxRuns,
		HTTPTimeout:         time.Duration(httpTimeoutSeconds) * time.Second,
		UserAgent:           getEnv("USER_AGENT", "pipeline-mcp/0.1.0"),
		Actor:               getEnv("ACTOR", "pipeline-mcp"),
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return defaultValue
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func getEnvInt(key string, defaultValue int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func getEnvInt64(key string, defaultValue int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func getEnvBool(key string, defaultValue bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, err
	}
	return parsed, nil
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level %q", value)
	}
}

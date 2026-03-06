package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/keithdoyle9/pipeline-mcp/internal/buildinfo"
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
	AuditSigningKey     string
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

	version := getEnv("VERSION", buildinfo.Version)

	cfg := &Config{
		ServerName:          getEnv("SERVER_NAME", "pipeline-mcp"),
		Version:             version,
		LogLevel:            level,
		GitHubAPIBaseURL:    strings.TrimRight(getEnv("GITHUB_API_BASE_URL", "https://api.github.com"), "/"),
		GitHubReadToken:     firstEnv("GITHUB_READ_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"),
		GitHubWriteToken:    strings.TrimSpace(os.Getenv("GITHUB_WRITE_TOKEN")),
		DisableMutations:    disableMutations,
		AuditLogPath:        getEnv("AUDIT_LOG_PATH", "var/audit-events.jsonl"),
		AuditSigningKey:     strings.TrimSpace(os.Getenv("AUDIT_SIGNING_KEY")),
		MetricsExportPath:   strings.TrimSpace(os.Getenv("METRICS_EXPORT_PATH")),
		MaxLogBytes:         maxLogBytes,
		DefaultLookbackDays: lookback,
		MaxHistoricalRuns:   maxRuns,
		HTTPTimeout:         time.Duration(httpTimeoutSeconds) * time.Second,
		UserAgent:           getEnv("USER_AGENT", defaultUserAgent(version)),
		Actor:               getEnv("ACTOR", "pipeline-mcp"),
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func defaultUserAgent(version string) string {
	return fmt.Sprintf("pipeline-mcp/%s", strings.TrimSpace(version))
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

func validate(cfg *Config) error {
	if cfg.MaxLogBytes <= 0 {
		return fmt.Errorf("MAX_LOG_BYTES must be greater than zero")
	}
	if cfg.DefaultLookbackDays <= 0 {
		return fmt.Errorf("DEFAULT_LOOKBACK_DAYS must be greater than zero")
	}
	if cfg.MaxHistoricalRuns <= 0 {
		return fmt.Errorf("MAX_HISTORICAL_RUNS must be greater than zero")
	}
	if cfg.HTTPTimeout <= 0 {
		return fmt.Errorf("HTTP_TIMEOUT_SECONDS must be greater than zero")
	}
	if !cfg.DisableMutations && strings.TrimSpace(cfg.GitHubWriteToken) == "" {
		return fmt.Errorf("GITHUB_WRITE_TOKEN is required when DISABLE_MUTATIONS=false")
	}

	parsedURL, err := url.Parse(cfg.GitHubAPIBaseURL)
	if err != nil {
		return fmt.Errorf("parse GITHUB_API_BASE_URL: %w", err)
	}
	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return fmt.Errorf("GITHUB_API_BASE_URL must use http or https")
	}
	if strings.TrimSpace(parsedURL.Host) == "" {
		return fmt.Errorf("GITHUB_API_BASE_URL must include a host")
	}

	return nil
}

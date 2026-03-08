package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/keithdoyle9/pipeline-mcp/config"
	"github.com/keithdoyle9/pipeline-mcp/internal/audit"
	"github.com/keithdoyle9/pipeline-mcp/internal/githubapi"
	"github.com/keithdoyle9/pipeline-mcp/internal/service"
	"github.com/keithdoyle9/pipeline-mcp/internal/telemetry"
	"github.com/keithdoyle9/pipeline-mcp/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "pipeline-mcp failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	telemetryCollector := telemetry.NewCollector(cfg.MetricsExportPath)
	ghClient := githubapi.NewClient(cfg.GitHubAPIBaseURL, cfg.GitHubReadToken, cfg.GitHubWriteToken, cfg.UserAgent, cfg.HTTPTimeout)
	ghProvider := githubapi.NewProviderAdapter(ghClient, cfg.GitHubAPIBaseURL)
	auditStore := audit.NewJSONLStore(cfg.AuditLogPath, cfg.AuditSigningKey)
	svc := service.New(cfg, ghProvider, auditStore, telemetryCollector, logger)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    cfg.ServerName,
		Version: cfg.Version,
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{
			Tools: &mcp.ToolCapabilities{},
		},
	})

	tools.Register(server, tools.Dependencies{Service: svc, Telemetry: telemetryCollector, Logger: logger})

	logger.Info("starting pipeline-mcp", "version", cfg.Version)
	err = server.Run(ctx, &mcp.StdioTransport{})
	if err != nil {
		return err
	}

	snapshot := telemetryCollector.Snapshot()
	logger.Info("telemetry snapshot", "snapshot", snapshot)
	return nil
}

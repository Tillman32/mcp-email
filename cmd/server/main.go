package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"

	"github.com/brandon/mcp-email/internal/cache"
	"github.com/brandon/mcp-email/internal/config"
	"github.com/brandon/mcp-email/internal/email"
	"github.com/brandon/mcp-email/internal/mcp"
)

var (
	version     = "dev"
	showVersion = flag.Bool("version", false, "Show version information")
)

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Printf("mcp-email-server version %s\n", version)
		os.Exit(0)
	}
	// Set up logging
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetOutput(os.Stderr)

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.WithError(err).Fatal("Failed to load configuration")
	}

	// Validate configuration
	if valErr := cfg.Validate(); valErr != nil {
		logger.WithError(valErr).Fatal("Invalid configuration")
	}

	// Set log level
	level, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	logger.Info("Starting MCP Email Server")

	// Initialize cache
	emailCache, err := cache.NewCache(cfg.CachePath, logger)
	if err != nil {
		logger.WithError(err).Fatal("Failed to initialize cache")
	}
	defer emailCache.Close()

	// Initialize cache store
	cacheStore := cache.NewStore(emailCache, logger)

	// Initialize accounts in cache
	for i := range cfg.Accounts {
		if _, upsertErr := cacheStore.UpsertAccount(&cfg.Accounts[i]); upsertErr != nil {
			logger.WithError(upsertErr).WithField("account", cfg.Accounts[i].Name).Warn("Failed to cache account")
		}
	}

	// Initialize email manager
	emailManager, err := email.NewManager(cfg, cacheStore, logger)
	if err != nil {
		logger.WithError(err).Fatal("Failed to create email manager")
	}
	defer emailManager.Close()

	// Create MCP server
	server, err := mcp.NewServer(cfg, emailManager, cacheStore, logger)
	if err != nil {
		logger.WithError(err).Fatal("Failed to create MCP server")
	}

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Run server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := server.Run(ctx); err != nil {
			errChan <- err
		}
	}()

	// Wait for shutdown signal or error
	select {
	case sig := <-sigChan:
		logger.WithField("signal", sig).Info("Received shutdown signal")
		cancel()
	case err := <-errChan:
		logger.WithError(err).Error("Server error")
		cancel()
	}

	logger.Info("Shutting down MCP Email Server")
}

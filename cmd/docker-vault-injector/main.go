package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/vyktory/docker-vault-injector/internal/config"
	"github.com/vyktory/docker-vault-injector/internal/controller"
	"github.com/vyktory/docker-vault-injector/internal/dockerclient"
	"github.com/vyktory/docker-vault-injector/internal/vaultclient"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "docker-vault-injector:", err)
		os.Exit(1)
	}
}

func run() error {
	configuration, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: configuration.LogLevel,
	}))
	slog.SetDefault(logger)

	docker, err := dockerclient.NewFromEnvironment()
	if err != nil {
		return err
	}
	defer docker.Close()

	vault, err := vaultclient.NewFromEnvironment(configuration.VaultTokenFile)
	if err != nil {
		return err
	}

	reconciler := controller.New(
		docker,
		vault,
		logger,
		configuration.PollInterval,
		configuration.ReconcileTimeout,
		configuration.EventRetryInterval,
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("starting docker-vault-injector",
		"poll_interval", configuration.PollInterval,
		"reconcile_timeout", configuration.ReconcileTimeout,
	)
	if err := reconciler.Run(ctx); err != nil {
		return fmt.Errorf("run controller: %w", err)
	}
	logger.Info("docker-vault-injector stopped")
	return nil
}

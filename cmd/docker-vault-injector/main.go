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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	docker, err := dockerclient.NewFromEnvironment()
	if err != nil {
		return err
	}
	defer docker.Close()

	vault, err := vaultclient.NewFromEnvironment()
	if err != nil {
		return err
	}
	if err := vault.Authenticate(ctx, vaultclient.AuthConfig{
		Method:              configuration.VaultAuth.Method,
		AppRoleAuthPath:     configuration.VaultAuth.AppRoleAuthPath,
		AppRoleRoleID:       configuration.VaultAuth.AppRoleRoleID,
		AppRoleRoleIDFile:   configuration.VaultAuth.AppRoleRoleIDFile,
		AppRoleSecretID:     configuration.VaultAuth.AppRoleSecretID,
		AppRoleSecretIDFile: configuration.VaultAuth.AppRoleSecretIDFile,
		Token:               configuration.VaultAuth.Token,
		TokenFile:           configuration.VaultAuth.TokenFile,
		TokenCheckInterval:  configuration.VaultAuth.TokenCheckInterval,
		AuthRetryInterval:   configuration.VaultAuth.AuthRetryInterval,
	}); err != nil {
		return fmt.Errorf("authenticate to Vault: %w", err)
	}
	if vault.MaintainsToken() {
		go vault.RunTokenLifecycle(ctx, logger)
	}

	reconciler := controller.New(
		docker,
		vault,
		logger,
		configuration.PollInterval,
		configuration.ReconcileTimeout,
		configuration.EventRetryInterval,
	)

	logger.Info("starting docker-vault-injector",
		"poll_interval", configuration.PollInterval,
		"reconcile_timeout", configuration.ReconcileTimeout,
		"vault_auth_method", configuration.VaultAuth.Method,
	)
	if err := reconciler.Run(ctx); err != nil {
		return fmt.Errorf("run controller: %w", err)
	}
	logger.Info("docker-vault-injector stopped")
	return nil
}

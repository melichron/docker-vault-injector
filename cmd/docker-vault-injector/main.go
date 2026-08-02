package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/melichron/docker-vault-injector/internal/config"
	"github.com/melichron/docker-vault-injector/internal/controller"
	"github.com/melichron/docker-vault-injector/internal/dockerclient"
	statusview "github.com/melichron/docker-vault-injector/internal/status"
	"github.com/melichron/docker-vault-injector/internal/vaultclient"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "docker-vault-injector:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 {
		if len(os.Args) == 2 {
			switch os.Args[1] {
			case "status":
				return runStatus(os.Stdout, config.StatusFileFromEnvironment(), false)
			case "status-yaml":
				return runStatus(os.Stdout, config.StatusFileFromEnvironment(), true)
			}
		}
		return fmt.Errorf("usage: docker-vault-injector [status|status-yaml]")
	}

	configuration, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: configuration.LogLevel,
	}))
	slog.SetDefault(logger)
	statusStore, err := statusview.NewStore(configuration.StatusFile)
	if err != nil {
		// Status reporting is observability only. A read-only filesystem or
		// another status-file problem must not stop secret reconciliation.
		logger.Warn("status snapshot is disabled", "path", configuration.StatusFile, "error", err)
		statusStore = nil
	}

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
		statusStore,
	)

	logger.Info("starting docker-vault-injector",
		"poll_interval", configuration.PollInterval,
		"reconcile_timeout", configuration.ReconcileTimeout,
		"vault_auth_method", configuration.VaultAuth.Method,
		"status_file", configuration.StatusFile,
	)
	if err := reconciler.Run(ctx); err != nil {
		return fmt.Errorf("run controller: %w", err)
	}
	logger.Info("docker-vault-injector stopped")
	return nil
}

func runStatus(writer io.Writer, path string, yamlOutput bool) error {
	snapshot, err := statusview.Read(path)
	if err != nil {
		return err
	}
	if yamlOutput {
		return statusview.WriteYAML(writer, snapshot)
	}
	return statusview.WriteTable(writer, snapshot)
}

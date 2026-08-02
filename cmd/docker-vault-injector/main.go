package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/melichron/docker-vault-injector/internal/config"
	"github.com/melichron/docker-vault-injector/internal/controller"
	"github.com/melichron/docker-vault-injector/internal/dockerclient"
	"github.com/melichron/docker-vault-injector/internal/retry"
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
			case "health":
				maximumAge, err := config.HealthMaxAgeFromEnvironment()
				if err != nil {
					return err
				}
				return runHealth(os.Stdout, config.StatusFileFromEnvironment(), maximumAge, time.Now())
			}
		}
		return fmt.Errorf("usage: docker-vault-injector [status|status-yaml|health]")
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
	authConfiguration := vaultclient.AuthConfig{
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
		AuthRetryMaximum:    configuration.VaultAuth.AuthRetryMaximum,
	}
	if err := authenticateVaultUntilSuccess(ctx, vault, authConfiguration, logger, statusStore); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
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
		configuration.EventRetryMaximum,
		statusStore,
	)

	logger.Info("starting docker-vault-injector",
		"poll_interval", configuration.PollInterval,
		"reconcile_timeout", configuration.ReconcileTimeout,
		"vault_auth_method", configuration.VaultAuth.Method,
		"status_file", configuration.StatusFile,
		"health_max_age", configuration.HealthMaxAge,
	)
	if err := reconciler.Run(ctx); err != nil {
		return fmt.Errorf("run controller: %w", err)
	}
	logger.Info("docker-vault-injector stopped")
	return nil
}

type vaultAuthenticator interface {
	Authenticate(context.Context, vaultclient.AuthConfig) error
}

func authenticateVaultUntilSuccess(
	ctx context.Context,
	vault vaultAuthenticator,
	configuration vaultclient.AuthConfig,
	logger *slog.Logger,
	statusStore *statusview.Store,
) error {
	backoff := retry.NewBackoff(configuration.AuthRetryInterval, configuration.AuthRetryMaximum)
	for ctx.Err() == nil {
		touchStatus(statusStore, logger)
		if err := vault.Authenticate(ctx, configuration); err == nil {
			return nil
		} else {
			delay := backoff.Next()
			// A Vault request may consume most of the health window. Record
			// progress again before entering the retry delay.
			touchStatus(statusStore, logger)
			logger.Error("initial Vault authentication failed; will retry",
				"retry_after", delay,
				"error", err,
			)
			if !waitForContext(ctx, delay) {
				break
			}
		}
	}
	return ctx.Err()
}

func touchStatus(store *statusview.Store, logger *slog.Logger) {
	if store == nil {
		return
	}
	if err := store.Heartbeat(); err != nil {
		logger.Warn("cannot write controller heartbeat", "error", err)
	}
}

func waitForContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
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

func runHealth(writer io.Writer, path string, maximumAge time.Duration, now time.Time) error {
	snapshot, err := statusview.Read(path)
	if err != nil {
		return err
	}
	if err := statusview.CheckHealth(snapshot, maximumAge, now); err != nil {
		return err
	}
	_, err = fmt.Fprintln(writer, "healthy")
	return err
}

// Package vaultclient is a deliberately thin adapter around HashiCorp's
// official Vault client. Besides KV v2 access it owns authentication because
// the token and the client using that token must have one lifecycle.
package vaultclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	vault "github.com/hashicorp/vault/api"

	"github.com/melichron/docker-vault-injector/internal/retry"
)

const (
	AuthMethodAppRole = "approle"
	AuthMethodToken   = "token"
)

type Secret struct {
	Version int
	Data    map[string]any
}

// AuthConfig is copied from process configuration so this package remains
// independent from internal/config. File values take precedence over direct
// environment values and are re-read before every AppRole login.
type AuthConfig struct {
	Method              string
	AppRoleAuthPath     string
	AppRoleRoleID       string
	AppRoleRoleIDFile   string
	AppRoleSecretID     string
	AppRoleSecretIDFile string
	Token               string
	TokenFile           string
	TokenCheckInterval  time.Duration
	AuthRetryInterval   time.Duration
	AuthRetryMaximum    time.Duration
}

type tokenLease struct {
	renewable bool
	renewAt   time.Time
}

type Client struct {
	client *vault.Client
	auth   AuthConfig
	lease  tokenLease
}

// NewFromEnvironment honors the standard VAULT_ADDR, VAULT_CACERT,
// VAULT_CAPATH, VAULT_CLIENT_CERT, VAULT_CLIENT_KEY, VAULT_NAMESPACE and proxy
// environment variables understood by the official client.
func NewFromEnvironment() (*Client, error) {
	configuration := vault.DefaultConfig()
	if err := configuration.ReadEnvironment(); err != nil {
		return nil, fmt.Errorf("read Vault environment: %w", err)
	}

	client, err := vault.NewClient(configuration)
	if err != nil {
		return nil, fmt.Errorf("create Vault client: %w", err)
	}
	return &Client{client: client}, nil
}

// Authenticate validates configuration and obtains the initial token before
// the reconciliation controller starts. This avoids starting in a state where
// every service immediately fails with an unauthenticated Vault request.
func (c *Client) Authenticate(ctx context.Context, configuration AuthConfig) error {
	configuration.Method = strings.ToLower(strings.TrimSpace(configuration.Method))
	if configuration.Method == "" {
		configuration.Method = AuthMethodAppRole
	}
	c.auth = configuration

	switch configuration.Method {
	case AuthMethodAppRole:
		path, err := normalizeAppRoleAuthPath(configuration.AppRoleAuthPath)
		if err != nil {
			return err
		}
		c.auth.AppRoleAuthPath = path
		if configuration.TokenCheckInterval <= 0 {
			return fmt.Errorf("Vault token check interval must be greater than zero")
		}
		if configuration.AuthRetryInterval <= 0 {
			return fmt.Errorf("Vault auth retry interval must be greater than zero")
		}
		if configuration.AuthRetryMaximum == 0 {
			configuration.AuthRetryMaximum = configuration.AuthRetryInterval
			c.auth.AuthRetryMaximum = configuration.AuthRetryMaximum
		}
		if configuration.AuthRetryMaximum < configuration.AuthRetryInterval {
			return fmt.Errorf("Vault auth retry maximum must be greater than or equal to the initial interval")
		}

		secret, err := c.loginAppRole(ctx)
		if err != nil {
			return err
		}
		lease, err := leaseFromAuthSecret(secret, time.Now())
		if err != nil {
			return err
		}
		c.client.SetToken(secret.Auth.ClientToken)
		c.lease = lease
		return nil

	case AuthMethodToken:
		token, err := readCredential("Vault token", configuration.Token, configuration.TokenFile)
		if err != nil {
			return err
		}
		c.client.SetToken(token)
		if _, err := c.client.Auth().Token().LookupSelfWithContext(ctx); err != nil {
			return fmt.Errorf("validate static Vault token: %w", err)
		}
		return nil

	default:
		return fmt.Errorf("unsupported Vault auth method %q", configuration.Method)
	}
}

// MaintainsToken reports whether this client needs the background lifecycle.
// Static token mode is intentionally static and exists only as an explicit
// development/fallback option.
func (c *Client) MaintainsToken() bool {
	return c.auth.Method == AuthMethodAppRole
}

// RunTokenLifecycle periodically proves that the AppRole token is still valid,
// renews it before its lease expires, and obtains a fresh token whenever lookup
// or renewal fails. It should run as one goroutine for the lifetime of Client.
//
// RoleID and SecretID files are re-read by every login. Operators can therefore
// rotate either credential atomically without restarting the controller.
func (c *Client) RunTokenLifecycle(ctx context.Context, logger *slog.Logger) {
	if !c.MaintainsToken() {
		return
	}

	for ctx.Err() == nil {
		wait := c.auth.TokenCheckInterval
		untilRenewal := time.Until(c.lease.renewAt)
		if untilRenewal < wait {
			wait = untilRenewal
		}
		if wait < 0 {
			wait = 0
		}
		if !waitForContext(ctx, wait) {
			return
		}

		// lookup-self detects externally revoked tokens rather than waiting for
		// the locally calculated TTL to elapse.
		lookup, err := c.client.Auth().Token().LookupSelfWithContext(ctx)
		if err != nil {
			logger.Warn("Vault token is no longer valid; re-authenticating with AppRole", "error", err)
			if !c.reauthenticateUntilSuccess(ctx, logger) {
				return
			}
			continue
		}
		remainingTTL, renewable, err := tokenStatusFromLookup(lookup)
		if err != nil {
			logger.Warn("Vault token lookup returned invalid status; re-authenticating with AppRole", "error", err)
			if !c.reauthenticateUntilSuccess(ctx, logger) {
				return
			}
			continue
		}
		c.lease.renewable = renewable
		// Vault is authoritative. If lookup-self reports less remaining time
		// than our local lease calculation, move renewal earlier.
		lookupRenewAt := time.Now().Add(remainingTTL * 2 / 3)
		if lookupRenewAt.Before(c.lease.renewAt) {
			c.lease.renewAt = lookupRenewAt
		}

		if time.Now().Before(c.lease.renewAt) {
			continue
		}
		if !c.lease.renewable {
			logger.Info("Vault token is not renewable; obtaining a replacement with AppRole")
			if !c.reauthenticateUntilSuccess(ctx, logger) {
				return
			}
			continue
		}

		renewed, err := c.client.Auth().Token().RenewSelfWithContext(ctx, 0)
		if err == nil {
			var lease tokenLease
			lease, err = leaseFromRenewal(renewed, time.Now())
			if err == nil {
				c.lease = lease
				logger.Debug("renewed Vault AppRole token", "next_renewal", lease.renewAt)
				continue
			}
		}

		logger.Warn("cannot renew Vault token; re-authenticating with AppRole", "error", err)
		if !c.reauthenticateUntilSuccess(ctx, logger) {
			return
		}
	}
}

func (c *Client) reauthenticateUntilSuccess(ctx context.Context, logger *slog.Logger) bool {
	backoff := retry.NewBackoff(c.auth.AuthRetryInterval, c.auth.AuthRetryMaximum)
	for ctx.Err() == nil {
		secret, err := c.loginAppRole(ctx)
		if err == nil {
			var lease tokenLease
			lease, err = leaseFromAuthSecret(secret, time.Now())
			if err == nil {
				c.client.SetToken(secret.Auth.ClientToken)
				c.lease = lease
				logger.Info("authenticated to Vault with AppRole",
					"auth_path", c.auth.AppRoleAuthPath,
					"renewable", lease.renewable,
					"next_renewal", lease.renewAt,
				)
				return true
			}
		}

		delay := backoff.Next()
		logger.Error("AppRole authentication failed; will retry",
			"auth_path", c.auth.AppRoleAuthPath,
			"retry_after", delay,
			"error", err,
		)
		if !waitForContext(ctx, delay) {
			return false
		}
	}
	return false
}

func (c *Client) loginAppRole(ctx context.Context) (*vault.Secret, error) {
	roleID, err := readCredential("AppRole RoleID", c.auth.AppRoleRoleID, c.auth.AppRoleRoleIDFile)
	if err != nil {
		return nil, err
	}
	secretID, err := readCredential("AppRole SecretID", c.auth.AppRoleSecretID, c.auth.AppRoleSecretIDFile)
	if err != nil {
		return nil, err
	}

	secret, err := c.client.Logical().WriteWithContext(ctx, c.auth.AppRoleAuthPath+"/login", map[string]any{
		"role_id":   roleID,
		"secret_id": secretID,
	})
	if err != nil {
		return nil, fmt.Errorf("AppRole login at %s: %w", c.auth.AppRoleAuthPath, err)
	}
	if secret == nil || secret.Auth == nil || secret.Auth.ClientToken == "" {
		return nil, fmt.Errorf("AppRole login at %s returned no client token", c.auth.AppRoleAuthPath)
	}
	return secret, nil
}

func normalizeAppRoleAuthPath(value string) (string, error) {
	path := strings.Trim(strings.TrimSpace(value), "/")
	if path == "" {
		return "", fmt.Errorf("VAULT_APPROLE_AUTH_PATH is required for AppRole authentication")
	}
	if !strings.HasPrefix(path, "auth/") {
		return "", fmt.Errorf("VAULT_APPROLE_AUTH_PATH must be a full mount path such as auth/approle")
	}
	if strings.TrimPrefix(path, "auth/") == "" || strings.Contains(path, "//") {
		return "", fmt.Errorf("VAULT_APPROLE_AUTH_PATH must contain a non-empty auth mount name")
	}
	if strings.HasSuffix(path, "/login") {
		return "", fmt.Errorf("VAULT_APPROLE_AUTH_PATH must name the auth mount without /login")
	}
	return path, nil
}

func readCredential(name, directValue, filePath string) (string, error) {
	value := directValue
	if filePath != "" {
		contents, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("read %s file %q: %w", name, filePath, err)
		}
		value = string(contents)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is empty", name)
	}
	return value, nil
}

func leaseFromAuthSecret(secret *vault.Secret, now time.Time) (tokenLease, error) {
	if secret == nil || secret.Auth == nil || secret.Auth.ClientToken == "" {
		return tokenLease{}, fmt.Errorf("Vault authentication response has no client token")
	}
	return leaseFromTTL(secret.Auth.LeaseDuration, secret.Auth.Renewable, now)
}

func leaseFromRenewal(secret *vault.Secret, now time.Time) (tokenLease, error) {
	if secret == nil || secret.Auth == nil {
		return tokenLease{}, fmt.Errorf("Vault token renewal response has no auth metadata")
	}
	return leaseFromTTL(secret.Auth.LeaseDuration, secret.Auth.Renewable, now)
}

func leaseFromTTL(ttlSeconds int, renewable bool, now time.Time) (tokenLease, error) {
	if ttlSeconds <= 0 {
		return tokenLease{}, fmt.Errorf("Vault token has invalid TTL %d", ttlSeconds)
	}
	ttl := time.Duration(ttlSeconds) * time.Second
	return tokenLease{
		renewable: renewable,
		// Renew after two thirds of the granted TTL. This leaves one third for
		// retries or AppRole re-login before the current token actually expires.
		renewAt: now.Add(ttl * 2 / 3),
	}, nil
}

func tokenStatusFromLookup(secret *vault.Secret) (time.Duration, bool, error) {
	if secret == nil || secret.Data == nil {
		return 0, false, fmt.Errorf("Vault token lookup response has no data")
	}

	rawTTL, exists := secret.Data["ttl"]
	if !exists {
		return 0, false, fmt.Errorf("Vault token lookup response has no TTL")
	}
	var ttlSeconds int64
	switch value := rawTTL.(type) {
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return 0, false, fmt.Errorf("parse Vault token TTL: %w", err)
		}
		ttlSeconds = parsed
	case float64:
		ttlSeconds = int64(value)
	case int:
		ttlSeconds = int64(value)
	case int64:
		ttlSeconds = value
	default:
		return 0, false, fmt.Errorf("Vault token lookup returned unsupported TTL type %T", rawTTL)
	}
	if ttlSeconds <= 0 {
		return 0, false, fmt.Errorf("Vault token is expired or has invalid TTL %d", ttlSeconds)
	}

	renewable, exists := secret.Data["renewable"].(bool)
	if !exists {
		return 0, false, fmt.Errorf("Vault token lookup response has no renewable flag")
	}
	return time.Duration(ttlSeconds) * time.Second, renewable, nil
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

func (c *Client) CurrentVersion(ctx context.Context, mount, path string) (int, error) {
	metadata, err := c.client.KVv2(mount).GetMetadata(ctx, path)
	if err != nil {
		return 0, fmt.Errorf("read metadata for %s/%s: %w", mount, path, err)
	}
	if metadata.CurrentVersion <= 0 {
		return 0, fmt.Errorf("Vault returned invalid current version %d for %s/%s", metadata.CurrentVersion, mount, path)
	}
	return metadata.CurrentVersion, nil
}

func (c *Client) ReadVersion(ctx context.Context, mount, path string, version int) (Secret, error) {
	secret, err := c.client.KVv2(mount).GetVersion(ctx, path, version)
	if err != nil {
		return Secret{}, fmt.Errorf("read version %d of %s/%s: %w", version, mount, path, err)
	}
	if secret.Data == nil {
		return Secret{}, fmt.Errorf("version %d of %s/%s is deleted or contains no data", version, mount, path)
	}
	if secret.VersionMetadata == nil {
		return Secret{}, fmt.Errorf("version %d of %s/%s has no version metadata", version, mount, path)
	}
	return Secret{Version: secret.VersionMetadata.Version, Data: secret.Data}, nil
}

// Package vaultclient is a deliberately thin adapter around HashiCorp's
// official Vault client. The rest of the application sees simple Go values and
// does not need to understand Vault's response envelope.
package vaultclient

import (
	"context"
	"fmt"
	"os"
	"strings"

	vault "github.com/hashicorp/vault/api"
)

type Secret struct {
	Version int
	Data    map[string]any
}

type Client struct {
	client *vault.Client
}

// NewFromEnvironment honors the standard VAULT_ADDR, VAULT_CACERT,
// VAULT_CAPATH, VAULT_CLIENT_CERT, VAULT_CLIENT_KEY, VAULT_NAMESPACE and proxy
// environment variables understood by the official client.
func NewFromEnvironment(tokenFile string) (*Client, error) {
	configuration := vault.DefaultConfig()
	if err := configuration.ReadEnvironment(); err != nil {
		return nil, fmt.Errorf("read Vault environment: %w", err)
	}

	client, err := vault.NewClient(configuration)
	if err != nil {
		return nil, fmt.Errorf("create Vault client: %w", err)
	}

	token := os.Getenv("VAULT_TOKEN")
	if tokenFile != "" {
		contents, err := os.ReadFile(tokenFile)
		if err != nil {
			return nil, fmt.Errorf("read VAULT_TOKEN_FILE %q: %w", tokenFile, err)
		}
		token = strings.TrimSpace(string(contents))
	}
	if token == "" {
		return nil, fmt.Errorf("Vault token is empty: set VAULT_TOKEN or VAULT_TOKEN_FILE")
	}
	client.SetToken(token)

	return &Client{client: client}, nil
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

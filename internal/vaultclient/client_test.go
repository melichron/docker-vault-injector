package vaultclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	vault "github.com/hashicorp/vault/api"
)

func TestAuthenticateAppRoleUsesConfiguredPathAndCredentialFiles(t *testing.T) {
	credentialDirectory := t.TempDir()
	roleIDFile := filepath.Join(credentialDirectory, "role-id")
	secretIDFile := filepath.Join(credentialDirectory, "secret-id")
	if err := os.WriteFile(roleIDFile, []byte("role-from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretIDFile, []byte("secret-from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/auth/swarm/login" {
			t.Errorf("request path = %q, want /v1/auth/swarm/login", request.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body["role_id"] != "role-from-file" || body["secret_id"] != "secret-from-file" {
			t.Errorf("unexpected login body: %#v", body)
		}
		writeAuthResponse(response, "token-from-approle", 60, true)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.Authenticate(context.Background(), AuthConfig{
		Method:              AuthMethodAppRole,
		AppRoleAuthPath:     "/auth/swarm/",
		AppRoleRoleID:       "ignored-direct-role",
		AppRoleRoleIDFile:   roleIDFile,
		AppRoleSecretID:     "ignored-direct-secret",
		AppRoleSecretIDFile: secretIDFile,
		TokenCheckInterval:  time.Second,
		AuthRetryInterval:   time.Second,
	})
	if err != nil {
		t.Fatalf("Authenticate returned an error: %v", err)
	}
	if got := client.client.Token(); got != "token-from-approle" {
		t.Fatalf("client token = %q", got)
	}
}

func TestTokenLifecycleReauthenticatesWhenLookupDetectsRevocation(t *testing.T) {
	var loginCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/auth/swarm/login":
			login := loginCount.Add(1)
			writeAuthResponse(response, fmt.Sprintf("token-%d", login), 60, true)
		case "/v1/auth/token/lookup-self":
			if loginCount.Load() == 1 {
				response.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(response, `{"errors":["permission denied"]}`)
				return
			}
			writeLookupResponse(response)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	configuration := AuthConfig{
		Method:             AuthMethodAppRole,
		AppRoleAuthPath:    "auth/swarm",
		AppRoleRoleID:      "role",
		AppRoleSecretID:    "secret",
		TokenCheckInterval: 10 * time.Millisecond,
		AuthRetryInterval:  10 * time.Millisecond,
	}
	if err := client.Authenticate(context.Background(), configuration); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		client.RunTokenLifecycle(ctx, discardLogger())
	}()

	waitForCondition(t, 2*time.Second, func() bool { return loginCount.Load() >= 2 })
	if got := client.client.Token(); got != "token-2" {
		t.Fatalf("client did not switch to replacement token: %q", got)
	}
	cancel()
	<-done
}

func TestTokenLifecycleRenewsBeforeTTLExpires(t *testing.T) {
	var renewCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/auth/swarm/login":
			writeAuthResponse(response, "token-1", 1, true)
		case "/v1/auth/token/lookup-self":
			writeLookupResponse(response)
		case "/v1/auth/token/renew-self":
			renewCount.Add(1)
			// Vault renewal responses carry updated auth lease metadata. They do
			// not need to repeat the client token.
			writeAuthResponse(response, "", 2, true)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	if err := client.Authenticate(context.Background(), AuthConfig{
		Method:             AuthMethodAppRole,
		AppRoleAuthPath:    "auth/swarm",
		AppRoleRoleID:      "role",
		AppRoleSecretID:    "secret",
		TokenCheckInterval: 20 * time.Millisecond,
		AuthRetryInterval:  10 * time.Millisecond,
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		client.RunTokenLifecycle(ctx, discardLogger())
	}()
	waitForCondition(t, 2*time.Second, func() bool { return renewCount.Load() >= 1 })
	cancel()
	<-done
}

func TestNormalizeAppRoleAuthPathRequiresFullMountPath(t *testing.T) {
	for _, invalid := range []string{"", "approle", "auth/", "auth//nested", "auth/approle/login"} {
		if _, err := normalizeAppRoleAuthPath(invalid); err == nil {
			t.Fatalf("normalizeAppRoleAuthPath(%q) should fail", invalid)
		}
	}
	if got, err := normalizeAppRoleAuthPath("/auth/custom/"); err != nil || got != "auth/custom" {
		t.Fatalf("normalized path = %q, err = %v", got, err)
	}
}

func TestTokenStatusFromLookupRejectsExpiredToken(t *testing.T) {
	_, _, err := tokenStatusFromLookup(&vault.Secret{Data: map[string]any{
		"ttl":       json.Number("0"),
		"renewable": true,
	}})
	if err == nil {
		t.Fatal("zero TTL should be treated as an expired token")
	}
}

func newTestClient(t *testing.T, address string) *Client {
	t.Helper()
	configuration := vault.DefaultConfig()
	configuration.Address = address
	configuration.MaxRetries = 0
	raw, err := vault.NewClient(configuration)
	if err != nil {
		t.Fatal(err)
	}
	return &Client{client: raw}
}

func writeAuthResponse(response http.ResponseWriter, token string, ttl int, renewable bool) {
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(map[string]any{
		"auth": map[string]any{
			"client_token":   token,
			"lease_duration": ttl,
			"renewable":      renewable,
		},
	})
}

func writeLookupResponse(response http.ResponseWriter) {
	response.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(response).Encode(map[string]any{
		"data": map[string]any{"ttl": 60, "renewable": true},
	})
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

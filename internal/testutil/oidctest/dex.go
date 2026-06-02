package oidctest

import (
	"context"
	_ "embed"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

//go:embed dex-config.yaml
var dexConfigYAML string

// Instance holds the coordinates of a running dex container.
type Instance struct {
	// Issuer is the OIDC issuer URL (http://localhost:<port>).
	Issuer string
	// ClientID is the server-side OAuth2 client ID.
	ClientID string
	// ClientSecret is the server-side OAuth2 client secret.
	ClientSecret string
	// CLIClientID is the public OAuth2 client ID for CLI / device flows.
	CLIClientID string
	// StaticUser is the pre-seeded test user.
	StaticUser StaticUser
	container  testcontainers.Container
}

// StaticUser is the pre-seeded OIDC user available in the dex container.
type StaticUser struct {
	Email    string
	Password string
	Sub      string
}

// Option is a functional option for StartDex.
type Option func(*options)

type options struct {
	extraRedirectURI string
}

// WithRedirectURI registers an additional redirect URI in the dex static
// client config. Use this in integration tests where the httptest server
// listens on a random port.
func WithRedirectURI(uri string) Option {
	return func(o *options) { o.extraRedirectURI = uri }
}

// StartDex boots a dex container and returns an Instance. Cleanup is
// registered with t.Cleanup. Requires Docker (or Podman with Docker socket).
func StartDex(t *testing.T, opts ...Option) *Instance {
	t.Helper()

	o := &options{}
	for _, opt := range opts {
		opt(o)
	}

	// Find a free port on the host so we can wire the dex issuer URL before
	// starting the container (Approach C — no chicken-and-egg with MappedPort).
	hostPort, err := freePort()
	if err != nil {
		t.Fatalf("oidctest: freePort: %v", err)
	}
	issuer := fmt.Sprintf("http://localhost:%d", hostPort)
	configured := strings.ReplaceAll(
		dexConfigYAML,
		"DEX_ISSUER_PLACEHOLDER",
		fmt.Sprintf("localhost:%d", hostPort),
	)
	if o.extraRedirectURI != "" {
		configured = strings.ReplaceAll(configured, "DEX_REDIRECT_URI_PLACEHOLDER", o.extraRedirectURI)
	} else {
		// Remove the placeholder line entirely when no extra URI is needed.
		configured = strings.ReplaceAll(configured, "\n      - 'DEX_REDIRECT_URI_PLACEHOLDER'", "")
	}

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "ghcr.io/dexidp/dex:v2.41.1",
		ExposedPorts: []string{fmt.Sprintf("%d:5556/tcp", hostPort)},
		Cmd:          []string{"dex", "serve", "/etc/dex/config.yaml"},
		WaitingFor: wait.ForHTTP("/.well-known/openid-configuration").
			WithPort("5556/tcp").
			WithStartupTimeout(30 * time.Second),
		Files: []testcontainers.ContainerFile{
			{
				Reader:            strings.NewReader(configured),
				ContainerFilePath: "/etc/dex/config.yaml",
				FileMode:          0o644,
			},
		},
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		// Skip rather than fail when Docker is simply not available. This lets
		// the integration test run cleanly in CI without Docker and avoids a
		// hard failure that obscures other test output.
		if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") ||
			strings.Contains(err.Error(), "docker: command not found") ||
			strings.Contains(err.Error(), "no such file or directory") {
			t.Skipf("oidctest: Docker unavailable — skipping integration test (%v); run manual smoke test (docs/runbook/m1-smoke-test.md)", err)
		}
		t.Fatalf("oidctest: start dex container: %v", err)
	}
	t.Cleanup(func() {
		_ = c.Terminate(context.Background())
	})

	return &Instance{
		Issuer:       issuer,
		ClientID:     "flow-server",
		ClientSecret: "flow-server-secret",
		CLIClientID:  "flow-cli",
		StaticUser: StaticUser{
			Email:    "alice@example.com",
			Password: "password",
			Sub:      "alice-static-uid",
		},
		container: c,
	}
}

// freePort asks the kernel for an unused TCP port and returns it.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

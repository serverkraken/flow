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

// StartDex boots a dex container and returns an Instance. Cleanup is
// registered with t.Cleanup. Requires Docker (or Podman with Docker socket).
func StartDex(t *testing.T) *Instance {
	t.Helper()

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

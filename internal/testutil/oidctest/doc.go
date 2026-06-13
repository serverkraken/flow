// Package oidctest spins up dexidp/dex as a lightweight OIDC IdP for
// integration tests. Each call to StartDex returns a running container with
// a unique issuer URL and the matching client credentials.
package oidctest

// Package oidcserver verifies OIDC ID-Tokens issued by an external IdP
// (Authentik in production, dex in tests). It uses coreos/go-oidc which
// internally maintains a JWKS cache keyed by issuer URL and refreshes on
// kid-miss.
package oidcserver

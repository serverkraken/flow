// Package oidcclient implements the OAuth2 Device-Authorization-Grant
// (RFC 8628) so flow's CLI/TUI/MCP can authenticate against Authentik
// without a browser callback. It also implements refresh-token rotation
// (refresh.go) and a Tokens facade (tokens.go) over the TokenStore.
package oidcclient

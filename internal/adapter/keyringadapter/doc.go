// Package keyringadapter implements ports.TokenStore against the OS keychain
// via zalando/go-keyring (macOS Keychain, GNOME Keyring, KWallet, libsecret).
// Tokens are JSON-encoded; Fake is the in-memory implementation for tests.
package keyringadapter

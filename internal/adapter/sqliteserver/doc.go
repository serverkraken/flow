// Package sqliteserver is flow-server's central SQLite store. Holds
// multi-user data (Phase 1 single-user via allowlist, schema is multi-
// user-ready). Mutations increment a global lamport counter; the
// counter value becomes the row's `version` and is what clients use to
// pull-watermark.
package sqliteserver

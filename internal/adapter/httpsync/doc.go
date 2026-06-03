// Package httpsync is flow's client-side sync engine. It contains:
//   - Client: typed REST wrappers around flow-server's /api/v1/{projects,sessions,active} endpoints.
//   - Queue: typed enqueue + drain over ports.WriteQueue for offline write durability.
//   - Worker: background goroutine that polls server every 30s + drains the queue on demand,
//     emitting ports.ConflictMsg on 409 responses (Task 29).
package httpsync

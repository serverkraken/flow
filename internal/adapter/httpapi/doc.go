// Package httpapi implements the client-side ports against flow-server's
// /api/v1 REST surface. One truth (the server), thin clients (Spec §5/§8):
// reads go through a cache fed by SSE invalidation, writes are synchronous.
package httpapi

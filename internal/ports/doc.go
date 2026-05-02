// Package ports declares the interfaces use cases depend on. Implementations
// live in internal/adapter/<name>; in-memory test doubles in internal/testutil.
//
// Dependency rule: domain + stdlib (context, io, time) only. Populated by
// phase F2.1 of the hexagonal refactor.
package ports

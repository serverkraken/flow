// Package usecase contains application logic — one file per use case.
// Each use case takes ports as constructor dependencies and exposes a single
// method that orchestrates domain logic against those ports.
//
// Dependency rule: domain + ports + stdlib only — no adapters, no I/O libs.
// Populated by phases F2.2 (read paths) and F2.3 (write paths).
package usecase

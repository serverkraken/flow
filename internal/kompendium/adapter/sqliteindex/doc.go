// Package sqliteindex implements ports.Indexer using a SQLite database with
// the FTS5 virtual table extension. The index lives outside the notebook so
// per-machine rebuilds stay cheap and the notebook itself remains pure
// Markdown — see CLAUDE.md section 11.
package sqliteindex

// Package sqliteclient is the per-device local cache backing flow's
// worktime + (later) repo-note data. Schema lives in `migrations/` and
// is applied on Open via embedded goose. All sub-adapters in this
// package share the *sql.DB returned by Store.DB().
package sqliteclient

package tarsnapshot

// SetCapsForTest swaps the decompression-bomb limits and returns a
// restorer. Lets the external test suite verify the per-entry AND
// cumulative caps without writing multi-GiB fixture archives. The
// helper does not synchronise — callers must keep the affected test
// non-parallel for the duration of the override.
func SetCapsForTest(entry, total int64) (restore func()) {
	prevEntry, prevTotal := maxEntryBytes, maxTotalBytes
	maxEntryBytes, maxTotalBytes = entry, total
	return func() { maxEntryBytes, maxTotalBytes = prevEntry, prevTotal }
}

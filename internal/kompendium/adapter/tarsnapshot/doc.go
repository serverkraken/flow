// Package tarsnapshot implements ports.TarSnapshot using stdlib archive/tar
// and compress/gzip. It deliberately excludes the .git/ tree — git history
// travels through the bundle adapter, never through tar.gz.
package tarsnapshot

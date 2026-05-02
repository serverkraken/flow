// Package kompendiumexec implements ports.KompendiumGateway by
// shelling out to the kompendium binary.
//
// The binary name is configurable per-instance (typically resolved from
// $KOMPENDIUM_BIN with a "kompendium" default by the composition root).
//
// In a future phase the kompendium codebase will be merged in-tree and
// this adapter will be replaced by a direct library call — the
// gateway-port interface stays the same so callers don't move.
package kompendiumexec

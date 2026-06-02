// Package fssourcedirs implements ports.SourceDirScanner by walking
// $SOURCECODE_ROOT (configurable per-instance) and returning every
// directory that contains a `.git` entry (regular Git repos and
// worktrees both qualify; worktrees use a `.git` file).
//
// Walks stop at depth 5 so a deeply nested node_modules tree doesn't
// dominate the scan time. The returned SourceDir.Name is the relative
// path from root, so nested repos like "owner/repo" round-trip
// cleanly into a tmux session name.
package fssourcedirs

// Package gitrepo implements ports.RepoDetector by shelling out to the git
// CLI. Production deployments rely on the `git` binary being on $PATH; tests
// drive the same binary against tempdir-init repositories.
package gitrepo

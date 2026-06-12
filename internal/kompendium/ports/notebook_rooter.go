package ports

// NotebookRooter exposes the filesystem root of the notebook. It is satisfied
// by adapters that have a local root directory, and left unimplemented
// by API-backed adapters which have no local filesystem path.
// Usecases that need the root (git, bundle, tar, remote, snapshot, doctor,
// init) accept this as a separate port rather than demanding it from NoteStore,
// so API-backed stores do not need to implement a meaningless method.
type NotebookRooter interface {
	Root() string
}

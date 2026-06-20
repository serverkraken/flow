package daydetail

// DialogOpen reports whether any modal (Nachbuchen add, edit, or delete confirm)
// is currently open. It exists for the external _test package to assert that a
// save error keeps the dialog open (and populated) so the user never loses typed
// input, and that a successful save / cancel closes it.
func (r *Route) DialogOpen() bool { return r.nachb != nil || r.edit != nil || r.del != nil }

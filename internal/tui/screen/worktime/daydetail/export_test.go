package daydetail

// DialogOpen reports whether the Nachbuchen (Add) dialog is currently open. It
// exists for the external _test package to assert that an AddSession error keeps
// the dialog open (and populated) so the user never loses typed input.
func (r *Route) DialogOpen() bool { return r.nachb != nil }

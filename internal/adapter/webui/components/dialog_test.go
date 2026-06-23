package components_test

import (
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/serverkraken/flow/internal/adapter/webui/components"
)

func TestDialogStructure(t *testing.T) {
	out := render(t, components.Dialog("editDlg", "common.edit", templ.Raw(`<p id="db">form</p>`)))
	for _, w := range []string{
		`id="editDlg"`, `aria-modal="true"`, `data-dialog-close`, `id="db"`,
		`/static/js/dialog.js`, "Bearbeiten",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("Dialog missing %q: %s", w, out)
		}
	}
}

func TestConfirmDialogDefaultsAndDanger(t *testing.T) {
	out := render(t, components.ConfirmDialog(components.ConfirmSpec{
		ID:           "delDlg",
		ConfirmAttrs: templ.Attributes{"hx-post": "/sessions/s1/delete"},
	}))
	for _, w := range []string{
		"Bist du sicher?",               // default title
		"kann nicht rückgängig",         // default body (confirm.deleteBody)
		"Abbrechen",                     // cancel
		"autofocus",                     // cancel focused by default (safe)
		"Löschen",                       // default confirm label
		"bg-red",                        // danger confirm button
		`hx-post="/sessions/s1/delete"`, // confirm action wired
	} {
		if !strings.Contains(out, w) {
			t.Errorf("ConfirmDialog missing %q: %s", w, out)
		}
	}
}

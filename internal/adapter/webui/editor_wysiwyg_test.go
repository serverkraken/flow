package webui

// Punkt (g) aus Soennes Review: "Wenn ein Editor mir Seiteninhalt darstellt,
// dann erwarte ich, dass er mir nach dem Speichern auch genau so angezeigt
// wird."
//
// Die Grundlage stimmt auf dieser Basis schon: Vorschau und Artikel laufen
// durch DENSELBEN Renderer (components.MarkdownProse → .prose). Was fehlte,
// war die Fläche drumherum — die Vorschau saß in einem gerundeten Kasten auf
// Kastenfarbe, der Artikel auf dem Lesesaal-Papier.

import (
	"context"
	"html/template"
	"strings"
	"testing"
)

// Beide Seiten rendern durch dieselbe Prosa-Hülle — sonst driften sie beim
// nächsten Stilwechsel wieder auseinander.
func TestEditorPreview_UsesTheSameProseAsTheArticle(t *testing.T) {
	html := template.HTML("<h2>Kopf</h2><p>Text</p>")
	preview := renderToBuf(t, context.Background(), MarkdownPreview(html))
	article := renderToBuf(t, context.Background(), DocumentFragment(DocumentVM{ID: "d1", Title: "T", HTML: html}))

	if !strings.Contains(preview, `class="prose"`) {
		t.Errorf("die Vorschau muss die Prosa-Hülle tragen: %s", preview)
	}
	if !strings.Contains(article, `class="prose"`) {
		t.Errorf("der Artikel muss die Prosa-Hülle tragen: %.300s", article)
	}
}

// Die Vorschau liegt auf der Lesefläche, nicht in einem Kasten — sonst ist
// das Versprechen "so wird es aussehen" schon an der Fläche gebrochen.
func TestEditorPreview_SitsOnTheReadingSurface(t *testing.T) {
	out := renderToBuf(t, context.Background(), EditorPage(EditorVM{ID: "d1", Title: "T"}))
	i := strings.Index(out, `id="preview"`)
	if i < 0 {
		t.Fatal("Vorschau-Bereich fehlt")
	}
	region := out[maxI(0, i-320):i]
	if strings.Contains(region, "panel") {
		t.Errorf("die Vorschau darf nicht im Kasten sitzen: %s", region)
	}
}

func maxI(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Rich Text ist Standard (eine Fläche), Markdown der Umschalter — erst dort
// Quelle und Vorschau nebeneinander. Blöcke und Auszeichnungen trägt der
// Stift selbst (Crepe); das Band gibt Bild, Verweis und den Modus dazu.
func TestEditor_RichTextIsDefaultWithBand(t *testing.T) {
	out := renderToBuf(t, context.Background(), EditorPage(EditorVM{ID: "d1", Title: "T"}))
	for _, want := range []string{
		`data-editor-body data-mode="rich"`,
		`data-editor-mode-set="rich" aria-pressed="true"`,
		`data-editor-mode-set="markdown" aria-pressed="false"`,
		`vendor/milkdown/editor.min.css`, `vendor/milkdown/editor.min.js`,
		`data-insert-toggle="insert-picker-artefakte"`, `data-insert-toggle="insert-picker-seiten"`,
		`data-preview-pane`, `id="preview"`, `js/editor-mode.js`,
		`class="field editor-title"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Editor ohne %s", want)
		}
	}
	if strings.Contains(out, "data-preview-toggle") {
		t.Errorf("der alte Vorschau-Schalter ist Geschichte")
	}
}

// Ein Klick in den Stift darf nichts anderes auslösen: Werkzeugleiste und
// Editor dürfen deshalb in keinem <label> stehen — ein Label reicht jeden
// Klick an sein erstes bedienbares Kind weiter, den Artefakt-Knopf.
func TestEditorBody_IsNotWrappedInALabel(t *testing.T) {
	out := renderToBuf(t, context.Background(), EditorPage(EditorVM{ID: "d1", Title: "T"}))
	i := strings.Index(out, `data-editor-field`)
	j := strings.Index(out, `id="editor-body"`)
	if i < 0 || j < 0 || j < i {
		t.Fatalf("Editor-Feld fehlt: %d %d", i, j)
	}
	between := out[i:j]
	if strings.Contains(between, "<label") {
		t.Errorf("zwischen Feldkopf und Stift darf kein <label> liegen:\n%s", between)
	}
	// … und das Feld selbst ist kein Label.
	if strings.Contains(out[maxI(0, i-40):i], "<label") {
		t.Errorf("das Editor-Feld ist ein <label>: %s", out[maxI(0, i-40):i+20])
	}
}

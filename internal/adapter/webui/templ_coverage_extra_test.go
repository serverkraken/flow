package webui

import (
	"context"
	"testing"
)

// TestExportIsBuffer_Coverage exercises export_templ.go inner functions.
// All use ExportPageData; zero value is safe (no pointer dereferences).
func TestExportIsBuffer_Coverage(t *testing.T) {
	ctx := context.Background()
	d := ExportPageData{}
	renderComp(t, ctx, exportBody(d))
	renderComp(t, ctx, exportContent(d))
	renderComp(t, ctx, exportCardBody(d))
	renderComp(t, ctx, exportSummaryTable(d))
}

// TestDocumentIsBuffer_Coverage exercises document_templ.go inner functions.
func TestDocumentIsBuffer_Coverage(t *testing.T) {
	ctx := context.Background()
	vm := DocumentVM{}
	renderComp(t, ctx, documentBody(vm))
	renderComp(t, ctx, documentBreadcrumb(vm))
	renderComp(t, ctx, documentOuter(vm))
}

// TestEditorTemplIsBuffer_Coverage exercises editor_templ.go inner functions.
func TestEditorTemplIsBuffer_Coverage(t *testing.T) {
	ctx := context.Background()
	vm := EditorVM{}
	renderComp(t, ctx, editorBody(vm))
	renderComp(t, ctx, editorBreadcrumb(vm))
	renderComp(t, ctx, editorOuter(vm))
}

// TestEinstellungenIsBuffer_Coverage exercises einstellungen_templ.go inner functions.
func TestEinstellungenIsBuffer_Coverage(t *testing.T) {
	ctx := context.Background()
	vm := EinstellungenVM{}
	renderComp(t, ctx, einstellungenBody(vm))
	renderComp(t, ctx, einstellungenContent(vm))
	renderComp(t, ctx, einstellungenTargetBody(vm))
}

// TestSaldoIsBuffer_Coverage exercises saldo_templ.go inner function.
func TestSaldoIsBuffer_Coverage(t *testing.T) {
	ctx := context.Background()
	renderComp(t, ctx, statsSaldoTile("stats.saldo", "+2h", true, ""))
}

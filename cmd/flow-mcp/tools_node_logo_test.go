package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/usecase"
)

// fakeLogoBackend serves the endpoints flow_set_node_logo touches. p1 (slug
// "alpha") accepts the upload and echoes the node with a stamped LogoRef;
// any other node 404s, mirroring the server-side ownership guard.
func fakeLogoBackend(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Node{{ID: "p1", Name: "Alpha", Slug: "alpha"}})
	})
	mux.HandleFunc("PUT /api/v1/nodes/{id}/logo", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != "p1" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var in struct {
			DataBase64 string `json:"dataBase64"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if _, err := base64.StdEncoding.DecodeString(in.DataBase64); err != nil {
			http.Error(w, "bad base64", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(domain.Node{ID: "p1", Name: "Alpha", Slug: "alpha", LogoRef: "abc123def456"})
	})
	return httptest.NewServer(mux)
}

func authedLogoServer(t *testing.T) *mcp.ClientSession {
	t.Helper()
	be := fakeLogoBackend(t)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	proj := domain.Node{ID: "p1", Name: "Alpha", Slug: "alpha"}
	mgr, h := managerFor(t, client, proj)
	_ = mgr
	return connect(t, h.srv)
}

// logoPixelPNG is a 1x1 PNG — small but sniffs as image/png.
func logoPixelPNG(t *testing.T) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
	)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLoopback_SetNodeLogo_Advertised(t *testing.T) {
	cs := authedLogoServer(t)
	res, err := cs.ListTools(t.Context(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if tool.Name == "flow_set_node_logo" {
			return
		}
	}
	t.Fatal("flow_set_node_logo not advertised")
}

func TestLoopback_SetNodeLogo_PathMode(t *testing.T) {
	cs := authedLogoServer(t)
	p := filepath.Join(t.TempDir(), "logo.png")
	if err := os.WriteFile(p, logoPixelPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "flow_set_node_logo",
		Arguments: map[string]any{"path": p},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}
	text := text(res)
	if !strings.Contains(text, "Set logo") || !strings.Contains(text, "abc123def456") {
		t.Errorf("text = %q, want Set logo … ref abc123def456", text)
	}
}

func TestLoopback_SetNodeLogo_RequiresExactlyOneSource(t *testing.T) {
	cs := authedLogoServer(t)
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "flow_set_node_logo",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("want error for missing source")
	}
	if text := text(res); !strings.Contains(text, "exactly one of path or base64") {
		t.Errorf("text = %q, want exactly-one error", text)
	}
}

func TestLoopback_SetNodeLogo_OversizeRejectedClientSide(t *testing.T) {
	cs := authedLogoServer(t)
	big := make([]byte, usecase.MaxNodeLogoBytes+1)
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "flow_set_node_logo",
		Arguments: map[string]any{"base64": base64.StdEncoding.EncodeToString(big)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("want error for oversized logo")
	}
	if text := text(res); !strings.Contains(text, "512 KiB") {
		t.Errorf("text = %q, want 512 KiB size error", text)
	}
}

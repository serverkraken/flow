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

// fakeBannerBackend serves the endpoints flow_set_node_banner touches, the
// twin of fakeLogoBackend.
func fakeBannerBackend(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/me", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(domain.User{ID: "u1", DisplayName: "Dev", Email: "dev@x"})
	})
	mux.HandleFunc("GET /api/v1/nodes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]domain.Node{{ID: "p1", Name: "Alpha", Slug: "alpha"}})
	})
	mux.HandleFunc("PUT /api/v1/nodes/{id}/banner", func(w http.ResponseWriter, r *http.Request) {
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
		_ = json.NewEncoder(w).Encode(domain.Node{ID: "p1", Name: "Alpha", Slug: "alpha", BannerRef: "bbb111ccc222"})
	})
	return httptest.NewServer(mux)
}

func authedBannerServer(t *testing.T) *mcp.ClientSession {
	t.Helper()
	be := fakeBannerBackend(t)
	t.Cleanup(be.Close)
	client := apiclient.New(be.URL, "tok")
	proj := domain.Node{ID: "p1", Name: "Alpha", Slug: "alpha"}
	mgr, h := managerFor(t, client, proj)
	_ = mgr
	return connect(t, h.srv)
}

func TestLoopback_SetNodeBanner_Advertised(t *testing.T) {
	cs := authedBannerServer(t)
	res, err := cs.ListTools(t.Context(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if tool.Name == "flow_set_node_banner" {
			return
		}
	}
	t.Fatal("flow_set_node_banner not advertised")
}

func TestLoopback_SetNodeBanner_PathMode(t *testing.T) {
	cs := authedBannerServer(t)
	p := filepath.Join(t.TempDir(), "banner.png")
	if err := os.WriteFile(p, logoPixelPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "flow_set_node_banner",
		Arguments: map[string]any{"path": p},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %v", res.Content)
	}
	if text := text(res); !strings.Contains(text, "Set banner") || !strings.Contains(text, "bbb111ccc222") {
		t.Errorf("text = %q, want Set banner … ref bbb111ccc222", text)
	}
}

func TestLoopback_SetNodeBanner_RequiresExactlyOneSource(t *testing.T) {
	cs := authedBannerServer(t)
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "flow_set_node_banner",
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

// TestLoopback_SetNodeBanner_OversizeRejectedClientSide pins that the banner's
// cap is its own, not the logo's: an image between the two limits must pass
// the client-side guard rather than be rejected as "too large".
func TestLoopback_SetNodeBanner_OversizeRejectedClientSide(t *testing.T) {
	cs := authedBannerServer(t)
	big := make([]byte, usecase.MaxNodeBannerBytes+1)
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "flow_set_node_banner",
		Arguments: map[string]any{"base64": base64.StdEncoding.EncodeToString(big)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("want error for oversized upload")
	}
	if text := text(res); !strings.Contains(text, usecase.ErrBannerTooLarge.Error()) {
		t.Errorf("text = %q, want the banner size error", text)
	}
	if usecase.MaxNodeBannerBytes <= usecase.MaxNodeLogoBytes {
		t.Fatalf("banner cap %d must exceed the logo cap %d", usecase.MaxNodeBannerBytes, usecase.MaxNodeLogoBytes)
	}
}

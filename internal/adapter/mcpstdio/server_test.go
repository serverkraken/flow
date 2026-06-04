package mcpstdio_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/adapter/mcpstdio"
)

// fakeImpl is a minimal Implementation that records calls so tests can
// drive the protocol surface without dragging the usecase layer in.
type fakeImpl struct {
	tools       []mcpstdio.Tool
	resources   []mcpstdio.Resource
	lastCall    string
	lastArgs    map[string]any
	callResult  mcpstdio.CallToolResult
	readErr     error
	readContent mcpstdio.ResourceContent
}

func (f *fakeImpl) Tools() []mcpstdio.Tool         { return f.tools }
func (f *fakeImpl) Resources() []mcpstdio.Resource { return f.resources }
func (f *fakeImpl) CallTool(name string, args map[string]any) mcpstdio.CallToolResult {
	f.lastCall = name
	f.lastArgs = args
	return f.callResult
}

func (f *fakeImpl) ReadResource(uri string) (mcpstdio.ResourceContent, error) {
	if f.readErr != nil {
		return mcpstdio.ResourceContent{}, f.readErr
	}
	f.readContent.URI = uri
	return f.readContent, nil
}

// drive serves a single request/response cycle through Server.Serve.
// Returns the decoded response or nil if the server didn't write
// anything (notification path).
func drive(t *testing.T, impl mcpstdio.Implementation, reqJSON string) *mcpstdio.Response {
	t.Helper()
	server := mcpstdio.NewServer(impl, mcpstdio.ServerInfo{Name: "test", Version: "0.0.0"}, nil)
	in := strings.NewReader(reqJSON)
	var out bytes.Buffer
	if err := server.Serve(in, &out); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("Serve: %v", err)
	}
	if out.Len() == 0 {
		return nil
	}
	var resp mcpstdio.Response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (body=%q)", err, out.String())
	}
	return &resp
}

func TestServer_Initialize(t *testing.T) {
	impl := &fakeImpl{}
	resp := drive(t, impl, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	if resp == nil {
		t.Fatal("expected a response for initialize")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	// Round-trip via JSON to inspect the result map.
	raw, _ := json.Marshal(resp.Result)
	var got mcpstdio.InitializeResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if got.ProtocolVersion != mcpstdio.ProtocolVersion {
		t.Fatalf("protocolVersion = %q, want %q", got.ProtocolVersion, mcpstdio.ProtocolVersion)
	}
	if got.ServerInfo.Name != "test" {
		t.Fatalf("serverInfo.name = %q, want test", got.ServerInfo.Name)
	}
	if _, ok := got.Capabilities["tools"]; !ok {
		t.Fatalf("missing tools capability: %+v", got.Capabilities)
	}
}

func TestServer_InitializedNotification_NoResponse(t *testing.T) {
	impl := &fakeImpl{}
	resp := drive(t, impl, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if resp != nil {
		t.Fatalf("expected no response for notification, got %+v", resp)
	}
}

func TestServer_ToolsList(t *testing.T) {
	impl := &fakeImpl{tools: []mcpstdio.Tool{{Name: "demo", Description: "d", InputSchema: map[string]any{"type": "object"}}}}
	resp := drive(t, impl, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if resp == nil || resp.Error != nil {
		t.Fatalf("tools/list failed: %+v", resp)
	}
	raw, _ := json.Marshal(resp.Result)
	var got mcpstdio.ToolsListResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Tools) != 1 || got.Tools[0].Name != "demo" {
		t.Fatalf("got %+v", got.Tools)
	}
}

func TestServer_ToolsCall(t *testing.T) {
	impl := &fakeImpl{
		callResult: mcpstdio.CallToolResult{
			Content: []mcpstdio.Content{{Type: "text", Text: "ok"}},
		},
	}
	body := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"demo","arguments":{"k":"v"}}}`
	resp := drive(t, impl, body)
	if resp == nil || resp.Error != nil {
		t.Fatalf("tools/call failed: %+v", resp)
	}
	if impl.lastCall != "demo" {
		t.Fatalf("lastCall = %q, want demo", impl.lastCall)
	}
	if impl.lastArgs["k"] != "v" {
		t.Fatalf("lastArgs = %+v", impl.lastArgs)
	}
}

func TestServer_ResourcesList(t *testing.T) {
	impl := &fakeImpl{resources: []mcpstdio.Resource{{URI: "flow://x", Name: "n"}}}
	resp := drive(t, impl, `{"jsonrpc":"2.0","id":4,"method":"resources/list"}`)
	if resp == nil || resp.Error != nil {
		t.Fatalf("resources/list failed: %+v", resp)
	}
	raw, _ := json.Marshal(resp.Result)
	var got mcpstdio.ResourcesListResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Resources) != 1 || got.Resources[0].URI != "flow://x" {
		t.Fatalf("got %+v", got.Resources)
	}
}

func TestServer_ResourcesRead(t *testing.T) {
	impl := &fakeImpl{
		readContent: mcpstdio.ResourceContent{MimeType: "text/markdown", Text: "hello"},
	}
	body := `{"jsonrpc":"2.0","id":5,"method":"resources/read","params":{"uri":"flow://x"}}`
	resp := drive(t, impl, body)
	if resp == nil || resp.Error != nil {
		t.Fatalf("resources/read failed: %+v", resp)
	}
	raw, _ := json.Marshal(resp.Result)
	var got mcpstdio.ReadResourceResult
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Contents) != 1 || got.Contents[0].Text != "hello" || got.Contents[0].URI != "flow://x" {
		t.Fatalf("got %+v", got.Contents)
	}
}

func TestServer_ResourcesRead_Error(t *testing.T) {
	impl := &fakeImpl{readErr: errors.New("nope")}
	body := `{"jsonrpc":"2.0","id":6,"method":"resources/read","params":{"uri":"flow://x"}}`
	resp := drive(t, impl, body)
	if resp == nil || resp.Error == nil {
		t.Fatalf("expected JSON-RPC error, got %+v", resp)
	}
	if resp.Error.Code != mcpstdio.ErrCodeInvalidParams {
		t.Fatalf("error code = %d, want %d", resp.Error.Code, mcpstdio.ErrCodeInvalidParams)
	}
}

func TestServer_UnknownMethod(t *testing.T) {
	impl := &fakeImpl{}
	resp := drive(t, impl, `{"jsonrpc":"2.0","id":7,"method":"nope"}`)
	if resp == nil || resp.Error == nil {
		t.Fatalf("expected JSON-RPC error, got %+v", resp)
	}
	if resp.Error.Code != mcpstdio.ErrCodeMethodNotFound {
		t.Fatalf("error code = %d, want %d", resp.Error.Code, mcpstdio.ErrCodeMethodNotFound)
	}
}

func TestServer_Ping(t *testing.T) {
	impl := &fakeImpl{}
	resp := drive(t, impl, `{"jsonrpc":"2.0","id":8,"method":"ping"}`)
	if resp == nil || resp.Error != nil {
		t.Fatalf("ping failed: %+v", resp)
	}
}

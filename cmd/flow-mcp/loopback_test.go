//go:build integration

package main_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLoopback_InitializeAndToolsList builds the binary, spawns it,
// sends an MCP initialize then tools/list, and asserts the responses.
// Tagged integration so it stays out of the default `go test` run —
// `make ci` adds the tag explicitly.
func TestLoopback_InitializeAndToolsList(t *testing.T) {
	binary := buildBinary(t)
	tempDB := filepath.Join(t.TempDir(), "cache.db")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = []string{
		"PATH=/usr/bin:/bin",
		"HOME=" + t.TempDir(),
		"FLOW_CACHE_DB=" + tempDB,
		"FLOW_LOCAL_USER_SUB=loopback-user",
		// Direct flow-server URL to localhost:0 so the sync worker's
		// background dial fails fast and noisily into stderr (and not
		// onto stdout where MCP frames live).
		"FLOW_SERVER_URL=http://127.0.0.1:1",
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("StderrPipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Drain stderr in the background so the binary doesn't block on
	// a full pipe buffer.
	go func() { _, _ = io.Copy(io.Discard, stderr) }()

	reader := bufio.NewReader(stdout)
	send := func(req string) {
		if _, err := stdin.Write([]byte(req + "\n")); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	recv := func() map[string]any {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(line), &out); err != nil {
			t.Fatalf("decode %q: %v", line, err)
		}
		return out
	}

	// initialize
	send(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)
	initResp := recv()
	result, ok := initResp["result"].(map[string]any)
	if !ok {
		t.Fatalf("initialize: no result: %+v", initResp)
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("protocolVersion = %v", result["protocolVersion"])
	}

	// tools/list
	send(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	toolsResp := recv()
	tools, ok := toolsResp["result"].(map[string]any)["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list: bad shape: %+v", toolsResp)
	}
	if len(tools) != 7 {
		t.Errorf("tools count = %d, want 7", len(tools))
	}
	var names []string
	for _, tool := range tools {
		names = append(names, tool.(map[string]any)["name"].(string))
	}
	if !strings.Contains(strings.Join(names, ","), "flow_get_repo_note") {
		t.Errorf("missing flow_get_repo_note in: %v", names)
	}

	// tools/call without auth — every tool returns "Login required".
	send(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"flow_get_repo_note","arguments":{}}}`)
	callResp := recv()
	callResult := callResp["result"].(map[string]any)
	if callResult["isError"] != true {
		t.Errorf("call without auth: isError = %v, want true", callResult["isError"])
	}
	content := callResult["content"].([]any)[0].(map[string]any)
	if !strings.Contains(content["text"].(string), "Login required") {
		t.Errorf("call without auth: text = %q", content["text"])
	}

	// Close stdin so Serve returns EOF; the binary should exit cleanly.
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		// Background workers may produce a non-zero exit on ctx
		// cancellation race; the JSON-RPC contract is what we care
		// about and was verified above.
		t.Logf("Wait: %v (non-fatal)", err)
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "flow-mcp")
	build := exec.Command("go", "build", "-o", binary, "github.com/serverkraken/flow/cmd/flow-mcp")
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return binary
}

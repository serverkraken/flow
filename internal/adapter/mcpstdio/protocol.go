package mcpstdio

import "encoding/json"

// JSON-RPC 2.0 envelopes plus the MCP subset used by tools/resources.

// Request is a JSON-RPC 2.0 request envelope. ID is absent (nil) for
// notifications; the server must not send a response for those.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response envelope. Exactly one of Result or
// Error is non-nil per spec; the encoder enforces this via the
// omitempty tags and the caller building only one branch.
type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

// Error is the JSON-RPC 2.0 error object.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Standard JSON-RPC error codes used by this package. The MCP spec adds
// more codes but we only need these for the methods we serve.
const (
	ErrCodeParse          = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternal       = -32603
)

// ProtocolVersion is the MCP protocol version this server advertises
// during initialize. Bump when upgrading to a newer spec revision.
const ProtocolVersion = "2024-11-05"

// InitializeResult is the response payload for the initialize method.
type InitializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      ServerInfo     `json:"serverInfo"`
}

// ServerInfo identifies the implementation to the client.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Tool describes a tool advertised via tools/list.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ToolsListResult is the response payload for tools/list.
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// CallToolRequest is the params payload for tools/call.
type CallToolRequest struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// CallToolResult is the response payload for tools/call. IsError=true
// carries a tool-level (recoverable, surfaced to the user) error; a
// JSON-RPC error envelope signals protocol/transport bugs instead.
type CallToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Content is a single content block returned to the client. We only
// emit text blocks; image/audio/resource blocks are spec'd but unused.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Resource describes a resource advertised via resources/list.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourcesListResult is the response payload for resources/list.
type ResourcesListResult struct {
	Resources []Resource `json:"resources"`
}

// ReadResourceRequest is the params payload for resources/read.
type ReadResourceRequest struct {
	URI string `json:"uri"`
}

// ReadResourceResult is the response payload for resources/read. The
// MCP spec wraps each item with the URI it came from so a single read
// can return multiple sub-documents; we always return exactly one.
type ReadResourceResult struct {
	Contents []ResourceContent `json:"contents"`
}

// ResourceContent pairs a resource URI with its content payload.
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

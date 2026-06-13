package mcpstdio

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
)

// Implementation is the dispatch surface every Server delegates to. The
// usecase layer provides a thin in-process implementation; the adapter
// layer never knows about RepoNotes / Sessions / etc.
type Implementation interface {
	// Tools returns the static tool catalog. Called once per tools/list.
	Tools() []Tool
	// Resources returns the dynamic resource catalog. Called once per
	// resources/list. May change between calls (new repos appear).
	Resources() []Resource
	// CallTool dispatches a tool by name. Errors here surface as MCP
	// tool errors (IsError=true content), not JSON-RPC errors — the
	// MCP client shows the message to the user and continues. Use a
	// JSON-RPC error only for transport/protocol bugs.
	CallTool(name string, args map[string]any) CallToolResult
	// ReadResource returns the content of a single resource URI. Errors
	// here become JSON-RPC errors so the client can distinguish "I
	// don't know that URI" from "the resource is empty".
	ReadResource(uri string) (ResourceContent, error)
}

// Server is a single stdio MCP server instance. Construct via NewServer
// and drive with Serve.
type Server struct {
	impl Implementation
	log  *slog.Logger
	info ServerInfo
}

// NewServer wires impl and the slog target. logger may be nil — in
// that case the server discards diagnostic logs (the MCP transport
// itself never touches the logger).
func NewServer(impl Implementation, info ServerInfo, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Server{impl: impl, info: info, log: logger}
}

// Serve reads newline-delimited JSON-RPC from r and writes responses to
// w. Returns nil on clean EOF, the underlying error otherwise. The
// loop is single-threaded per the MCP stdio model — each request is
// dispatched in-line before the next is decoded.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	dec := json.NewDecoder(r)
	enc := json.NewEncoder(w)
	for {
		var req Request
		if err := dec.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		resp, send := s.dispatch(req)
		if !send {
			continue
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
}

// dispatch resolves the method, populates the response, and reports
// whether a response should be sent (false for notifications).
func (s *Server) dispatch(req Request) (Response, bool) {
	resp := Response{JSONRPC: "2.0", ID: req.ID}

	// Per JSON-RPC: requests without an "id" field are notifications
	// and must not be answered.
	notification := req.ID == nil

	switch req.Method {
	case "initialize":
		resp.Result = InitializeResult{
			ProtocolVersion: ProtocolVersion,
			Capabilities: map[string]any{
				"tools":     map[string]any{"listChanged": false},
				"resources": map[string]any{"listChanged": false, "subscribe": false},
			},
			ServerInfo: s.info,
		}
	case "initialized", "notifications/initialized":
		// Client tells us setup is done. No response.
		return resp, false
	case "tools/list":
		resp.Result = ToolsListResult{Tools: s.impl.Tools()}
	case "tools/call":
		var call CallToolRequest
		if err := json.Unmarshal(req.Params, &call); err != nil {
			resp.Error = &Error{Code: ErrCodeInvalidParams, Message: err.Error()}
			break
		}
		resp.Result = s.impl.CallTool(call.Name, call.Arguments)
	case "resources/list":
		resp.Result = ResourcesListResult{Resources: s.impl.Resources()}
	case "resources/read":
		var rr ReadResourceRequest
		if err := json.Unmarshal(req.Params, &rr); err != nil {
			resp.Error = &Error{Code: ErrCodeInvalidParams, Message: err.Error()}
			break
		}
		content, err := s.impl.ReadResource(rr.URI)
		if err != nil {
			resp.Error = &Error{Code: ErrCodeInvalidParams, Message: err.Error()}
			break
		}
		resp.Result = ReadResourceResult{Contents: []ResourceContent{content}}
	case "ping":
		// MCP spec: ping returns an empty result.
		resp.Result = struct{}{}
	default:
		if notification {
			// Spec: unknown notifications are silently ignored.
			return resp, false
		}
		resp.Error = &Error{Code: ErrCodeMethodNotFound, Message: "method not found: " + req.Method}
	}

	if notification {
		return resp, false
	}
	return resp, true
}

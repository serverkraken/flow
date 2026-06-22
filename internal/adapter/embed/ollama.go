// Package embed provides the Ollama implementation of ports.Embedder.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

// compile-time assertion
var _ ports.Embedder = (*Ollama)(nil)

// Ollama calls a local Ollama server's /api/embed endpoint.
type Ollama struct {
	host   string
	model  string
	client *http.Client
}

// NewOllama returns an Ollama embedder. Empty host/model fall back to the
// localhost default and nomic-embed-text.
func NewOllama(host, model string) *Ollama {
	if host == "" {
		host = "http://localhost:11434"
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	return &Ollama{
		host:   strings.TrimRight(host, "/"),
		model:  model,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

type embedReq struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResp struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// maxInputsPerCall caps how many texts go in one /api/embed request so a large
// document never grows a single call past the client timeout.
const maxInputsPerCall = 64

// Embed implements ports.Embedder.
func (o *Ollama) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += maxInputsPerCall {
		end := start + maxInputsPerCall
		if end > len(texts) {
			end = len(texts)
		}
		vecs, err := o.embedOnce(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	if len(out) != len(texts) {
		return nil, fmt.Errorf("ollama embed: got %d vectors for %d texts", len(out), len(texts))
	}
	return out, nil
}

// isTransientStatus reports whether an Ollama HTTP status means the backend is
// unavailable or misconfigured (retry later) rather than this input being bad.
// 404 = model not found — a server-config problem that fails every document.
func isTransientStatus(code int) bool {
	return code == http.StatusServiceUnavailable ||
		code == http.StatusTooManyRequests ||
		code == http.StatusNotFound
}

func (o *Ollama) embedOnce(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(embedReq{Model: o.model, Input: texts})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.host+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		// dial failure / connection refused / reset / client timeout — environmental.
		return nil, fmt.Errorf("ollama embed: %w: %w", ports.ErrEmbedTransient, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
		if isTransientStatus(resp.StatusCode) {
			return nil, fmt.Errorf("ollama embed: %w: status %d: %s", ports.ErrEmbedTransient, resp.StatusCode, strings.TrimSpace(string(b)))
		}
		return nil, fmt.Errorf("ollama embed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var er embedResp
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		return nil, fmt.Errorf("ollama embed decode: %w", err)
	}
	if len(er.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama embed: got %d vectors for %d texts", len(er.Embeddings), len(texts))
	}
	return er.Embeddings, nil
}

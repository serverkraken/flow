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

// Embed implements ports.Embedder.
func (o *Ollama) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
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
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
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

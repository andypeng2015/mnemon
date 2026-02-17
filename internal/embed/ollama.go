package embed

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

// DefaultModel is the default Ollama embedding model.
const DefaultModel = "nomic-embed-text"

// DefaultEndpoint is the default Ollama API endpoint.
const DefaultEndpoint = "http://localhost:11434"

// Client communicates with an Ollama instance for embedding generation.
type Client struct {
	endpoint string
	model    string
	http     *http.Client
}

// NewClient creates an Ollama embedding client.
// It checks MNEMON_EMBED_ENDPOINT and MNEMON_EMBED_MODEL env vars.
func NewClient() *Client {
	endpoint := os.Getenv("MNEMON_EMBED_ENDPOINT")
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	model := os.Getenv("MNEMON_EMBED_MODEL")
	if model == "" {
		model = DefaultModel
	}
	return &Client{
		endpoint: endpoint,
		model:    model,
		http: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				// Bypass system proxy for localhost Ollama connections.
				Proxy: nil,
				DialContext: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
			},
		},
	}
}

// Available returns true if the Ollama server is reachable and the model is loaded.
func (c *Client) Available() bool {
	resp, err := c.http.Get(c.endpoint + "/api/tags")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Model returns the configured model name.
func (c *Client) Model() string {
	return c.model
}

type embedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embedResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

// Embed generates an embedding vector for the given text.
func (c *Client) Embed(text string) ([]float64, error) {
	body, err := json.Marshal(embedRequest{Model: c.model, Input: text})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.http.Post(c.endpoint+"/api/embed", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var result embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Embeddings) == 0 || len(result.Embeddings[0]) == 0 {
		return nil, fmt.Errorf("empty embedding returned")
	}

	return result.Embeddings[0], nil
}

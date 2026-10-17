// Package ai is REA's provider-independent AI layer. Every call asks the model for JSON that
// matches a schema, cites numbered evidence items, and is validated before it is stored. AI is
// optional: nothing outside this package depends on a provider being configured.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// Image is an optional image input for vision-capable providers.
type Image struct {
	MediaType string
	Data      []byte
}

// Request is a single structured-output completion.
type Request struct {
	System     string
	Prompt     string
	Images     []Image
	Schema     map[string]any
	SchemaName string
}

// Response is the provider's answer.
type Response struct {
	JSON         json.RawMessage
	Text         string
	Model        string
	InputTokens  int
	OutputTokens int
}

// Provider performs completions.
type Provider interface {
	Kind() string
	Complete(ctx context.Context, req Request) (Response, error)
}

// Config describes a configured provider.
type Config struct {
	Kind        string
	BaseURL     string
	Model       string
	APIKey      string
	Temperature float64
	MaxTokens   int
	Timeout     time.Duration
	HTTPClient  *http.Client
}

// Kinds lists the supported provider kinds.
var Kinds = []string{"ollama", "openai", "openai_compatible", "anthropic"}

// DefaultBaseURL returns the usual endpoint for a kind.
func DefaultBaseURL(kind string) string {
	switch kind {
	case "ollama":
		return "http://localhost:11434"
	case "openai":
		return "https://api.openai.com/v1"
	case "anthropic":
		return "https://api.anthropic.com"
	}
	return ""
}

// SuggestedModels are pre-filled in the provider form. Operators can enter any model their
// endpoint serves.
var SuggestedModels = map[string][]string{
	"ollama":            {"llama3.3", "qwen3", "mistral-small3.2", "llava"},
	"openai":            {"gpt-5", "gpt-5-mini"},
	"openai_compatible": {},
	"anthropic":         {"claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-5-5"},
}

// ErrProvider wraps failures reported by a provider.
type ErrProvider struct {
	Status    int
	Message   string
	Retryable bool
}

func (e *ErrProvider) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("provider returned HTTP %d: %s", e.Status, e.Message)
	}
	return "provider error: " + e.Message
}

// New builds a provider from configuration.
func New(cfg Config) (Provider, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL(cfg.Kind)
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Timeout == 0 {
		cfg.Timeout = 2 * time.Minute
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: cfg.Timeout}
	}
	if cfg.Model == "" {
		return nil, errors.New("a model name is required")
	}
	switch cfg.Kind {
	case "ollama":
		return &ollama{cfg: cfg}, nil
	case "openai", "openai_compatible":
		if cfg.Kind == "openai" && cfg.APIKey == "" {
			return nil, errors.New("the OpenAI provider needs an API key")
		}
		return &openAI{cfg: cfg}, nil
	case "anthropic":
		if cfg.APIKey == "" {
			return nil, errors.New("the Anthropic provider needs an API key")
		}
		return newAnthropic(cfg), nil
	}
	return nil, fmt.Errorf("unknown provider kind %q", cfg.Kind)
}

// IsLocalURL reports whether an endpoint is on this machine or a private network. Requests to
// such endpoints keep data inside the operator's infrastructure.
func IsLocalURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".lan") || host == "host.docker.internal" {
		return true
	}
	if a, err := netip.ParseAddr(host); err == nil {
		a = a.Unmap()
		return a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast()
	}
	// Single-label names (for example a Docker Compose service called "ollama") are local.
	return host != "" && !strings.Contains(host, ".") && net.ParseIP(host) == nil
}

// postJSON sends a JSON request and decodes a JSON response, mapping HTTP errors.
func postJSON(ctx context.Context, client *http.Client, endpoint string, headers map[string]string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return &ErrProvider{Message: "could not reach the provider: " + redactURL(err.Error()), Retryable: true}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return &ErrProvider{Message: "could not read the provider response", Retryable: true}
	}
	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		var e struct {
			Error any `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error != nil {
			switch t := e.Error.(type) {
			case string:
				msg = t
			case map[string]any:
				if m, ok := t["message"].(string); ok {
					msg = m
				}
			}
		}
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return &ErrProvider{Status: resp.StatusCode, Message: msg, Retryable: resp.StatusCode == 429 || resp.StatusCode >= 500}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &ErrProvider{Message: "the provider returned malformed JSON"}
	}
	return nil
}

func redactURL(s string) string {
	if i := strings.Index(s, "?"); i >= 0 {
		if j := strings.IndexAny(s[i:], " \""); j > 0 {
			return s[:i] + s[i+j:]
		}
		return s[:i]
	}
	return s
}

// ExtractJSON pulls a JSON object out of model text, tolerating code fences and preambles.
func ExtractJSON(text string) (json.RawMessage, error) {
	t := strings.TrimSpace(text)
	if strings.HasPrefix(t, "```") {
		t = strings.TrimPrefix(t, "```json")
		t = strings.TrimPrefix(t, "```")
		if i := strings.LastIndex(t, "```"); i >= 0 {
			t = t[:i]
		}
		t = strings.TrimSpace(t)
	}
	if json.Valid([]byte(t)) && strings.HasPrefix(t, "{") {
		return json.RawMessage(t), nil
	}
	start, end := strings.Index(t, "{"), strings.LastIndex(t, "}")
	if start >= 0 && end > start && json.Valid([]byte(t[start:end+1])) {
		return json.RawMessage(t[start : end+1]), nil
	}
	return nil, &ErrProvider{Message: "the model did not return a JSON object", Retryable: true}
}

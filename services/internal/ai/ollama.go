package ai

import (
	"context"
	"encoding/base64"
)

// ollama talks to a local Ollama server (POST /api/chat) and uses its JSON-schema "format".
type ollama struct{ cfg Config }

func (o *ollama) Kind() string { return "ollama" }

func (o *ollama) Complete(ctx context.Context, req Request) (Response, error) {
	user := map[string]any{"role": "user", "content": req.Prompt}
	if len(req.Images) > 0 {
		var imgs []string
		for _, im := range req.Images {
			imgs = append(imgs, base64.StdEncoding.EncodeToString(im.Data))
		}
		user["images"] = imgs
	}
	body := map[string]any{
		"model":    o.cfg.Model,
		"stream":   false,
		"messages": []any{map[string]any{"role": "system", "content": req.System}, user},
		"options":  map[string]any{"temperature": o.cfg.Temperature, "num_predict": o.cfg.MaxTokens},
	}
	if req.Schema != nil {
		body["format"] = req.Schema
	}
	var out struct {
		Model   string `json:"model"`
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		PromptEvalCount int `json:"prompt_eval_count"`
		EvalCount       int `json:"eval_count"`
	}
	if err := postJSON(ctx, o.cfg.HTTPClient, o.cfg.BaseURL+"/api/chat", nil, body, &out); err != nil {
		return Response{}, err
	}
	res := Response{Text: out.Message.Content, Model: firstNonEmpty(out.Model, o.cfg.Model), InputTokens: out.PromptEvalCount, OutputTokens: out.EvalCount}
	if req.Schema != nil {
		js, err := ExtractJSON(out.Message.Content)
		if err != nil {
			return res, err
		}
		res.JSON = js
	}
	return res, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

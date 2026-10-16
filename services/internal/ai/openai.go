package ai

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
)

// openAI covers the OpenAI API and OpenAI-compatible servers (LM Studio, vLLM, llama.cpp,
// LocalAI and hosted gateways) through POST /chat/completions.
type openAI struct{ cfg Config }

func (o *openAI) Kind() string { return o.cfg.Kind }

func (o *openAI) Complete(ctx context.Context, req Request) (Response, error) {
	var userContent any = req.Prompt
	if len(req.Images) > 0 {
		parts := []any{map[string]any{"type": "text", "text": req.Prompt}}
		for _, im := range req.Images {
			parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{
				"url": "data:" + im.MediaType + ";base64," + base64.StdEncoding.EncodeToString(im.Data)}})
		}
		userContent = parts
	}
	body := map[string]any{
		"model":    o.cfg.Model,
		"messages": []any{map[string]any{"role": "system", "content": req.System}, map[string]any{"role": "user", "content": userContent}},
	}
	if o.cfg.Kind == "openai" {
		body["max_completion_tokens"] = o.cfg.MaxTokens
	} else {
		body["max_tokens"] = o.cfg.MaxTokens
		body["temperature"] = o.cfg.Temperature
	}
	if req.Schema != nil {
		name := firstNonEmpty(req.SchemaName, "rea_output")
		body["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": name, "schema": req.Schema}}
	}
	headers := map[string]string{}
	if o.cfg.APIKey != "" {
		headers["Authorization"] = "Bearer " + o.cfg.APIKey
	}
	var out struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
				Refusal string `json:"refusal"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	err := postJSON(ctx, o.cfg.HTTPClient, o.cfg.BaseURL+"/chat/completions", headers, body, &out)
	var pe *ErrProvider
	if errors.As(err, &pe) && pe.Status == 400 && req.Schema != nil && strings.Contains(strings.ToLower(pe.Message), "response_format") {
		// Older compatible servers only support JSON mode; the schema is still in the prompt.
		body["response_format"] = map[string]any{"type": "json_object"}
		err = postJSON(ctx, o.cfg.HTTPClient, o.cfg.BaseURL+"/chat/completions", headers, body, &out)
	}
	if err != nil {
		return Response{}, err
	}
	if len(out.Choices) == 0 {
		return Response{}, &ErrProvider{Message: "the provider returned no choices", Retryable: true}
	}
	ch := out.Choices[0]
	if ch.Message.Refusal != "" {
		return Response{}, &ErrProvider{Message: "the model declined the request: " + ch.Message.Refusal}
	}
	if ch.FinishReason == "length" {
		return Response{}, &ErrProvider{Message: "the response was cut off; raise the provider's maximum output tokens"}
	}
	res := Response{Text: ch.Message.Content, Model: firstNonEmpty(out.Model, o.cfg.Model), InputTokens: out.Usage.PromptTokens, OutputTokens: out.Usage.CompletionTokens}
	if req.Schema != nil {
		js, err := ExtractJSON(ch.Message.Content)
		if err != nil {
			return res, err
		}
		res.JSON = js
	}
	return res, nil
}

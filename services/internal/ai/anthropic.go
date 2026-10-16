package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// anthropicProvider calls the Claude Messages API through the official Go SDK. Thinking stays on
// (current models always think; depth is set with effort), JSON is requested with structured
// outputs, and server-side fallbacks re-serve requests that a safety classifier declines.
type anthropicProvider struct {
	cfg    Config
	client anthropic.Client
}

// minAnthropicTokens leaves room for thinking, which shares the max_tokens budget.
const minAnthropicTokens = 16000

func newAnthropic(cfg Config) *anthropicProvider {
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(cfg.BaseURL),
		option.WithRequestTimeout(cfg.Timeout),
		option.WithMaxRetries(1),
		option.WithHTTPClient(cfg.HTTPClient),
	}
	return &anthropicProvider{cfg: cfg, client: anthropic.NewClient(opts...)}
}

func (a *anthropicProvider) Kind() string { return "anthropic" }

func (a *anthropicProvider) Complete(ctx context.Context, req Request) (Response, error) {
	blocks := []anthropic.BetaContentBlockParamUnion{}
	for _, im := range req.Images {
		blocks = append(blocks, anthropic.NewBetaImageBlock(anthropic.BetaBase64ImageSourceParam{
			Data:      base64.StdEncoding.EncodeToString(im.Data),
			MediaType: anthropic.BetaBase64ImageSourceMediaType(im.MediaType),
		}))
	}
	blocks = append(blocks, anthropic.NewBetaTextBlock(req.Prompt))
	maxTokens := int64(a.cfg.MaxTokens)
	if maxTokens < minAnthropicTokens {
		maxTokens = minAnthropicTokens
	}
	params := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(a.cfg.Model),
		MaxTokens: maxTokens,
		System:    []anthropic.BetaTextBlockParam{{Text: req.System}},
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(blocks...)},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: anthropic.BetaOutputConfigEffortMedium,
		},
		Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
	}
	if req.Schema != nil {
		schema, err := json.Marshal(req.Schema)
		if err != nil {
			return Response{}, err
		}
		params.OutputConfig.Format = anthropic.BetaJSONOutputFormatParam{Schema: json.RawMessage(schema)}
	}
	msg, err := a.client.Beta.Messages.New(ctx, params)
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			text := apiErr.Error()
			if len(text) > 500 {
				text = text[:500]
			}
			return Response{}, &ErrProvider{Status: apiErr.StatusCode, Message: text,
				Retryable: apiErr.StatusCode == 429 || apiErr.StatusCode >= 500}
		}
		if ctx.Err() != nil {
			return Response{}, ctx.Err()
		}
		return Response{}, &ErrProvider{Message: "could not reach the Anthropic API: " + err.Error(), Retryable: true}
	}
	if msg.StopReason == anthropic.BetaStopReasonRefusal {
		return Response{}, &ErrProvider{Message: "the model declined the request (category: " + string(msg.StopDetails.Category) + ")"}
	}
	if msg.StopReason == anthropic.BetaStopReasonMaxTokens {
		return Response{}, &ErrProvider{Message: "the response was cut off; raise the provider's maximum output tokens"}
	}
	var text strings.Builder
	for _, block := range msg.Content {
		if tb, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			text.WriteString(tb.Text)
		}
	}
	res := Response{Text: text.String(), Model: string(msg.Model), InputTokens: int(msg.Usage.InputTokens), OutputTokens: int(msg.Usage.OutputTokens)}
	if req.Schema != nil {
		js, err := ExtractJSON(res.Text)
		if err != nil {
			return res, err
		}
		res.JSON = js
	}
	return res, nil
}

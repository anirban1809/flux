package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"zipcode/src/config"
	"zipcode/src/llm/errors"
	"zipcode/src/tools"
)

const anthropicAPIVersion = "2023-06-01"

type Anthropic struct {
	ProviderId string
	Model      string
	Tools      []tools.Tool
	ApiKey     string
}

func NewAnthropicProvider() *Anthropic {
	return &Anthropic{}
}

func (p Anthropic) Name() ProviderName {
	return AnthropicProvider
}

func (p *Anthropic) SetApiKey(key string) {
	p.ApiKey = key
}

type anthropicTool struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	InputSchema tools.JSONSchema `json:"input_schema"`
}

type anthropicContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
	Temperature float64            `json:"temperature,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

type anthropicResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	Role       string                  `json:"role"`
	Model      string                  `json:"model"`
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *Anthropic) AuthCheck(key string) AuthResult {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(
		http.MethodGet,
		"https://api.anthropic.com/v1/models",
		nil,
	)
	if err != nil {
		return AuthResult{Status: 0, ErrorMessage: err.Error()}
	}
	req.Header.Set("x-api-key", key)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := client.Do(req)
	if err != nil {
		return AuthResult{Status: 0, ErrorMessage: err.Error()}
	}
	defer resp.Body.Close()

	result := AuthResult{Status: resp.StatusCode}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		result.ErrorMessage = string(body)
	} else {
		p.ApiKey = key
	}
	return result
}

func (p Anthropic) Complete(request ChatRequest) (ChatResponse, error) {
	if request.Stream {
		return p.completeStream(request)
	}

	system, msgs := convertMessagesToAnthropic(request.Messages)

	maxTokens := request.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 8192
	}

	body, err := json.Marshal(anthropicRequest{
		Model:       config.Cfg.CurrentModel,
		MaxTokens:   maxTokens,
		System:      system,
		Messages:    msgs,
		Tools:       convertToolsToAnthropic(request.Tools),
		Temperature: request.Temperature,
	})
	if err != nil {
		return ChatResponse{}, err
	}

	res, err := errors.RetryWithBackoff(p, func() (*http.Response, error) {
		req, err := http.NewRequest(
			http.MethodPost,
			"https://api.anthropic.com/v1/messages",
			bytes.NewReader(body),
		)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", p.ApiKey)
		req.Header.Set("anthropic-version", anthropicAPIVersion)
		return http.DefaultClient.Do(req)
	})
	if err != nil {
		return ChatResponse{}, err
	}

	respBody, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return ChatResponse{}, err
	}

	if res.StatusCode >= 400 {
		return ChatResponse{}, fmt.Errorf(
			"anthropic: status %d: %s",
			res.StatusCode,
			string(respBody),
		)
	}

	var parsed anthropicResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return ChatResponse{}, err
	}

	if parsed.Error != nil {
		return ChatResponse{}, fmt.Errorf(
			"anthropic: %s: %s",
			parsed.Error.Type,
			parsed.Error.Message,
		)
	}

	textParts := []string{}
	toolCalls := []ToolCall{}
	for i, block := range parsed.Content {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			args := string(block.Input)
			if args == "" {
				args = "{}"
			}
			toolCalls = append(toolCalls, ToolCall{
				Type:  "function",
				Index: i,
				ID:    block.ID,
				Function: ToolCallFunction{
					Name:      block.Name,
					Arguments: args,
				},
			})
		}
	}

	role := parsed.Role
	if role == "" {
		role = "assistant"
	}

	return ChatResponse{
		ID:         parsed.ID,
		Model:      parsed.Model,
		StopReason: parsed.StopReason,
		Usage: Usage{
			// Normalize so InputTokens represents *total* input (matches OpenAI
			// semantics) while CachedInputTokens is the cached-read portion.
			// Anthropic's cache_creation is folded into the regular input
			// bucket; we don't currently price it separately.
			InputTokens: parsed.Usage.InputTokens +
				parsed.Usage.CacheCreationInputTokens +
				parsed.Usage.CacheReadInputTokens,
			CachedInputTokens: parsed.Usage.CacheReadInputTokens,
			OutputTokens:      parsed.Usage.OutputTokens,
		},
		Message: Message{
			Role:      role,
			Content:   strings.Join(textParts, ""),
			ToolCalls: toolCalls,
		},
	}, nil
}

type anthropicStreamEvent struct {
	Type         string                `json:"type"`
	Index        int                   `json:"index"`
	Message      *anthropicStreamMsg   `json:"message,omitempty"`
	Delta        anthropicStreamDelta  `json:"delta,omitempty"`
	ContentBlock anthropicContentBlock `json:"content_block,omitempty"`
	Usage        anthropicStreamUsage  `json:"usage,omitempty"`
	Error        *anthropicStreamError `json:"error,omitempty"`
}

type anthropicStreamMsg struct {
	ID      string                  `json:"id"`
	Type    string                  `json:"type"`
	Role    string                  `json:"role"`
	Model   string                  `json:"model"`
	Usage   anthropicStreamUsage    `json:"usage"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicStreamDelta struct {
	Type        string               `json:"type"`
	Text        string               `json:"text,omitempty"`
	PartialJSON string               `json:"partial_json,omitempty"`
	StopReason  string               `json:"stop_reason,omitempty"`
	Usage       anthropicStreamUsage `json:"usage,omitempty"`
}

type anthropicStreamUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

type anthropicStreamError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (u anthropicStreamUsage) normalized() Usage {
	return Usage{
		InputTokens: u.InputTokens +
			u.CacheCreationInputTokens +
			u.CacheReadInputTokens,
		CachedInputTokens: u.CacheReadInputTokens,
		OutputTokens:      u.OutputTokens,
	}
}

func (p Anthropic) completeStream(request ChatRequest) (ChatResponse, error) {
	system, msgs := convertMessagesToAnthropic(request.Messages)

	maxTokens := request.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 8192
	}

	body, err := json.Marshal(anthropicRequest{
		Model:       config.Cfg.CurrentModel,
		MaxTokens:   maxTokens,
		System:      system,
		Messages:    msgs,
		Tools:       convertToolsToAnthropic(request.Tools),
		Temperature: request.Temperature,
		Stream:      true,
	})
	if err != nil {
		return ChatResponse{}, err
	}

	res, err := errors.RetryWithBackoff(p, func() (*http.Response, error) {
		req, err := http.NewRequest(
			http.MethodPost,
			"https://api.anthropic.com/v1/messages",
			bytes.NewReader(body),
		)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("x-api-key", p.ApiKey)
		req.Header.Set("anthropic-version", anthropicAPIVersion)
		return http.DefaultClient.Do(req)
	})
	if err != nil {
		return ChatResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		respBody, _ := io.ReadAll(res.Body)
		return ChatResponse{}, fmt.Errorf(
			"anthropic: status %d: %s",
			res.StatusCode,
			string(respBody),
		)
	}

	var response ChatResponse
	var content strings.Builder
	role := "assistant"
	toolCalls := map[int]*streamingToolCall{}

	err = readSSE(res.Body, func(_, data string) error {
		var ev anthropicStreamEvent
		if err := decodeJSONEvent(data, &ev); err != nil {
			return err
		}

		switch ev.Type {
		case "message_start":
			if ev.Message != nil {
				response.ID = ev.Message.ID
				response.Model = ev.Message.Model
				if ev.Message.Role != "" {
					role = ev.Message.Role
				}
				response.Usage = ev.Message.Usage.normalized()
			}
		case "content_block_start":
			if ev.ContentBlock.Type == "tool_use" {
				rawInput := string(ev.ContentBlock.Input)
				if rawInput == "{}" {
					rawInput = ""
				}
				mergeToolCallDelta(toolCalls, ToolCall{
					Type:  "function",
					Index: ev.Index,
					ID:    ev.ContentBlock.ID,
					Function: ToolCallFunction{
						Name:      ev.ContentBlock.Name,
						Arguments: rawInput,
					},
				})
			}
		case "content_block_delta":
			switch ev.Delta.Type {
			case "text_delta":
				if ev.Delta.Text != "" {
					content.WriteString(ev.Delta.Text)
					emitStream(request.OnStream, StreamEvent{
						Type:    StreamText,
						Content: ev.Delta.Text,
					})
				}
			case "input_json_delta":
				if ev.Delta.PartialJSON != "" {
					mergeToolCallDelta(toolCalls, ToolCall{
						Type:  "function",
						Index: ev.Index,
						Function: ToolCallFunction{
							Arguments: ev.Delta.PartialJSON,
						},
					})
				}
			}
		case "message_delta":
			if ev.Delta.StopReason != "" {
				response.StopReason = ev.Delta.StopReason
			}
			usage := ev.Usage
			if usage.OutputTokens == 0 {
				usage = ev.Delta.Usage
			}
			normalized := usage.normalized()
			if normalized.InputTokens != 0 {
				response.Usage.InputTokens = normalized.InputTokens
			}
			if normalized.CachedInputTokens != 0 {
				response.Usage.CachedInputTokens = normalized.CachedInputTokens
			}
			if normalized.OutputTokens != 0 {
				response.Usage.OutputTokens = normalized.OutputTokens
			}
		case "message_stop":
			emitStream(request.OnStream, StreamEvent{
				Type:       StreamStop,
				StopReason: response.StopReason,
				Usage:      response.Usage,
			})
		case "error":
			if ev.Error != nil {
				return fmt.Errorf("anthropic stream: %s: %s", ev.Error.Type, ev.Error.Message)
			}
		}
		return nil
	})
	if err != nil {
		return ChatResponse{}, err
	}

	response.Message = Message{
		Role:      role,
		Content:   content.String(),
		ToolCalls: finalizeToolCalls(toolCalls),
		Streamed:  true,
	}
	for _, tc := range response.Message.ToolCalls {
		call := tc
		emitStream(request.OnStream, StreamEvent{
			Type:     StreamToolCall,
			ToolCall: &call,
		})
	}

	return response, nil
}

func (p Anthropic) Models() []ModelDescriptor {
	entries := []struct {
		id            string
		contextWindow int
		inputCost     float64
		outputCost    float64
	}{
		{"claude-opus-4-7", 1_000_000, 5.00, 25.00},
		{"claude-sonnet-4-6", 1_000_000, 3.00, 15.00},
		{"claude-haiku-4-5-20251001", 200_000, 1.00, 5.00},
	}
	descriptors := make([]ModelDescriptor, len(entries))
	for i, e := range entries {
		descriptors[i] = ModelDescriptor{
			ID:                   e.id,
			DisplayName:          e.id,
			ProviderName:         string(AnthropicProvider),
			ContextWindow:        e.contextWindow,
			InputCostPerMillion:  e.inputCost,
			OutputCostPerMillion: e.outputCost,
		}
	}
	return descriptors
}

func (p Anthropic) IsQuotaError(resp *http.Response, body []byte) bool {
	if resp.StatusCode != http.StatusTooManyRequests &&
		resp.StatusCode != http.StatusPaymentRequired &&
		resp.StatusCode != http.StatusBadRequest {
		return false
	}
	var parsed struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false
	}
	// Anthropic surfaces credit/quota issues via these markers.
	if strings.Contains(strings.ToLower(parsed.Error.Message), "credit balance") {
		return true
	}
	return parsed.Error.Type == "billing_error" ||
		parsed.Error.Type == "insufficient_quota"
}

func convertToolsToAnthropic(in []tools.Tool) []anthropicTool {
	if len(in) == 0 {
		return nil
	}
	out := make([]anthropicTool, 0, len(in))
	for _, t := range in {
		out = append(out, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}
	return out
}

// convertMessagesToAnthropic adapts the internal OpenAI-shaped message list
// to Anthropic's messages API. System turns are hoisted into the top-level
// system field; tool results (role="tool") become user messages with
// tool_result blocks; assistant tool calls become tool_use blocks.
func convertMessagesToAnthropic(msgs []Message) (string, []anthropicMessage) {
	var system string
	out := make([]anthropicMessage, 0, len(msgs))

	for _, m := range msgs {
		switch m.Role {
		case "system":
			if system != "" {
				system += "\n\n"
			}
			system += m.Content
		case "tool":
			out = append(out, anthropicMessage{
				Role: "user",
				Content: []anthropicContentBlock{{
					Type:      "tool_result",
					ToolUseID: m.ToolCallId,
					Content:   m.Content,
				}},
			})
		case "assistant":
			blocks := []anthropicContentBlock{}
			if m.Content != "" {
				blocks = append(blocks, anthropicContentBlock{
					Type: "text",
					Text: m.Content,
				})
			}
			for _, tc := range m.ToolCalls {
				input := json.RawMessage(tc.Function.Arguments)
				if len(input) == 0 {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, anthropicContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Function.Name,
					Input: input,
				})
			}
			if len(blocks) == 0 {
				continue
			}
			out = append(out, anthropicMessage{
				Role:    "assistant",
				Content: blocks,
			})
		default:
			out = append(out, anthropicMessage{
				Role: "user",
				Content: []anthropicContentBlock{{
					Type: "text",
					Text: m.Content,
				}},
			})
		}
	}

	return system, out
}

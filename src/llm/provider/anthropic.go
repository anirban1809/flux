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
			// InputTokens is the total input context (all buckets combined) so
			// that context-window percentage stays accurate.
			InputTokens: parsed.Usage.InputTokens +
				parsed.Usage.CacheCreationInputTokens +
				parsed.Usage.CacheReadInputTokens,
			CachedInputTokens: parsed.Usage.CacheReadInputTokens,
			CacheWriteTokens:  parsed.Usage.CacheCreationInputTokens,
			OutputTokens:      parsed.Usage.OutputTokens,
		},
		Message: Message{
			Role:      role,
			Content:   strings.Join(textParts, ""),
			ToolCalls: toolCalls,
		},
	}, nil
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
			ID:                       e.id,
			DisplayName:              e.id,
			ProviderName:             string(AnthropicProvider),
			ContextWindow:            e.contextWindow,
			InputCostPerMillion:      e.inputCost,
			OutputCostPerMillion:     e.outputCost,
			CacheReadCostPerMillion:  e.inputCost * 0.10,
			CacheWriteCostPerMillion: e.inputCost * 1.25,
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
	if strings.Contains(
		strings.ToLower(parsed.Error.Message),
		"credit balance",
	) {
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

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

type OpenAI struct {
	ProviderId string
	Model      string
	Tools      []tools.Tool
	ApiKey     string
}

func (p OpenAI) Name() ProviderName {
	return OpenAIProvider
}

func (p *OpenAI) SetApiKey(key string) {
	p.ApiKey = key
}

func (p *OpenAI) AuthCheck(key string) AuthResult {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(
		http.MethodGet,
		"https://api.openai.com/v1/models",
		nil,
	)
	if err != nil {
		return AuthResult{Status: 0, ErrorMessage: err.Error()}
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))

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

type openAIRequest struct {
	Model         string         `json:"model"`
	Messages      []Message      `json:"messages"`
	Tools         []tools.Tool   `json:"tools,omitempty"`
	MaxTokens     int            `json:"max_tokens,omitempty"`
	Temperature   float64        `json:"temperature,omitempty"`
	Stream        bool           `json:"stream,omitempty"`
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

func isOpenAIMissingToolResponseError(statusCode int, body string) bool {
	return statusCode == http.StatusBadRequest &&
		strings.Contains(body, "tool_calls") &&
		strings.Contains(body, "must be followed by tool messages") &&
		strings.Contains(body, "tool_call_id")
}

func sanitizeOpenAIMessagesForMissingToolResponses(messages []Message) ([]Message, bool) {
	sanitized := make([]Message, 0, len(messages))
	changed := false

	for i := 0; i < len(messages); {
		message := messages[i]
		if message.Role == "tool" {
			changed = true
			i++
			continue
		}

		if message.Role == "assistant" && len(message.ToolCalls) > 0 {
			required := map[string]bool{}
			valid := true
			for _, toolCall := range message.ToolCalls {
				if toolCall.ID == "" || required[toolCall.ID] {
					valid = false
				}
				required[toolCall.ID] = true
			}

			j := i + 1
			seen := map[string]bool{}
			for j < len(messages) && messages[j].Role == "tool" {
				toolCallID := messages[j].ToolCallId
				if !required[toolCallID] || seen[toolCallID] {
					valid = false
				}
				seen[toolCallID] = true
				j++
			}

			for toolCallID := range required {
				if !seen[toolCallID] {
					valid = false
				}
			}

			if valid {
				sanitized = append(sanitized, message)
				sanitized = append(sanitized, messages[i+1:j]...)
			} else {
				changed = true
			}
			i = j
			continue
		}

		sanitized = append(sanitized, message)
		i++
	}

	return sanitized, changed
}

type openAIResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

func (p OpenAI) Complete(request ChatRequest) (ChatResponse, error) {
	if request.Stream {
		return p.completeStream(request)
	}
	return p.completeWithMessages(request, request.Messages, false)
}

func (p OpenAI) completeWithMessages(request ChatRequest, messages []Message, alreadySanitized bool) (ChatResponse, error) {
	body, err := json.Marshal(openAIRequest{
		Model:       config.Cfg.CurrentModel,
		Messages:    messages,
		Tools:       request.Tools,
		MaxTokens:   request.MaxTokens,
		Temperature: request.Temperature,
	})
	if err != nil {
		return ChatResponse{}, err
	}

	res, err := errors.RetryWithBackoff(p, func() (*http.Response, error) {
		req, err := http.NewRequest(
			http.MethodPost,
			"https://api.openai.com/v1/chat/completions",
			bytes.NewReader(body),
		)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.ApiKey))
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
		bodyStr := string(respBody)
		if !alreadySanitized && isOpenAIMissingToolResponseError(res.StatusCode, bodyStr) {
			sanitized, changed := sanitizeOpenAIMessagesForMissingToolResponses(messages)
			if changed {
				return p.completeWithMessages(request, sanitized, true)
			}
		}
		return ChatResponse{}, fmt.Errorf(
			"openai: status %d: %s",
			res.StatusCode,
			bodyStr,
		)
	}

	var parsed openAIResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return ChatResponse{}, err
	}

	if len(parsed.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("openai: no choices in response")
	}

	return ChatResponse{
		ID:    parsed.ID,
		Model: parsed.Model,
		Usage: Usage{
			InputTokens:       parsed.Usage.PromptTokens,
			CachedInputTokens: parsed.Usage.PromptTokensDetails.CachedTokens,
			OutputTokens:      parsed.Usage.CompletionTokens,
		},
		Message: Message{
			Role:      parsed.Choices[0].Message.Role,
			Content:   parsed.Choices[0].Message.Content,
			ToolCalls: parsed.Choices[0].Message.ToolCalls,
		},
	}, nil
}

type openAIStreamChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}

func (p OpenAI) completeStream(request ChatRequest) (ChatResponse, error) {
	return p.completeStreamWithMessages(request, request.Messages, false)
}

func (p OpenAI) completeStreamWithMessages(request ChatRequest, messages []Message, alreadySanitized bool) (ChatResponse, error) {
	body, err := json.Marshal(openAIRequest{
		Model:       config.Cfg.CurrentModel,
		Messages:    messages,
		Tools:       request.Tools,
		MaxTokens:   request.MaxTokens,
		Temperature: request.Temperature,
		Stream:      true,
		StreamOptions: &streamOptions{
			IncludeUsage: true,
		},
	})
	if err != nil {
		return ChatResponse{}, err
	}

	res, err := errors.RetryWithBackoff(p, func() (*http.Response, error) {
		req, err := http.NewRequest(
			http.MethodPost,
			"https://api.openai.com/v1/chat/completions",
			bytes.NewReader(body),
		)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.ApiKey))
		return http.DefaultClient.Do(req)
	})
	if err != nil {
		return ChatResponse{}, err
	}
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		respBody, _ := io.ReadAll(res.Body)
		bodyStr := string(respBody)
		if !alreadySanitized && isOpenAIMissingToolResponseError(res.StatusCode, bodyStr) {
			sanitized, changed := sanitizeOpenAIMessagesForMissingToolResponses(messages)
			if changed {
				return p.completeStreamWithMessages(request, sanitized, true)
			}
		}
		return ChatResponse{}, fmt.Errorf(
			"openai: status %d: %s",
			res.StatusCode,
			bodyStr,
		)
	}

	var response ChatResponse
	var content strings.Builder
	role := "assistant"
	toolCalls := map[int]*streamingToolCall{}

	err = readSSE(res.Body, func(_, data string) error {
		if data == "[DONE]" {
			emitStream(request.OnStream, StreamEvent{
				Type:       StreamStop,
				StopReason: response.StopReason,
				Usage:      response.Usage,
			})
			return nil
		}

		var chunk openAIStreamChunk
		if err := decodeJSONEvent(data, &chunk); err != nil {
			return err
		}
		if chunk.ID != "" {
			response.ID = chunk.ID
		}
		if chunk.Model != "" {
			response.Model = chunk.Model
		}
		if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
			response.Usage = Usage{
				InputTokens:       chunk.Usage.PromptTokens,
				CachedInputTokens: chunk.Usage.PromptTokensDetails.CachedTokens,
				OutputTokens:      chunk.Usage.CompletionTokens,
			}
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Role != "" {
				role = choice.Delta.Role
			}
			if choice.Delta.Content != "" {
				content.WriteString(choice.Delta.Content)
				emitStream(request.OnStream, StreamEvent{
					Type:    StreamText,
					Content: choice.Delta.Content,
				})
			}
			for _, tc := range choice.Delta.ToolCalls {
				mergeToolCallDelta(toolCalls, tc)
			}
			if choice.FinishReason != "" {
				response.StopReason = choice.FinishReason
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

func (p OpenAI) Models() []ModelDescriptor {
	entries := []struct {
		id            string
		contextWindow int
		inputCost     float64
		outputCost    float64
	}{
		{"gpt-5.2", 400_000, 1.75, 14.00},
		{"gpt-5.5", 1_000_000, 5.00, 30.00},
		{"gpt-5.4", 272_000, 2.50, 15.00},
		{"gpt-5.4-nano", 400_000, 0.20, 1.25},
		{"gpt-5.3-codex", 400_000, 1.75, 14.00},
		{"gpt-5.1-codex-mini", 400_000, 0.25, 2.00},
		{"gpt-5-nano", 400_000, 0.05, 0.40},
	}
	descriptors := make([]ModelDescriptor, len(entries))
	for i, e := range entries {
		descriptors[i] = ModelDescriptor{
			ID:                   e.id,
			DisplayName:          e.id,
			ProviderName:         string(OpenAIProvider),
			ContextWindow:        e.contextWindow,
			InputCostPerMillion:  e.inputCost,
			OutputCostPerMillion: e.outputCost,
		}
	}
	return descriptors
}

func (p OpenAI) IsQuotaError(resp *http.Response, body []byte) bool {
	if resp.StatusCode != http.StatusTooManyRequests &&
		resp.StatusCode != http.StatusPaymentRequired {
		return false
	}
	var parsed struct {
		Error struct {
			Code string `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false
	}
	return parsed.Error.Code == "insufficient_quota" ||
		parsed.Error.Type == "insufficient_quota"
}

package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flux/src/config"
	"flux/src/events"
	"flux/src/llm/errors"
	"flux/src/tools"
	"flux/src/utils"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
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

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type CompletionUsage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	TotalTokens         int `json:"total_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

type openAIRequest struct {
	Model         string         `json:"model"`
	Messages      []Message      `json:"messages"`
	Tools         []tools.Tool   `json:"tools,omitempty"`
	MaxTokens     int            `json:"max_tokens,omitempty"`
	Temperature   float64        `json:"temperature,omitempty"`
	Stream        bool           `json:"stream,omitempty"`
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`
}

type function struct {
	Name      string
	Arguments string
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Role      string     `json:"role"`
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *CompletionUsage `json:"usage,omitempty"`
}

type message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls"`
}

type choice struct {
	Message      message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type openAIResponse struct {
	ID      string           `json:"id"`
	Model   string           `json:"model"`
	Choices []choice         `json:"choices"`
	Usage   *CompletionUsage `json:"usage"`
}

func ParseStreamResponse(read io.Reader) ChatResponse {
	reader := bufio.NewReader(read)
	var response openAIResponse
	response.Choices = append(response.Choices, choice{})

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			panic(err)
		}

		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue // SSE keep-alive / separator
		}
		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}

		data := bytes.TrimPrefix(line, []byte("data: "))
		if strings.TrimSpace(string(data)) == "[DONE]" {
			break
		}

		var chunk streamChunk
		if err := json.Unmarshal(data, &chunk); err != nil {
			fmt.Printf("\nparse error: %v\n", err)
			continue
		}

		if chunk.Usage != nil {
			response.Usage = chunk.Usage
		}

		for _, c := range chunk.Choices {
			if c.Delta.Role != "" {
				response.Choices[0].Message.Role = c.Delta.Role
			}

			for _, t := range c.Delta.ToolCalls {
				for len(response.Choices[0].Message.ToolCalls) <= t.Index {
					response.Choices[0].Message.ToolCalls = append(
						response.Choices[0].Message.ToolCalls, ToolCall{},
					)
				}
				tc := &response.Choices[0].Message.ToolCalls[t.Index]
				if t.ID != "" {
					tc.ID = t.ID
					tc.Index = t.Index
					tc.Type = t.Type
					tc.Function.Name = t.Function.Name
				}
				tc.Function.Arguments += t.Function.Arguments
			}

			if c.Delta.Content != "" {
				events.EventManager.WriteToChannel(
					events.STREAM_CHUNK_CHANNEL,
					c.Delta.Content,
				)
			}

			response.Choices[0].Message.Content = response.Choices[0].Message.Content + c.Delta.Content
		}
	}

	var usage Usage
	if response.Usage != nil {
		usage = Usage{
			InputTokens:       response.Usage.PromptTokens,
			CachedInputTokens: response.Usage.PromptTokensDetails.CachedTokens,
			OutputTokens:      response.Usage.CompletionTokens,
		}
	}

	return ChatResponse{
		Model: config.Cfg.CurrentModel,
		Usage: usage,
		Message: Message{
			Role:      response.Choices[0].Message.Role,
			Content:   response.Choices[0].Message.Content,
			ToolCalls: response.Choices[0].Message.ToolCalls,
		},
	}
}

func (p OpenAI) Complete(request ChatRequest) (ChatResponse, error) {
	requestBody := openAIRequest{
		Model:       config.Cfg.CurrentModel,
		Messages:    request.Messages,
		Tools:       request.Tools,
		MaxTokens:   request.MaxTokens,
		Temperature: request.Temperature,
		Stream:      config.Cfg.StreamResponses,
	}

	if config.Cfg.StreamResponses {
		requestBody.StreamOptions = &StreamOptions{IncludeUsage: true}
	}

	body, err := json.Marshal(requestBody)
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
	defer res.Body.Close()

	if res.StatusCode >= 400 {
		respBody, _ := io.ReadAll(res.Body)
		return ChatResponse{}, fmt.Errorf(
			"openai: status %d: %s",
			res.StatusCode,
			string(respBody),
		)
	}

	if config.Cfg.StreamResponses {
		output := ParseStreamResponse(res.Body)
		utils.LogValue(output)
		return output, nil
	}

	respBody, err := io.ReadAll(res.Body)
	if err != nil {
		return ChatResponse{}, err
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
			ID:                      e.id,
			DisplayName:             e.id,
			ProviderName:            string(OpenAIProvider),
			ContextWindow:           e.contextWindow,
			InputCostPerMillion:     e.inputCost,
			OutputCostPerMillion:    e.outputCost,
			CacheReadCostPerMillion: e.inputCost * 0.50,
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

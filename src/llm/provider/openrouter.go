package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"zipcode/src/config"
	"zipcode/src/llm/errors"
	"zipcode/src/tools"
	"zipcode/src/utils"

	"github.com/joho/godotenv"
)

type OpenRouterProvider struct {
	ProviderId string
	Model      string
	Tools      []tools.Tool
	ApiKey     string
}

func NewOpenRouterProvider() *OpenRouterProvider {
	return &OpenRouterProvider{}
}

type OpenRouterRequest struct {
	Model               string                 `json:"model,omitempty"`
	Messages            []Message              `json:"messages"`
	Provider            *ProviderConfig        `json:"provider,omitempty"`
	Temperature         float64                `json:"temperature,omitempty"`
	TopP                *float64               `json:"top_p,omitempty"`
	FrequencyPenalty    *float64               `json:"frequency_penalty,omitempty"`
	PresencePenalty     *float64               `json:"presence_penalty,omitempty"`
	MaxTokens           int                    `json:"max_tokens,omitempty"`
	MaxCompletionTokens int                    `json:"max_completion_tokens,omitempty"`
	Stop                []string               `json:"stop,omitempty"`
	Stream              bool                   `json:"stream,omitempty"`
	User                string                 `json:"user,omitempty"`
	SessionID           string                 `json:"session_id,omitempty"`
	Modalities          []string               `json:"modalities,omitempty"`
	Plugins             []PluginConfig         `json:"plugins,omitempty"`
	ToolChoice          interface{}            `json:"tool_choice,omitempty"`
	Tools               []tools.Tool           `json:"tools,omitempty"`
	Extra               map[string]interface{} `json:"extra,omitempty"` // forward compatibility
}

type ContentPart struct {
	Type     string        `json:"type"` // text | image_url | input_text | input_image
	Text     string        `json:"text,omitempty"`
	ImageURL *ImageURLPart `json:"image_url,omitempty"`
}

type ProviderConfig struct {
	AllowFallbacks *bool    `json:"allow_fallbacks,omitempty"`
	Order          []string `json:"order,omitempty"`
}

type PluginConfig struct {
	Name   string                 `json:"name"`
	Config map[string]interface{} `json:"config,omitempty"`
}

type ToolDefinition struct {
	Type     string       `json:"type"` // typically "function"
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"` // JSON Schema
}

type ImageURLPart struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // auto | low | high
}

type OpenRouterResponse struct {
	ID                string    `json:"id"`
	Provider          string    `json:"provider"`
	Model             string    `json:"model"`
	Object            string    `json:"object"`
	Created           int       `json:"created"`
	Choices           []Choices `json:"choices"`
	SystemFingerprint string    `json:"system_fingerprint"`
	Usage             struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}
type ReasoningDetails struct {
	Format any    `json:"format"`
	Index  int    `json:"index"`
	Type   string `json:"type"`
	Text   string `json:"text"`
}

type Choices struct {
	FinishReason       string `json:"finish_reason"`
	NativeFinishReason string `json:"native_finish_reason"`
	Index              int    `json:"index"`
	Message            struct {
		Content   string     `json:"content"`
		Role      string     `json:"role"`
		ToolCalls []ToolCall `json:"tool_calls"`
	} `json:"message"`
	Delta MessageDelta `json:"delta"`
}

type MessageDelta struct {
	Content   string     `json:"content"`
	Role      string     `json:"role"`
	Reasoning string     `json:"reasoning"`
	ToolCalls []ToolCall `json:"tool_calls"`
}

type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
	AudioTokens  int `json:"audio_tokens"`
}
type CostDetails struct {
	UpstreamInferenceCost            float64 `json:"upstream_inference_cost"`
	UpstreamInferencePromptCost      float64 `json:"upstream_inference_prompt_cost"`
	UpstreamInferenceCompletionsCost float64 `json:"upstream_inference_completions_cost"`
}
type CompletionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
	AudioTokens     int `json:"audio_tokens"`
}

func (p *OpenRouterProvider) SetModel(model string, nitro bool) {
	if nitro {
		p.Model = fmt.Sprintf("%s:nitro", model)
		return
	}
	p.Model = model
}

func (p *OpenRouterProvider) AuthCheck(key string) AuthResult {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(
		http.MethodGet,
		"https://openrouter.ai/api/v1/key",
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

func (p OpenRouterProvider) IsQuotaError(
	resp *http.Response,
	body []byte,
) bool {
	// OpenRouter uses 402 Payment Required for insufficient credits; 429 is rate-limit only.
	return resp.StatusCode == http.StatusPaymentRequired
}

func (p OpenRouterProvider) Models() []ModelDescriptor {
	entries := []struct {
		id             string
		contextWindow  int
		inputCost      float64
		outputCost     float64
		cacheReadRate  float64 // multiplier of inputCost (0 = no caching)
		cacheWriteRate float64 // multiplier of inputCost (0 = no cache writes)
	}{
		{"google/gemini-3.5-flash", 1048576, 1.40, 9, 0, 0},
		{"openai/gpt-5.2", 400_000, 1.75, 14.00, 0.50, 0},
		{"openai/gpt-5.5", 1_000_000, 5.00, 30.00, 0.50, 0},
		{"minimax/minimax-m2.5", 196_608, 0.15, 1.15, 0, 0},
		{"minimax/minimax-m2.7", 196_608, 0.279, 1.20, 0, 0},
		{"openai/gpt-5.1-codex-mini", 400_000, 0.25, 2.00, 0.50, 0},
		{"moonshotai/kimi-k2.5", 262_144, 0.40, 1.90, 0, 0},
		{"meta-llama/llama-3.3-70b-instruct", 128_000, 0.10, 0.32, 0, 0},
		{"z-ai/glm-4.7", 200_000, 0.40, 1.75, 0, 0},
		{"qwen/qwen3-coder-flash", 1_000_000, 0.195, 0.975, 0, 0},
		{"openai/gpt-5-nano", 400_000, 0.05, 0.40, 0.50, 0},
		{"z-ai/glm-5", 200_000, 0.60, 1.92, 0, 0},
		{"openai/gpt-5.4-nano", 400_000, 0.20, 1.25, 0.50, 0},
		{"deepseek/deepseek-v3.2", 200_000, 0.252, 0.378, 0, 0},
		{"openai/gpt-5.4", 272_000, 2.50, 15.00, 0.50, 0},
		{"openai/gpt-5.3-codex", 400_000, 1.75, 14.00, 0.50, 0},
		{"z-ai/glm-5v-turbo", 202_752, 1.20, 4.00, 0, 0},
		{"moonshotai/kimi-k2.6", 262144, 0.73, 3.49, 0, 0},
	}
	descriptors := make([]ModelDescriptor, len(entries))
	for i, e := range entries {
		descriptors[i] = ModelDescriptor{
			ID:                       e.id,
			DisplayName:              e.id,
			ProviderName:             string(OpenRouterAPIProvider),
			ContextWindow:            e.contextWindow,
			InputCostPerMillion:      e.inputCost,
			OutputCostPerMillion:     e.outputCost,
			CacheReadCostPerMillion:  e.inputCost * e.cacheReadRate,
			CacheWriteCostPerMillion: e.inputCost * e.cacheWriteRate,
		}
	}
	return descriptors
}

func (p OpenRouterProvider) Name() ProviderName {
	return OpenRouterAPIProvider
}

func (p *OpenRouterProvider) SetApiKey(key string) {
	p.ApiKey = key
}

func (p *OpenRouterProvider) Complete(
	request ChatRequest,
) (ChatResponse, error) {
	godotenv.Load()

	retry := true
	var finalResponse OpenRouterResponse

	for retry {
		requestBody := OpenRouterRequest{
			Model:               config.Cfg.CurrentModel,
			Messages:            request.Messages,
			Stream:              true,
			Tools:               request.Tools,
			MaxTokens:           8192,
			MaxCompletionTokens: 2048,
		}

		value, err := json.Marshal(requestBody)
		if err != nil {
			return ChatResponse{}, err
		}

		res, err := errors.RetryWithBackoff(p, func() (*http.Response, error) {
			req, err := http.NewRequest(
				http.MethodPost,
				"https://openrouter.ai/api/v1/chat/completions",
				bytes.NewReader(value),
			)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(
				"Authorization",
				fmt.Sprintf("Bearer %s", p.ApiKey),
			)
			return http.DefaultClient.Do(req)
		})
		if err != nil {
			return ChatResponse{}, err
		}

		defer res.Body.Close()

		if config.Cfg.StreamResponses {
			output := ParseStreamResponse(res.Body)
			utils.LogValue(output)
			return output, nil
		}

		body, err := io.ReadAll(res.Body)

		if err != nil {
			return ChatResponse{}, err
		}

		if err := json.Unmarshal(body, &finalResponse); err != nil {
			return ChatResponse{}, err
		}

		if len(finalResponse.Choices) > 0 {
			retry = false
		} else {
			fmt.Println("retrying", string(body))
		}
	}

	return ChatResponse{
		ID:    finalResponse.ID,
		Model: finalResponse.Model,
		Usage: Usage{
			InputTokens:       finalResponse.Usage.PromptTokens,
			CachedInputTokens: finalResponse.Usage.PromptTokensDetails.CachedTokens,
			OutputTokens:      finalResponse.Usage.CompletionTokens,
		},
		Message: Message{
			Role:      finalResponse.Choices[0].Message.Role,
			Content:   finalResponse.Choices[0].Message.Content,
			ToolCalls: finalResponse.Choices[0].Message.ToolCalls,
		},
	}, nil
}

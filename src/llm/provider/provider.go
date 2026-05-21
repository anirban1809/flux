package llm

import (
	"errors"
	"flux/src/tools"
	"net/http"
	"strings"
)

type ProviderName string

const (
	OpenAIProvider        ProviderName = "OpenAI"
	AnthropicProvider     ProviderName = "Anthropic"
	OpenRouterAPIProvider ProviderName = "OpenRouter"
)

func GetSupportedProviders() []ProviderName {
	return []ProviderName{
		OpenAIProvider,
		OpenRouterAPIProvider,
		AnthropicProvider,
	}
}

func GetProviderEnvVar(provider ProviderName) (string, error) {
	if provider == OpenAIProvider {
		return "OPENAI_API_KEY", nil
	}

	if provider == OpenRouterAPIProvider {
		return "OPENROUTER_API_KEY", nil
	}

	if provider == AnthropicProvider {
		return "ANTHROPIC_API_KEY", nil
	}

	return "", errors.New("unsupported provider")
}

func GetProviderName(provider string) (ProviderName, error) {
	switch strings.ToLower(provider) {
	case "openai":
		return OpenAIProvider, nil
	case "anthropic":
		return AnthropicProvider, nil
	case "openrouter":
		return OpenRouterAPIProvider, nil
	}

	return "", errors.New("unsupported provider")
}

type ModelDescriptor struct {
	ID            string
	DisplayName   string
	ProviderName  string
	ContextWindow int
	Effort        string
	// USD per 1M tokens.
	InputCostPerMillion      float64
	OutputCostPerMillion     float64
	CacheReadCostPerMillion  float64 // 0 if provider doesn't support prompt caching
	CacheWriteCostPerMillion float64 // 0 if provider doesn't charge separately for cache writes
}

type BlockType string

const (
	BlockText       BlockType = "text"
	BlockToolUse    BlockType = "tool_use"
	BlockToolResult BlockType = "tool_result"
)

type ToolUse struct {
	ID    string
	Name  string
	Input map[string]any
}

type ToolResult struct {
	ToolUseID string
	Content   string
	IsError   bool
}

type ContentBlock struct {
	Type       BlockType
	Text       string
	ToolUse    *ToolUse
	ToolResult *ToolResult
}

type ToolCall struct {
	Type     string           `json:"type"`
	Index    int              `json:"index"`
	ID       string           `json:"id"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls"`
	ToolCallId string     `json:"tool_call_id,omitempty"`
	// Usage is the API-reported token usage for the call that produced this
	// message. Only set on assistant messages returned by the provider; nil
	// for user/tool/system messages.
	Usage *Usage `json:"usage,omitempty"`
}

type ContextUsage struct {
	InputTokens  int
	OutputTokens int
}

type ChatRequest struct {
	Model       string
	Messages    []Message
	Tools       []tools.Tool
	MaxTokens   int
	Temperature float64
	Stream      bool
}

type Usage struct {
	InputTokens       int `json:"input_tokens"`
	CachedInputTokens int `json:"cached_input_tokens"`
	CacheWriteTokens  int `json:"cache_write_tokens"`
	OutputTokens      int `json:"output_tokens"`
}

type ChatResponse struct {
	ID         string
	Model      string
	Message    Message
	StopReason string
	Usage      Usage
}

type Conversation struct {
	Tools    []tools.Tool
	Messages []Message
	Usage    Usage
}

type AuthResult struct {
	Status       int
	ErrorMessage string
}

type Provider interface {
	Name() ProviderName
	AuthCheck(key string) AuthResult
	SetApiKey(key string)
	Complete(req ChatRequest) (ChatResponse, error)
	Models() []ModelDescriptor // static list of models this provider serves
	IsQuotaError(resp *http.Response, body []byte) bool
}

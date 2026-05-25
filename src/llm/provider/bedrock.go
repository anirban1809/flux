package llm

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"flux/src/config"
	"flux/src/llm/errors"
	"flux/src/tools"
)

const bedrockDefaultRegion = "us-east-1"

var bedrockThinkingTagRe = regexp.MustCompile(`(?s)<thinking>.*?</thinking>\s*`)

type Bedrock struct {
	ApiKey string
	Region string
}

func (p *Bedrock) Name() ProviderName {
	return BedrockProvider
}

func (p *Bedrock) SetApiKey(key string) {
	p.ApiKey = key
}

func (p *Bedrock) region() string {
	if p.Region != "" {
		return p.Region
	}
	if r := os.Getenv("AWS_REGION"); r != "" {
		return r
	}
	if r := os.Getenv("AWS_DEFAULT_REGION"); r != "" {
		return r
	}
	return bedrockDefaultRegion
}

func (p *Bedrock) AuthCheck(key string) AuthResult {
	endpoint := fmt.Sprintf(
		"https://bedrock.%s.amazonaws.com/foundation-models",
		p.region(),
	)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return AuthResult{Status: 0, ErrorMessage: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+key)

	client := &http.Client{Timeout: 10 * time.Second}
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

// ---- Bedrock Converse API types ----

type bedrockConverseRequest struct {
	Messages        []bedrockMessage       `json:"messages"`
	System          []bedrockSystemBlock   `json:"system,omitempty"`
	ToolConfig      *bedrockToolConfig     `json:"toolConfig,omitempty"`
	InferenceConfig bedrockInferenceConfig `json:"inferenceConfig"`
}

type bedrockMessage struct {
	Role    string                `json:"role"`
	Content []bedrockContentBlock `json:"content"`
}

type bedrockContentBlock struct {
	Text       *string            `json:"text,omitempty"`
	ToolUse    *bedrockToolUse    `json:"toolUse,omitempty"`
	ToolResult *bedrockToolResult `json:"toolResult,omitempty"`
}

type bedrockToolUse struct {
	ToolUseId string          `json:"toolUseId"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
}

type bedrockToolResult struct {
	ToolUseId string                   `json:"toolUseId"`
	Content   []bedrockToolResultBlock `json:"content"`
	Status    string                   `json:"status,omitempty"`
}

type bedrockToolResultBlock struct {
	Text string `json:"text"`
}

type bedrockSystemBlock struct {
	Text string `json:"text"`
}

type bedrockToolConfig struct {
	Tools []bedrockTool `json:"tools"`
}

type bedrockTool struct {
	ToolSpec bedrockToolSpec `json:"toolSpec"`
}

type bedrockToolSpec struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	InputSchema bedrockInputSchema `json:"inputSchema"`
}

type bedrockInputSchema struct {
	JSON tools.JSONSchema `json:"json"`
}

type bedrockInferenceConfig struct {
	MaxTokens   int     `json:"maxTokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

type bedrockConverseResponse struct {
	Output struct {
		Message bedrockMessage `json:"message"`
	} `json:"output"`
	StopReason string `json:"stopReason"`
	Usage      struct {
		InputTokens  int `json:"inputTokens"`
		OutputTokens int `json:"outputTokens"`
	} `json:"usage"`
}

func (p *Bedrock) Complete(request ChatRequest) (ChatResponse, error) {
	model := config.Cfg.CurrentModel

	system, msgs := convertMessagesToBedrock(request.Messages)

	maxTokens := request.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	converseReq := bedrockConverseRequest{
		Messages: msgs,
		InferenceConfig: bedrockInferenceConfig{
			MaxTokens:   maxTokens,
			Temperature: request.Temperature,
		},
	}
	if system != "" {
		converseReq.System = []bedrockSystemBlock{{Text: system}}
	}
	if len(request.Tools) > 0 {
		converseReq.ToolConfig = convertToolsToBedrock(request.Tools)
	}

	body, err := json.Marshal(converseReq)
	if err != nil {
		return ChatResponse{}, err
	}

	suffix := "converse"
	if config.Cfg.StreamResponses {
		suffix = "converse-stream"
	}
	endpoint := fmt.Sprintf(
		"https://bedrock-runtime.%s.amazonaws.com/model/%s/%s",
		p.region(), url.PathEscape(model), suffix,
	)

	res, err := errors.RetryWithBackoff(p, func() (*http.Response, error) {
		req, err := http.NewRequest(
			http.MethodPost,
			endpoint,
			bytes.NewReader(body),
		)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+p.ApiKey)
		return http.DefaultClient.Do(req)
	})
	if err != nil {
		return ChatResponse{}, err
	}

	if res.StatusCode >= 400 {
		respBody, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return ChatResponse{}, fmt.Errorf(
			"bedrock: status %d: %s",
			res.StatusCode,
			string(respBody),
		)
	}

	if config.Cfg.StreamResponses {
		defer res.Body.Close()
		return parseBedrockStreamResponse(res.Body, model)
	}

	respBody, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return ChatResponse{}, err
	}

	var parsed bedrockConverseResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return ChatResponse{}, err
	}

	var textParts []string
	var toolCalls []ToolCall

	for i, block := range parsed.Output.Message.Content {
		if block.Text != nil {
			textParts = append(textParts, *block.Text)
		}
		if block.ToolUse != nil {
			args := string(block.ToolUse.Input)
			if args == "" {
				args = "{}"
			}
			toolCalls = append(toolCalls, ToolCall{
				Type:  "function",
				Index: i,
				ID:    block.ToolUse.ToolUseId,
				Function: ToolCallFunction{
					Name:      block.ToolUse.Name,
					Arguments: args,
				},
			})
		}
	}

	content := bedrockThinkingTagRe.ReplaceAllString(
		strings.Join(textParts, ""),
		"",
	)

	return ChatResponse{
		Model:      model,
		StopReason: parsed.StopReason,
		Usage: Usage{
			InputTokens:  parsed.Usage.InputTokens,
			OutputTokens: parsed.Usage.OutputTokens,
		},
		Message: Message{
			Role:      "assistant",
			Content:   content,
			ToolCalls: toolCalls,
		},
	}, nil
}

// parseBedrockStreamResponse decodes the AWS Event Stream binary protocol
// returned by the /converse-stream endpoint.
//
// Frame layout:
//
//	[total_len:4][headers_len:4][prelude_crc:4][headers:N][payload:M][msg_crc:4]
//
// total_len covers all bytes including itself and msg_crc.
// payload_len = total_len - 16 - headers_len
func parseBedrockStreamResponse(
	r io.Reader,
	model string,
) (ChatResponse, error) {
	type pendingTool struct {
		id    string
		name  string
		input strings.Builder
		index int
	}

	var text strings.Builder
	var toolCalls []ToolCall
	var stopReason string
	var inputTokens, outputTokens int
	pendingTools := map[int]*pendingTool{}

	for {
		var prelude [12]byte
		if _, err := io.ReadFull(r, prelude[:]); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return ChatResponse{}, fmt.Errorf(
				"bedrock stream: read prelude: %w",
				err,
			)
		}

		totalLen := binary.BigEndian.Uint32(prelude[0:4])
		headersLen := binary.BigEndian.Uint32(prelude[4:8])

		// remaining = headers + payload + msg_crc(4)
		remaining := int(totalLen) - 12
		if remaining < 4 {
			continue
		}
		rest := make([]byte, remaining)
		if _, err := io.ReadFull(r, rest); err != nil {
			return ChatResponse{}, fmt.Errorf(
				"bedrock stream: read frame: %w",
				err,
			)
		}

		headers := rest[:headersLen]
		// payload sits between headers and the trailing 4-byte msg CRC
		payloadEnd := remaining - 4
		if int(headersLen) >= payloadEnd {
			continue
		}
		payload := rest[headersLen:payloadEnd]

		eventType := bedrockEventType(headers)
		switch eventType {
		case "contentBlockStart":
			var ev struct {
				ContentBlockIndex int `json:"contentBlockIndex"`
				Start             struct {
					ToolUse struct {
						ToolUseId string `json:"toolUseId"`
						Name      string `json:"name"`
					} `json:"toolUse"`
				} `json:"start"`
			}
			if err := json.Unmarshal(payload, &ev); err != nil {
				continue
			}
			if ev.Start.ToolUse.ToolUseId != "" {
				pendingTools[ev.ContentBlockIndex] = &pendingTool{
					id:    ev.Start.ToolUse.ToolUseId,
					name:  ev.Start.ToolUse.Name,
					index: ev.ContentBlockIndex,
				}
			}

		case "contentBlockDelta":
			var ev struct {
				ContentBlockIndex int `json:"contentBlockIndex"`
				Delta             struct {
					Text    string `json:"text"`
					ToolUse struct {
						Input string `json:"input"`
					} `json:"toolUse"`
				} `json:"delta"`
			}
			if err := json.Unmarshal(payload, &ev); err != nil {
				continue
			}
			text.WriteString(ev.Delta.Text)
			if pt, ok := pendingTools[ev.ContentBlockIndex]; ok {
				pt.input.WriteString(ev.Delta.ToolUse.Input)
			}

		case "contentBlockStop":
			var ev struct {
				ContentBlockIndex int `json:"contentBlockIndex"`
			}
			if err := json.Unmarshal(payload, &ev); err != nil {
				continue
			}
			if pt, ok := pendingTools[ev.ContentBlockIndex]; ok {
				args := pt.input.String()
				if args == "" {
					args = "{}"
				}
				toolCalls = append(toolCalls, ToolCall{
					Type:  "function",
					Index: pt.index,
					ID:    pt.id,
					Function: ToolCallFunction{
						Name:      pt.name,
						Arguments: args,
					},
				})
				delete(pendingTools, ev.ContentBlockIndex)
			}

		case "messageStop":
			var ev struct {
				StopReason string `json:"stopReason"`
			}
			if err := json.Unmarshal(payload, &ev); err != nil {
				continue
			}
			stopReason = ev.StopReason

		case "metadata":
			var ev struct {
				Usage struct {
					InputTokens  int `json:"inputTokens"`
					OutputTokens int `json:"outputTokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal(payload, &ev); err != nil {
				continue
			}
			inputTokens = ev.Usage.InputTokens
			outputTokens = ev.Usage.OutputTokens
		}
	}

	content := bedrockThinkingTagRe.ReplaceAllString(text.String(), "")
	return ChatResponse{
		Model:      model,
		StopReason: stopReason,
		Usage: Usage{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		},
		Message: Message{
			Role:      "assistant",
			Content:   content,
			ToolCalls: toolCalls,
		},
	}, nil
}

// bedrockEventType extracts the :event-type header value from raw AWS Event
// Stream header bytes.
// Header encoding: [name_len:1][name:N][value_type:1][value_len:2][value:M]
// value_type 7 = string
func bedrockEventType(headers []byte) string {
	i := 0
	for i < len(headers) {
		nameLen := int(headers[i])
		i++
		if i+nameLen > len(headers) {
			break
		}
		name := string(headers[i : i+nameLen])
		i += nameLen
		if i+3 > len(headers) {
			break
		}
		valueType := headers[i]
		i++
		valueLen := int(binary.BigEndian.Uint16(headers[i : i+2]))
		i += 2
		if i+valueLen > len(headers) {
			break
		}
		value := string(headers[i : i+valueLen])
		i += valueLen
		if name == ":event-type" && valueType == 7 {
			return value
		}
	}
	return ""
}

func (p *Bedrock) Models() []ModelDescriptor {
	entries := []struct {
		id            string
		displayName   string
		contextWindow int
		inputCost     float64
		outputCost    float64
	}{
		{"minimax.minimax-m2.5", "MiniMax M2.5", 196000, 0.30, 1.20},
		{"zai.glm-5", "GLM 5", 203000, 1.00, 3.20},
		{
			"global.anthropic.claude-sonnet-4-6",
			"Claude Opus 4.6",
			1000000,
			3.00,
			15.00,
		},
		{
			"anthropic.claude-3-5-sonnet-20241022-v2:0",
			"Claude 3.5 Sonnet v2",
			200_000,
			3.00,
			15.00,
		},
		{
			"anthropic.claude-3-5-haiku-20241022-v1:0",
			"Claude 3.5 Haiku",
			200_000,
			0.80,
			4.00,
		},
		{
			"anthropic.claude-3-opus-20240229-v1:0",
			"Claude 3 Opus",
			200_000,
			15.00,
			75.00,
		},
		{"amazon.nova-pro-v1:0", "Amazon Nova Pro", 300_000, 0.80, 3.20},
		{"amazon.nova-lite-v1:0", "Amazon Nova Lite", 300_000, 0.06, 0.24},
		{"amazon.nova-micro-v1:0", "Amazon Nova Micro", 128_000, 0.035, 0.14},
		{
			"meta.llama3-3-70b-instruct-v1:0",
			"Llama 3.3 70B Instruct",
			128_000,
			0.72,
			0.72,
		},
		{
			"mistral.mistral-large-2402-v1:0",
			"Mistral Large",
			32_000,
			4.00,
			12.00,
		},
	}
	out := make([]ModelDescriptor, len(entries))
	for i, e := range entries {
		out[i] = ModelDescriptor{
			ID:                   e.id,
			DisplayName:          e.displayName,
			ProviderName:         string(BedrockProvider),
			ContextWindow:        e.contextWindow,
			InputCostPerMillion:  e.inputCost,
			OutputCostPerMillion: e.outputCost,
		}
	}
	return out
}

func (p *Bedrock) IsQuotaError(resp *http.Response, body []byte) bool {
	if resp.StatusCode != http.StatusTooManyRequests {
		return false
	}
	var parsed struct {
		Type string `json:"__type"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false
	}
	return strings.Contains(parsed.Type, "ThrottlingException") ||
		strings.Contains(parsed.Type, "ServiceQuotaExceededException")
}

// ---- Message conversion ----

func convertMessagesToBedrock(msgs []Message) (string, []bedrockMessage) {
	var system string
	var out []bedrockMessage

	for _, m := range msgs {
		switch m.Role {
		case "system":
			if system != "" {
				system += "\n\n"
			}
			system += m.Content

		case "tool":
			status := ""
			if m.Content == "" {
				status = "error"
			}
			block := bedrockContentBlock{
				ToolResult: &bedrockToolResult{
					ToolUseId: m.ToolCallId,
					Content:   []bedrockToolResultBlock{{Text: m.Content}},
					Status:    status,
				},
			}
			// Bedrock requires all tool results for an assistant turn to be
			// in a single user message — merge into the previous one if it
			// already holds tool results, otherwise start a new one.
			if len(out) > 0 && out[len(out)-1].Role == "user" &&
				len(out[len(out)-1].Content) > 0 &&
				out[len(out)-1].Content[0].ToolResult != nil {
				out[len(out)-1].Content = append(out[len(out)-1].Content, block)
			} else {
				out = append(out, bedrockMessage{
					Role:    "user",
					Content: []bedrockContentBlock{block},
				})
			}

		case "assistant":
			var blocks []bedrockContentBlock
			if m.Content != "" {
				text := m.Content
				blocks = append(blocks, bedrockContentBlock{Text: &text})
			}
			for _, tc := range m.ToolCalls {
				input := json.RawMessage(tc.Function.Arguments)
				if len(input) == 0 {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, bedrockContentBlock{
					ToolUse: &bedrockToolUse{
						ToolUseId: tc.ID,
						Name:      tc.Function.Name,
						Input:     input,
					},
				})
			}
			if len(blocks) > 0 {
				out = append(
					out,
					bedrockMessage{Role: "assistant", Content: blocks},
				)
			}

		default:
			text := m.Content
			out = append(out, bedrockMessage{
				Role:    "user",
				Content: []bedrockContentBlock{{Text: &text}},
			})
		}
	}

	return system, out
}

func convertToolsToBedrock(in []tools.Tool) *bedrockToolConfig {
	if len(in) == 0 {
		return nil
	}
	bedrockTools := make([]bedrockTool, 0, len(in))
	for _, t := range in {
		bedrockTools = append(bedrockTools, bedrockTool{
			ToolSpec: bedrockToolSpec{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				InputSchema: bedrockInputSchema{JSON: t.Function.Parameters},
			},
		})
	}
	return &bedrockToolConfig{Tools: bedrockTools}
}

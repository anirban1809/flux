package agent

import (
	"encoding/json"
	"errors"
	"flux/src/config"
	"flux/src/events"
	llm "flux/src/llm/provider"
	"flux/src/secrets"
	"flux/src/tools"
	"flux/src/utils"
	"fmt"
	"os"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

type Executor struct {
	EventChannel    chan events.ResponseEvent
	MessageChannel  chan string
	SystemPrompt    string
	Tools           []tools.Tool
	SubAgentRunning bool
	SubAgent        string
	ActiveSkill     string
	PlanActive      bool
}

func (e *Executor) IsSubagentTool(name string) bool {
	return strings.HasPrefix(name, "subagent")
}

func (e *Executor) IsSkillTool(name string) bool {
	return name == "invoke_skill"
}

func (e *Executor) IsPlanTool(name string) bool {
	return name == "create_plan"
}

func NewExecutor(systemPrompt string, tools []tools.Tool) *Executor {
	return &Executor{
		EventChannel:   make(chan events.ResponseEvent, 512),
		MessageChannel: make(chan string),
		SystemPrompt:   systemPrompt,
	}
}

type ExecutionResultStatus int

const (
	ExecutionSucceeded ExecutionResultStatus = iota
	ExecutionFailed
	ExecutionCancelled
	ExecutionCompleted
)

type RequestType string

const (
	RequestTask       RequestType = "task"
	RequestToolResult RequestType = "tool_result"
	RequestMessage    RequestType = "message"
)

type ResponseType string

const (
	ResponseToolCall ResponseType = "tool_call"
	ResponseMessage  ResponseType = "message"
	ResponseFinish   ResponseType = "finish"
)

type ToolResultRequestData struct {
	ToolCallID string `json:"tool_call_id"`
	Role       string `json:"role"`
	Content    string `json:"content"`
}

type ToolCallResponseData struct {
	Id        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ExecutionActionType string

const (
	ActionMessage  ExecutionActionType = "message"
	ActionToolCall ExecutionActionType = "tool_call"
	ActionSubagent ExecutionActionType = "subagent"
	ActionSkill    ExecutionActionType = "skill"
	ActionPlan     ExecutionActionType = "plan"
	ActionComplete ExecutionActionType = "complete"
)

type ExecutionAction struct {
	Type     ExecutionActionType
	Message  *llm.Message
	ToolCall *ToolCallResponseData
}

func (e *Executor) SetSubAgentModeOn(mode bool, name string) {
	e.SubAgent = name
	e.SubAgentRunning = mode
}

func (e *Executor) SetActiveSkill(name string) {
	e.ActiveSkill = name
}

func (e *Executor) SetPlanActive(active bool) {
	e.PlanActive = active
}

func (e *Executor) EmitMessage(content string) {
	e.pushEvent(events.Message, content)
}

func (e *Executor) ProcessResponse(
	response llm.Message,
) ([]ExecutionAction, ExecutionResultStatus, error) {
	if response.ToolCalls == nil && strings.TrimSpace(response.Content) == "" {
		return []ExecutionAction{
			{
				Type:    ActionMessage,
				Message: &llm.Message{Role: "user", Content: "retry"},
			},
		}, ExecutionSucceeded, nil
	}

	if response.ToolCalls == nil && strings.TrimSpace(response.Content) != "" {
		if !e.SubAgentRunning && !e.PlanActive {
			e.pushEvent(events.Message, response.Content)
		}
		return nil, ExecutionCompleted, nil
	}

	if len(response.ToolCalls) > 0 {
		results := []ExecutionAction{}

		for _, toolCall := range response.ToolCalls {
			tool := ToolCallResponseData{
				Id:        toolCall.ID,
				Name:      toolCall.Function.Name,
				Arguments: json.RawMessage(toolCall.Function.Arguments),
			}

			actionType := ActionToolCall

			if e.IsSubagentTool(tool.Name) {
				actionType = ActionSubagent
			}

			if e.IsSkillTool(tool.Name) {
				actionType = ActionSkill
			}

			if e.IsPlanTool(tool.Name) {
				actionType = ActionPlan
			}

			results = append(
				results,
				ExecutionAction{Type: actionType, ToolCall: &tool},
			)

		}

		return results, ExecutionSucceeded, nil
	}

	return nil, ExecutionFailed, errors.New("invalid response type")
}

func (e *Executor) pushEvent(eventType events.ResponseEventType, value string) {
	if config.Cfg.Headless {
		return
	}
	event := events.ResponseEvent{
		EventType:    eventType,
		Message:      secrets.RedactForDisplay(value),
		SubAgent:     e.SubAgentRunning,
		SubAgentName: e.SubAgent,
		SkillName:    e.ActiveSkill,
	}
	if config.Cfg.DaemonMode {
		select {
		case e.EventChannel <- event:
		default:
		}
		return
	}
	events.EventManager.WriteToChannel(events.AGENT_OUTPUT_CHANNEL, event)
}

func GetTool(path string, toolname string) (tools.Tool, error) {
	name := strings.ReplaceAll(toolname, "_tool", "")
	content, err := os.ReadFile(fmt.Sprintf("%s/%s/%s.json", path, name, name))

	if err != nil {
		return tools.Tool{}, errors.New("failed to read tool manifest")
	}

	var tool tools.Tool
	err = json.Unmarshal([]byte(content), &tool)

	if err != nil {
		return tools.Tool{}, errors.New("invalid tool manifest")
	}

	return tool, nil
}

func (e *Executor) GetToolCallCommand(
	input ToolCallResponseData,
) (string, []string, error) {
	internaltool, err1 := GetTool(config.Cfg.InternalToolPath, input.Name)
	externaltool, err2 := GetTool(config.Cfg.ExternalToolPath, input.Name)

	if err1 != nil && err2 != nil {
		return "", []string{}, errors.New("failed to get tool")
	}

	var tool tools.Tool
	var toolPath string
	if err1 == nil {
		tool = internaltool
		toolPath = config.Cfg.InternalToolPath
	} else {
		tool = externaltool
		toolPath = config.Cfg.ExternalToolPath
	}

	command := fmt.Sprintf(
		"python3 %s/%s/%s.py",
		toolPath,
		input.Name,
		input.Name,
	)

	var rawArgs map[string]any
	if err := json.Unmarshal(input.Arguments, &rawArgs); err != nil {
		return "", []string{}, fmt.Errorf("invalid arguments for %s: %w", input.Name, err)
	}

	argValues := []string{}

	for _, param := range tool.Function.Parameters.Required {
		var arg string
		switch v := rawArgs[param].(type) {
		case string:
			arg = strings.ReplaceAll(v, `"`, `\"`)
		case nil:
			// missing required param — pass empty string
		default:
			b, _ := json.Marshal(v)
			arg = strings.ReplaceAll(string(b), `"`, `\"`)
		}
		argValues = append(argValues, arg)

		command = fmt.Sprintf(
			"%s --%s \"%s\"",
			command,
			param,
			arg,
		)
	}

	if input.Name == "bash" {
		argValues = argValues[1 : len(argValues)-1]
	} else {
		argValues = argValues[1:]
	}

	return command, argValues, nil
}

func (e *Executor) ProcessToolCall(
	input ToolCallResponseData,
) (*ToolResultRequestData, error) {
	// toolError wraps an error as a tool-result message so the model can
	// observe and self-correct, rather than crashing the agent loop.
	toolError := func(format string, args ...any) *ToolResultRequestData {
		return &ToolResultRequestData{
			ToolCallID: input.Id,
			Role:       "tool",
			Content:    "error: " + fmt.Sprintf(format, args...),
		}
	}

	switch input.Name {
	default:
		command, params, err := e.GetToolCallCommand(input)
		if err != nil {
			return &ToolResultRequestData{
				ToolCallID: input.Id,
				Role:       "tool",
				Content:    fmt.Sprintf("unknown tool: %s", input.Name),
			}, nil
		}

		utils.Log(command)

		var args map[string]any

		if err := json.Unmarshal(input.Arguments, &args); err != nil {
			return toolError(
				"invalid arguments for %s: %s",
				input.Name,
				err,
			), nil
		}

		if msg, ok := args["message"].(string); ok {
			e.pushEvent(
				events.Tool,
				fmt.Sprintf(
					"%s\n%s: %s",
					msg,
					input.Name,
					strings.Join(params, ","),
				),
			)
		}

		result, err := tools.RunBashCommand(command)
		utils.Log(result)

		if err != nil {
			utils.Log(err.Error())
			return toolError("%s failed: %s", input.Name, err), nil
		}

		return &ToolResultRequestData{
			ToolCallID: input.Id,
			Role:       "tool",
			Content:    string(result),
		}, nil

	case "file_write":
		var fileWriteInput tools.FileWriteInput
		err := json.Unmarshal(input.Arguments, &fileWriteInput)
		if err != nil {
			return toolError("invalid arguments for file_write: %s", err), nil
		}

		var msg string
		var patches []tools.ParsedDiff

		for _, p := range fileWriteInput.Patches {
			diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
				A:        difflib.SplitLines(p.Target),
				B:        difflib.SplitLines(p.Content),
				FromFile: fileWriteInput.FilePath,
				ToFile:   fileWriteInput.FilePath,
				Context:  3,
			})

			parsedDiff, _ := tools.ParseUnifiedDiff(diff)
			patches = append(patches, parsedDiff)
		}

		var changeType events.FileChangeType

		switch fileWriteInput.Operation {
		case "append":
			changeType = events.FileChange_Append

		case "create":
			changeType = events.FileChange_Create

		case "patch":
			changeType = events.FileChange_Patch

		}

		if config.Cfg.Headless || config.Cfg.YoloMode || config.Cfg.DaemonMode {
			msg = "Yes"
		} else {
			events.EventManager.WriteToChannel(
				events.FILE_DIFF_CHANNEL,
				events.FileChangeEvent{
					FileName:   fileWriteInput.FilePath,
					ChangeType: changeType,
					Content:    fileWriteInput.Content,
					Patches:    patches,
				},
			)

			events.EventManager.WriteToChannel(
				events.AGENT_OUTPUT_CHANNEL,
				events.ResponseEvent{
					Question:  "Do you want to make this change?",
					Options:   []string{"Yes", "No"},
					EventType: events.Tool,
					Message:   fileWriteInput.Message,
				},
			)

			msg = events.EventManager.ReadFromChannel(events.AGENT_INPUT_CHANNEL).(string)
		}

		if msg == "Yes" || msg == "Yes, and do not ask again for this session" {
			output, err := tools.RunFileWrite(fileWriteInput)

			if err != nil {
				return toolError("file_write failed: %s", err), nil
			}
			value, err := json.Marshal(output)
			if err != nil {
				return toolError(
					"file_write succeeded but result could not be encoded: %s",
					err,
				), nil
			}

			return &ToolResultRequestData{
				ToolCallID: input.Id,
				Role:       "tool",
				Content:    string(value),
			}, nil
		}

		return &ToolResultRequestData{
			ToolCallID: input.Id,
			Role:       "tool",
			Content:    "denied",
		}, nil

	case "question":
		if config.Cfg.Headless || config.Cfg.DaemonMode {
			return &ToolResultRequestData{
				ToolCallID: input.Id,
				Role:       "tool",
				Content:    `{"error":"question tool is not available in this mode"}`,
			}, nil
		}

		var questionInput tools.QuestionInput
		err := json.Unmarshal(input.Arguments, &questionInput)
		if err != nil {
			return toolError("invalid arguments for question: %s", err), nil
		}

		if len(questionInput.Options) < 2 {
			return &ToolResultRequestData{
				ToolCallID: input.Id,
				Role:       "tool",
				Content:    `{"error":"question tool requires at least two options"}`,
			}, nil
		}

		events.EventManager.WriteToChannel(
			events.AGENT_OUTPUT_CHANNEL,
			events.ResponseEvent{
				Question:  questionInput.Question,
				Options:   questionInput.Options,
				EventType: events.Tool,
				Message:   questionInput.Question,
			},
		)

		answer := events.EventManager.ReadFromChannel(events.AGENT_INPUT_CHANNEL).(string)

		payload, err := json.Marshal(map[string]string{"selected": answer})
		if err != nil {
			return toolError(
				"question result could not be encoded: %s",
				err,
			), nil
		}

		return &ToolResultRequestData{
			ToolCallID: input.Id,
			Role:       "tool",
			Content:    string(payload),
		}, nil

	}

}

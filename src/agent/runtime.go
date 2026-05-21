package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"zipcode/src/config"
	"zipcode/src/credentials"
	"zipcode/src/events"
	"zipcode/src/llm/prompts"
	llm "zipcode/src/llm/provider"
	"zipcode/src/skills"
	"zipcode/src/tools"
	"zipcode/src/utils"
	"zipcode/src/workspace"
)

// Represents the current status of the runtime
type RuntimeStatus int

const (
	Idle RuntimeStatus = iota
	Running
	Cancelled
)

// AutoCompactThreshold is the input-token count above which Run will
// trigger a context compaction before processing the next prompt.
const AutoCompactThreshold = 200000

// traceTurn writes one breadcrumb line to stderr in headless mode so a
// silent timeout leaves an actionable trail.
func traceTurn(turn int, format string, args ...any) {
	if !config.Cfg.Headless {
		return
	}
	fmt.Fprintf(
		os.Stderr,
		"[zipcode] turn %d: %s\n",
		turn,
		fmt.Sprintf(format, args...),
	)
}

const compactSummarizationPrompt = "Provide a concise but thorough summary of the conversation above. Preserve key context, decisions made, files touched, user preferences expressed, and the current state of any in-progress work. The summary will replace the full transcript so that the conversation can continue without losing context."

type RuntimeEvent string

type Runtime struct {
	Prompt            string
	Executor          *Executor
	Status            RuntimeStatus
	Registry          llm.Registry
	CurrentProvider   llm.Provider
	Workspace         *workspace.Workspace
	Tools             []tools.Tool
	InputTokens       int
	CachedInputTokens int
	CacheWriteTokens  int
	OutputTokens      int
	Conversation      llm.Conversation
	Agent             Agent
	Session           string
	ChildRuntime      bool
	SkillRegistry     *skills.SkillRegistry
	SkillWatcher      *skills.Watcher
	CredStore         *credentials.Store
	Validator         credentials.Validator
	activePlan        *Plan
}

func NewRuntime(workspace *workspace.Workspace) Runtime {
	registry, watcher, _ := skills.Init(
		config.Cfg.InternalSkillsPath,
		config.Cfg.GlobalSkillsPath,
		config.Cfg.ProjectSkillsPath,
		config.Cfg.SkillsStatePath,
	)

	runtime := Runtime{
		Status:    Idle,
		Registry:  llm.NewRegistry(),
		Workspace: workspace,
		Executor: NewExecutor(prompts.MainSystemPrompt,
			[]tools.Tool{
				tools.FileWriteTool,
				tools.SubAgentTool,
				tools.InvokeSkillTool,
				tools.CreatePlanTool,
			},
		),
		SkillRegistry: registry,
		SkillWatcher:  watcher,
		CredStore:     credentials.NewStore(),
	}

	err := runtime.CredStore.Load()

	if err != nil {
		go events.EventManager.WriteToChannel(
			events.NOTIFICATION_CHANNEL,
			events.Notification{
				Type: events.ERROR,
				Message: fmt.Sprintf(
					"Failed to load credentials. Error: %s",
					err.Error(),
				),
			},
		)
	}

	runtime.Validator = *credentials.NewValidator(runtime.Registry.Providers, runtime.CredStore)

	for name, prov := range runtime.Registry.Providers {
		if creds, ok := runtime.CredStore.Get(name); ok {
			prov.SetApiKey(creds.APIKey)
		}
	}

	if config.Cfg.ActiveProviderName != "" {
		if canonical, err := llm.GetProviderName(config.Cfg.ActiveProviderName); err == nil {
			config.Cfg.ActiveProviderName = string(canonical)
		}
		active := runtime.Registry.GetProvider(
			llm.ProviderName(config.Cfg.ActiveProviderName),
		)
		runtime.CurrentProvider = active
		if saved, ok := config.Cfg.ProviderModels[config.Cfg.ActiveProviderName]; ok &&
			saved != "" {
			config.Cfg.CurrentModel = saved
		} else if active != nil {
			if models := active.Models(); len(models) > 0 {
				config.Cfg.CurrentModel = models[0].ID
				if config.Cfg.ProviderModels == nil {
					config.Cfg.ProviderModels = map[string]string{}
				}
				config.Cfg.ProviderModels[config.Cfg.ActiveProviderName] = models[0].ID
				config.Cfg.Save()
			}
		}
	}

	if workspace != nil && workspace.Session != nil {
		runtime.Session = workspace.Session.ID
	}

	runtime.Agent = NewAgent(
		prompts.MainSystemPrompt,
		&runtime.Tools,
		&runtime.Registry,
		&runtime.Validator,
	)
	runtime.Tools = append(
		runtime.Tools,
		tools.FileWriteTool,
		tools.InvokeSkillTool,
		tools.CreatePlanTool,
	)
	if !config.Cfg.Headless {
		runtime.Tools = append(runtime.Tools, tools.QuestionTool)
	}
	runtime.loadTools(config.Cfg.InternalToolPath)
	runtime.loadTools(config.Cfg.ExternalToolPath)

	return runtime
}

func (r Runtime) GetExecutorEventChannel() chan events.ResponseEvent {
	return r.Executor.EventChannel
}

func (r Runtime) GetExecutorMessageChannel() chan string {
	return r.Executor.MessageChannel
}

func (r *Runtime) SetSession(session *workspace.Session) {
	if session == nil {
		return
	}
	r.Session = session.ID
	if r.Workspace != nil {
		r.Workspace.Session = session
	}
	r.Agent.RestoreConversation(session.Messages)
}

func (r *Runtime) persistSessionHistory() {
	if r.Workspace == nil || r.Workspace.Session == nil {
		return
	}
	messages := r.Agent.Conversation.Messages
	if len(messages) > 0 && messages[0].Role == "system" {
		messages = messages[1:]
	}
	r.Workspace.Session.Messages = messages
	_ = r.Workspace.Session.Save()
}

// maybeAutoCompact triggers a compaction if the most recent API call's input
// token count exceeded AutoCompactThreshold. Failures are surfaced as
// notifications so the run can continue with the uncompacted history.
func (r *Runtime) maybeAutoCompact() {
	if r.ChildRuntime {
		return
	}
	if r.Agent.Conversation.Usage.InputTokens <= AutoCompactThreshold {
		return
	}

	go events.EventManager.WriteToChannel(
		events.NOTIFICATION_CHANNEL,
		events.Notification{
			Type:    events.INFO,
			Message: "Context exceeded auto-compact threshold; compacting conversation...",
		},
	)

	if _, err := r.Compact(); err != nil {
		go events.EventManager.WriteToChannel(
			events.NOTIFICATION_CHANNEL,
			events.Notification{
				Type: events.ERROR,
				Message: fmt.Sprintf(
					"Auto-compact failed: %s",
					err.Error(),
				),
			},
		)
		return
	}

	go events.EventManager.WriteToChannel(
		events.NOTIFICATION_CHANNEL,
		events.Notification{
			Type:    events.INFO,
			Message: "Conversation compacted.",
		},
	)
}

// Clear resets the conversation history back to just the system prompt and
// zeroes accumulated token counters. The session file is updated to match.
func (r *Runtime) Clear() {
	systemPrompt := r.Agent.SystemPrompt
	r.Agent.Conversation.Messages = []llm.Message{
		{Role: "system", Content: systemPrompt},
	}
	r.Agent.Conversation.Usage = llm.Usage{}
	r.InputTokens = 0
	r.CachedInputTokens = 0
	r.CacheWriteTokens = 0
	r.OutputTokens = 0
	r.persistSessionHistory()
}

// Compact asks the active provider to summarize the current conversation and
// replaces the message history with that summary, preserving the system prompt.
// Returns the summary text (or an empty string and error if compaction fails).
func (r *Runtime) Compact() (string, error) {
	if config.Cfg.ActiveProviderName == "" {
		return "", fmt.Errorf("no active provider configured")
	}

	provider := r.Registry.GetProvider(
		llm.ProviderName(config.Cfg.ActiveProviderName),
	)
	if provider == nil {
		return "", fmt.Errorf(
			"provider %q not registered",
			config.Cfg.ActiveProviderName,
		)
	}

	msgs := r.Agent.Conversation.Messages
	if len(msgs) <= 1 {
		return "", nil
	}

	requestMessages := append([]llm.Message{}, msgs...)
	requestMessages = append(requestMessages, llm.Message{
		Role:    "user",
		Content: compactSummarizationPrompt,
	})

	resp, err := provider.Complete(llm.ChatRequest{
		Model:    config.Cfg.CurrentModel,
		Messages: requestMessages,
	})
	if err != nil {
		return "", err
	}

	summary := strings.TrimSpace(resp.Message.Content)
	if summary == "" {
		return "", fmt.Errorf("provider returned empty summary")
	}

	r.Agent.Conversation.Messages = []llm.Message{
		{Role: "system", Content: r.Agent.SystemPrompt},
		{
			Role:    "user",
			Content: "Summary of prior conversation:\n\n" + summary,
		},
		{
			Role:    "assistant",
			Content: "Understood. I have the summary of our prior conversation and will continue from here.",
		},
	}
	r.Agent.Conversation.Usage = llm.Usage{}
	r.InputTokens = 0
	r.CachedInputTokens = 0
	r.CacheWriteTokens = 0
	r.OutputTokens = 0

	r.persistSessionHistory()
	return summary, nil
}

type SubAgentRequest struct {
	AgentName   string
	AgentPrompt string
}

type SubAgent struct {
	Name             string   `json:"name"`
	ShortDescription string   `json:"short_description"`
	SystemPrompt     string   `json:"system_prompt"`
	AllowedTools     []string `json:"allowed_tools"`
}

func GetToolsforSubAgent(toolNames []string) ([]tools.Tool, error) {
	allowedTools := []tools.Tool{}

	for _, toolName := range toolNames {
		toolManifest, err := GetTool(config.Cfg.InternalToolPath, toolName)
		if err != nil {
			return nil, err
		}

		allowedTools = append(allowedTools, toolManifest)
	}

	return allowedTools, nil
}

func (r *Runtime) NewChildRuntime(
	agentName string,
	parent *Runtime,
) (*Runtime, error) {
	content, err := os.ReadFile(
		fmt.Sprintf("%s/%s.json", config.Cfg.InternalSubagentsPath, agentName),
	)
	if err != nil {
		return nil, err
	}

	var subAgentDefinition SubAgent
	err = json.Unmarshal(content, &subAgentDefinition)
	if err != nil {
		return nil, err
	}

	tools, err := GetToolsforSubAgent(subAgentDefinition.AllowedTools)

	runtime := &Runtime{
		Status:        Idle,
		Workspace:     r.Workspace,
		Executor:      r.Executor,
		Session:       r.Session,
		ChildRuntime:  true,
		SkillRegistry: r.SkillRegistry,
		Registry:      parent.Registry,
	}

	childAgent := NewAgent(
		subAgentDefinition.SystemPrompt,
		&tools,
		&runtime.Registry,
		&r.Validator,
	)

	runtime.Agent = childAgent

	return runtime, nil
}

func (r *Runtime) loadTools(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			content, err := os.ReadFile(
				fmt.Sprintf("%s/%s/%s.json", path, entry.Name(), entry.Name()),
			)
			if err != nil {
				fmt.Println("Error reading file:", err)
				continue
			}

			var tool tools.Tool
			err = json.Unmarshal([]byte(content), &tool)

			r.Tools = append(r.Tools, tool)
		}
	}

	return nil
}

type SubagentToolArgs struct {
	AgentName string `json:"agent"`
	Task      string `json:"task"`
	Context   string `json:"context,omitempty"`
}

type SkillToolArgs struct {
	SkillName string `json:"skill_name"`
	Args      string `json:"args,omitempty"`
}

const stepPromptSystem = `You convert one step outline from a plan into a concrete instruction for an autonomous coding agent that operates in the same workspace.
Rules:
- Output ONLY the instruction text. No preamble, no markdown headings, no "Step N:" prefix.
- Make it self-contained — do not reference "the previous step" by index. Restate any concrete facts (paths, symbols, decisions) you need from prior output.
- Stay within this step's scope. Do not anticipate later steps.
- Be specific: name files, functions, or symbols rather than vague pronouns.`

func (r *Runtime) emitPlanStatus() {
	if r.activePlan == nil {
		return
	}
	events.EventManager.WriteToChannel(
		events.PLAN_STATUS_CHANNEL,
		r.activePlan.snapshot(),
	)
}

func (r *Runtime) generateStepPrompt(idx int) (string, error) {
	if r.activePlan == nil {
		return "", fmt.Errorf("no active plan")
	}
	plan := r.activePlan
	if idx < 0 || idx >= len(plan.Steps) {
		return "", fmt.Errorf("step index out of range")
	}
	if config.Cfg.ActiveProviderName == "" {
		return "", fmt.Errorf("no active provider configured")
	}
	provider := r.Registry.GetProvider(
		llm.ProviderName(config.Cfg.ActiveProviderName),
	)
	if provider == nil {
		return "", fmt.Errorf(
			"provider %q not registered",
			config.Cfg.ActiveProviderName,
		)
	}

	var sb strings.Builder
	sb.WriteString("Overall task title: ")
	sb.WriteString(plan.Title)
	sb.WriteString("\n\nPlan outline:\n")
	for i, s := range plan.Steps {
		marker := "  -"
		if i == idx {
			marker = "  >"
		}
		fmt.Fprintf(&sb, "%s %d. %s\n", marker, i+1, s.Outline)
	}
	if idx > 0 {
		prev := plan.Steps[idx-1]
		sb.WriteString("\nPrevious step (")
		sb.WriteString(prev.Outline)
		sb.WriteString(") output:\n")
		sb.WriteString(prev.Output)
		sb.WriteString("\n")
	}
	fmt.Fprintf(
		&sb,
		"\nWrite the concrete prompt for step %d (%q).",
		idx+1,
		plan.Steps[idx].Outline,
	)

	resp, err := provider.Complete(llm.ChatRequest{
		Model: config.Cfg.CurrentModel,
		Messages: []llm.Message{
			{Role: "system", Content: stepPromptSystem},
			{Role: "user", Content: sb.String()},
		},
	})
	if err != nil {
		return "", err
	}
	out := strings.TrimSpace(resp.Message.Content)
	if out == "" {
		return "", fmt.Errorf("provider returned empty step prompt")
	}
	return out, nil
}

func (r *Runtime) handlePlanAction(
	tc *ToolCallResponseData,
) (llm.Message, *string, error) {
	var args tools.CreatePlanInput
	if err := json.Unmarshal(tc.Arguments, &args); err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallId: tc.Id,
			Content: fmt.Sprintf(
				`{"success":false,"error":"invalid plan args: %s"}`,
				err.Error(),
			),
		}, nil, nil
	}
	if len(args.Steps) == 0 {
		return llm.Message{
			Role:       "tool",
			ToolCallId: tc.Id,
			Content:    `{"success":false,"error":"plan requires at least one step"}`,
		}, nil, nil
	}
	if r.activePlan != nil && r.activePlan.Active {
		return llm.Message{
			Role:       "tool",
			ToolCallId: tc.Id,
			Content:    `{"success":false,"error":"a plan is already active"}`,
		}, nil, nil
	}

	r.activePlan = newPlan(args.Title, args.Steps)
	r.emitPlanStatus()

	firstPrompt, err := r.generateStepPrompt(0)
	if err != nil {
		r.activePlan.Active = false
		r.emitPlanStatus()
		return llm.Message{
			Role:       "tool",
			ToolCallId: tc.Id,
			Content: fmt.Sprintf(
				`{"success":false,"error":"failed to generate first step prompt: %s"}`,
				err.Error(),
			),
		}, nil, nil
	}
	r.activePlan.Steps[0].Prompt = firstPrompt
	r.activePlan.Steps[0].Status = events.PlanStepRunning
	r.emitPlanStatus()

	ack := llm.Message{
		Role:       "tool",
		ToolCallId: tc.Id,
		Content: fmt.Sprintf(
			`{"success":true,"title":%q,"steps":%d}`,
			args.Title,
			len(args.Steps),
		),
	}
	return ack, &firstPrompt, nil
}

func (r *Runtime) InvokeSubAgent(
	tool ToolCallResponseData,
) (llm.Message, error) {
	var args SubagentToolArgs
	if err := json.Unmarshal(tool.Arguments, &args); err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallId: tool.Id,
			Content: fmt.Sprintf(
				`{"success":false,"error":"invalid subagent args: %s"}`,
				err.Error(),
			),
		}, nil
	}

	childRuntime, err := r.NewChildRuntime(args.AgentName, r)
	if err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallId: tool.Id,
			Content: fmt.Sprintf(
				`{"success":false,"error":"failed to create subagent: %s"}`,
				err.Error(),
			),
		}, nil
	}

	r.Executor.SetSubAgentModeOn(true, args.AgentName)
	output, err := childRuntime.Run(args.Task)
	r.Executor.SetSubAgentModeOn(false, "")
	if err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallId: tool.Id,
			Content: fmt.Sprintf(
				`{"success":false,"error":"subagent failed: %s"}`,
				err.Error(),
			),
		}, nil
	}

	result := map[string]any{
		"success":    true,
		"agent_type": args.AgentName,
		"output":     output,
	}

	content, _ := json.Marshal(result)

	return llm.Message{
		Role:       "tool",
		ToolCallId: tool.Id,
		Content:    string(content),
	}, nil
}

// ParseSkillCommand scans the prompt for a /skill-name token (anywhere, not
// just at the start) that resolves to a registered enabled skill. Returns the
// skill name and the rest of the prompt with the token removed; ok=false if
// no enabled skill is referenced.
func (r *Runtime) ParseSkillCommand(
	prompt string,
) (name, args string, ok bool) {
	if r.SkillRegistry == nil {
		return "", "", false
	}

	fields := strings.Fields(prompt)
	for i, tok := range fields {
		if !strings.HasPrefix(tok, "/") {
			continue
		}
		candidate := strings.TrimPrefix(tok, "/")
		if candidate == "" {
			continue
		}
		skill, found := r.SkillRegistry.Get(candidate)
		if !found || !skill.Enabled {
			continue
		}
		rest := append([]string{}, fields[:i]...)
		rest = append(rest, fields[i+1:]...)
		return skill.Name, strings.TrimSpace(strings.Join(rest, " ")), true
	}
	return "", "", false
}

func (r *Runtime) IsSkillCommand(prompt string) bool {
	_, _, ok := r.ParseSkillCommand(prompt)
	return ok
}

func (r *Runtime) ExpandSkillCommand(prompt string) string {
	name, args, ok := r.ParseSkillCommand(prompt)
	if !ok {
		return prompt
	}
	skill, found := r.SkillRegistry.Get(name)
	if !found {
		return prompt
	}
	return skills.Resolve(skill.PromptTemplate, args, r.Workspace)
}

func (r *Runtime) skillSummaries() []prompts.SkillSummary {
	if r.SkillRegistry == nil {
		return nil
	}
	enabled := r.SkillRegistry.ListEnabled()
	out := make([]prompts.SkillSummary, 0, len(enabled))
	for _, s := range enabled {
		out = append(
			out,
			prompts.SkillSummary{Name: s.Name, Description: s.Description},
		)
	}
	return out
}

func (r *Runtime) workspaceContext() prompts.WorkspaceContext {
	if r.Workspace == nil {
		return prompts.WorkspaceContext{}
	}
	return prompts.WorkspaceContext{
		RootPath: r.Workspace.RootPath,
		FileTree: r.Workspace.FileTreeSnapshot,
	}
}

func (r *Runtime) refreshSystemPrompt() {
	if r.ChildRuntime {
		return
	}
	prompt := prompts.BuildSystemPrompt(
		r.workspaceContext(),
		r.skillSummaries(),
	)
	r.Agent.SystemPrompt = prompt
	if len(r.Agent.Conversation.Messages) > 0 &&
		r.Agent.Conversation.Messages[0].Role == "system" {
		r.Agent.Conversation.Messages[0].Content = prompt
	}
}

func (r *Runtime) invokeSkill(
	tool ToolCallResponseData,
) (llm.Message, *string, error) {
	var args SkillToolArgs
	if err := json.Unmarshal(tool.Arguments, &args); err != nil {
		return llm.Message{
			Role:       "tool",
			ToolCallId: tool.Id,
			Content: fmt.Sprintf(
				`{"success":false,"error":"invalid skill args: %s"}`,
				err.Error(),
			),
		}, nil, nil
	}

	if r.SkillRegistry == nil {
		return llm.Message{
			Role:       "tool",
			ToolCallId: tool.Id,
			Content:    `{"success":false,"error":"skills not available"}`,
		}, nil, nil
	}

	skill, ok := r.SkillRegistry.Get(args.SkillName)
	if !ok || !skill.Enabled {
		return llm.Message{
			Role:       "tool",
			ToolCallId: tool.Id,
			Content: fmt.Sprintf(
				`{"success":false,"error":"unknown or disabled skill: %s"}`,
				args.SkillName,
			),
		}, nil, nil
	}

	resolved := skills.Resolve(skill.PromptTemplate, args.Args, r.Workspace)

	ack := llm.Message{
		Role:       "tool",
		ToolCallId: tool.Id,
		Content:    fmt.Sprintf(`{"success":true,"skill":"%s"}`, skill.Name),
	}
	return ack, &resolved, nil
}

func (r *Runtime) Run(prompt string) (*llm.Message, error) {
	r.Status = Running
	r.Prompt = prompt
	r.refreshSystemPrompt()

	r.maybeAutoCompact()

	conv, err := r.Agent.RunStep(llm.Message{
		Role:    "user",
		Content: prompt,
	})
	if err != nil {
		return nil, err
	}

	turn := 0
	maxTurns := config.Cfg.MaxHeadlessTurns
	if maxTurns <= 0 {
		maxTurns = 100
	}
	for r.Status != Idle {
		turn++
		if config.Cfg.Headless && turn > maxTurns {
			traceTurn(turn, "max turns reached (%d); aborting", maxTurns)
			return nil, fmt.Errorf(
				"max headless turns (%d) reached without completing the task",
				maxTurns,
			)
		}

		lastResponseIndex := len(conv.Messages) - 1
		lastResponse := conv.Messages[lastResponseIndex]
		actions, status, err := r.Executor.ProcessResponse(lastResponse)

		utils.Log(lastResponse.Content)

		if err != nil {
			return nil, err
		}

		switch {
		case status == ExecutionCompleted:
			traceTurn(turn, "complete (in=%d cached=%d out=%d)",
				r.InputTokens, r.CachedInputTokens, r.OutputTokens)
		case len(actions) == 1 && actions[0].Type == ActionMessage:
			traceTurn(turn, "empty model response, retrying")
		default:
			names := make([]string, 0, len(actions))
			for _, a := range actions {
				kind := string(a.Type)
				if a.ToolCall != nil {
					kind += ":" + a.ToolCall.Name
				}
				names = append(names, kind)
			}
			traceTurn(turn, "actions=%v", names)
		}

		if status == ExecutionCompleted {
			if r.activePlan != nil && r.activePlan.Active {
				cur := r.activePlan.Current
				if cur < len(r.activePlan.Steps) {
					r.activePlan.Steps[cur].Output = lastResponse.Content
					r.activePlan.Steps[cur].Status = events.PlanStepCompleted
					r.activePlan.Current++
					r.emitPlanStatus()
				}

				if r.activePlan.Current >= len(r.activePlan.Steps) {
					r.activePlan.Active = false
					r.emitPlanStatus()
					r.Status = Idle
					break
				}

				nextPrompt, perr := r.generateStepPrompt(r.activePlan.Current)
				if perr != nil {
					r.activePlan.Steps[r.activePlan.Current].Status = events.PlanStepFailed
					r.activePlan.Active = false
					r.emitPlanStatus()
					go events.EventManager.WriteToChannel(
						events.NOTIFICATION_CHANNEL,
						events.Notification{
							Type: events.ERROR,
							Message: fmt.Sprintf(
								"Plan step prompt generation failed: %s",
								perr.Error(),
							),
						},
					)
					r.Status = Idle
					break
				}
				r.activePlan.Steps[r.activePlan.Current].Prompt = nextPrompt
				r.activePlan.Steps[r.activePlan.Current].Status = events.PlanStepRunning
				r.emitPlanStatus()

				conv, err = r.Agent.RunStep(llm.Message{
					Role:    "user",
					Content: nextPrompt,
				})
				if err != nil {
					return nil, err
				}
				r.InputTokens += conv.Usage.InputTokens
				r.CachedInputTokens += conv.Usage.CachedInputTokens
				r.CacheWriteTokens += conv.Usage.CacheWriteTokens
				r.OutputTokens += conv.Usage.OutputTokens
				continue
			}

			r.Status = Idle
			break
		}

		messages := []llm.Message{}
		var pendingSkillPrompts []string
		var pendingSkillNames []string
		var pendingPlanPrompts []string

		for _, action := range actions {
			switch action.Type {
			case ActionToolCall:

				result, err := r.Executor.ProcessToolCall(*action.ToolCall)
				if err != nil {
					// Defensive net: any tool-handler that still returns
					// (nil, err) gets surfaced to the model as a tool
					// result instead of killing the run.
					result = &ToolResultRequestData{
						ToolCallID: action.ToolCall.Id,
						Role:       "tool",
						Content:    fmt.Sprintf("error: %s", err.Error()),
					}
				}

				messages = append(messages, llm.Message{
					Role:       result.Role,
					Content:    result.Content,
					ToolCallId: result.ToolCallID,
				})

			case ActionSubagent:
				var args SubagentToolArgs
				err := json.Unmarshal((*action.ToolCall).Arguments, &args)
				if err != nil {
					return nil, err
				}

				result, err := r.InvokeSubAgent(*action.ToolCall)
				if err != nil {
					return nil, err
				}

				messages = append(messages, result)

			case ActionSkill:
				ack, resolved, err := r.invokeSkill(*action.ToolCall)
				if err != nil {
					return nil, err
				}
				messages = append(messages, ack)

				if resolved != nil {
					var args SkillToolArgs
					_ = json.Unmarshal((*action.ToolCall).Arguments, &args)
					pendingSkillPrompts = append(pendingSkillPrompts, *resolved)
					pendingSkillNames = append(
						pendingSkillNames,
						args.SkillName,
					)
				}

			case ActionPlan:
				ack, firstPrompt, err := r.handlePlanAction(action.ToolCall)
				if err != nil {
					return nil, err
				}
				messages = append(messages, ack)
				if firstPrompt != nil {
					pendingPlanPrompts = append(
						pendingPlanPrompts,
						*firstPrompt,
					)
				}
			}
		}

		for i, body := range pendingSkillPrompts {
			messages = append(messages, llm.Message{
				Role:    "user",
				Content: body,
			})
			r.Executor.SetActiveSkill(pendingSkillNames[i])
		}

		for _, body := range pendingPlanPrompts {
			messages = append(messages, llm.Message{
				Role:    "user",
				Content: body,
			})
		}

		// Surface tool-result errors so a model stuck in a tool-error loop
		// is visible in the breadcrumb trail.
		toolErrs := 0
		for _, m := range messages {
			if m.Role == "tool" && strings.HasPrefix(m.Content, "error:") {
				toolErrs++
			}
		}
		if toolErrs > 0 {
			traceTurn(
				turn,
				"tool errors this turn: %d/%d",
				toolErrs,
				len(messages),
			)
		}

		conv, err = r.Agent.RunStep(messages...)
		if err != nil {
			return nil, err
		}

		r.InputTokens += conv.Usage.InputTokens
		r.CachedInputTokens += conv.Usage.CachedInputTokens
		r.CacheWriteTokens += conv.Usage.CacheWriteTokens
		r.OutputTokens += conv.Usage.OutputTokens
	}

	r.Conversation.Messages = append(r.Conversation.Messages, conv.Messages...)
	r.Conversation.Usage.InputTokens += r.InputTokens
	r.Conversation.Usage.OutputTokens += r.OutputTokens

	r.persistSessionHistory()
	r.Executor.SetActiveSkill("")

	return &conv.Messages[len(conv.Messages)-1], nil
}

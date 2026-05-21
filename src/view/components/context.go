package view

import (
	"encoding/json"
	"fmt"
	"strings"

	"flux/src/agent"
	"flux/src/config"
	llm "flux/src/llm/provider"

	"github.com/anirban1809/tuix/tuix"
)

const charsPerToken = 4

func estimateTokens(s string) int {
	return len(s) / charsPerToken
}

func formatTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.2fM", float64(n)/1_000_000)
}

// Context renders a breakdown of token usage for the current session. The
// "Used" / "Last response" / "Session output" figures are exact and come from
// per-message Usage saved in the session file. The category breakdown is a
// char/4 approximation — no local tokenizer is run.
func Context(props tuix.Props) tuix.Element {
	runtime := props.Get("runtime").(*agent.Runtime)

	if runtime == nil {
		return tuix.Box(
			tuix.Props{
				Direction: tuix.Column,
				Padding:   [4]int{1, 1, 1, 1},
			},
			tuix.NewStyle(),
			tuix.Text("Runtime not available.", tuix.NewStyle()),
			tuix.Text("Press Esc to go back.", tuix.NewStyle()),
		)
	}

	messages := sessionMessages(runtime)
	latestInput, latestOutput, sessionOutput := walkUsage(messages)

	systemPrompt := runtime.Agent.SystemPrompt
	sysTokens := estimateTokens(systemPrompt)
	wsTokens, skillsTokens := breakdownSystemPrompt(systemPrompt)

	toolJSON, _ := json.Marshal(runtime.Tools)
	toolTokens := estimateTokens(string(toolJSON))

	userTokens, asstTokens, toolMsgTokens := breakdownMessages(messages)
	msgTotal := userTokens + asstTokens + toolMsgTokens

	modelID := config.Cfg.CurrentModel
	ctxWindow := lookupContextWindow(runtime, modelID)

	lines := []string{
		"Context Usage",
		"",
		fmt.Sprintf("Model: %s", modelID),
	}
	if ctxWindow > 0 {
		pct := 0.0
		if latestInput > 0 {
			pct = float64(latestInput) / float64(ctxWindow) * 100
		}
		lines = append(lines,
			fmt.Sprintf(
				"Window: %s   Used: %s (%.1f%%)",
				formatTokens(ctxWindow),
				formatTokens(latestInput),
				pct,
			),
		)
	} else if latestInput > 0 {
		lines = append(lines, fmt.Sprintf("Used: %s tokens", formatTokens(latestInput)))
	} else {
		lines = append(lines, "(no usage recorded yet — send a prompt first)")
	}

	lines = append(lines,
		"",
		"Breakdown (approx, char/4):",
		fmt.Sprintf("  System prompt   ~%s", formatTokens(sysTokens)),
	)
	if wsTokens > 0 {
		lines = append(
			lines,
			fmt.Sprintf("    Workspace     ~%s", formatTokens(wsTokens)),
		)
	}
	if skillsTokens > 0 {
		lines = append(
			lines,
			fmt.Sprintf("    Skills        ~%s", formatTokens(skillsTokens)),
		)
	}
	lines = append(lines,
		fmt.Sprintf("  Tool schemas    ~%s", formatTokens(toolTokens)),
		fmt.Sprintf("  Messages        ~%s", formatTokens(msgTotal)),
		fmt.Sprintf("    user          ~%s", formatTokens(userTokens)),
		fmt.Sprintf("    assistant     ~%s", formatTokens(asstTokens)),
		fmt.Sprintf("    tool          ~%s", formatTokens(toolMsgTokens)),
		"",
		fmt.Sprintf("Last response:   %s tokens", formatTokens(latestOutput)),
		fmt.Sprintf("Session output:  %s tokens", formatTokens(sessionOutput)),
		"",
		"Press Esc to go back.",
	)

	children := make([]tuix.Element, 0, len(lines))
	for _, line := range lines {
		children = append(children, tuix.Text(line, tuix.NewStyle()))
	}

	return tuix.Box(
		tuix.Props{
			Direction: tuix.Column,
			Padding:   [4]int{1, 1, 1, 1},
		},
		tuix.NewStyle(),
		children...,
	)
}

func sessionMessages(runtime *agent.Runtime) []llm.Message {
	if runtime.Workspace != nil && runtime.Workspace.Session != nil &&
		len(runtime.Workspace.Session.Messages) > 0 {
		return runtime.Workspace.Session.Messages
	}
	return runtime.Agent.Conversation.Messages
}

// walkUsage returns the input tokens of the most recent assistant call (i.e.
// the current context size), the output tokens of that same call, and the
// sum of output tokens across the whole session.
func walkUsage(
	messages []llm.Message,
) (latestInput, latestOutput, sessionOutput int) {
	seenLatest := false
	for i := len(messages) - 1; i >= 0; i-- {
		u := messages[i].Usage
		if u == nil {
			continue
		}
		if !seenLatest {
			latestInput = u.InputTokens
			latestOutput = u.OutputTokens
			seenLatest = true
		}
		sessionOutput += u.OutputTokens
	}
	return
}

func breakdownSystemPrompt(prompt string) (workspaceTokens, skillsTokens int) {
	wsIdx := strings.Index(prompt, "## Workspace")
	skIdx := strings.Index(prompt, "## Available Skills")

	if wsIdx >= 0 {
		end := len(prompt)
		if skIdx > wsIdx {
			end = skIdx
		}
		workspaceTokens = estimateTokens(prompt[wsIdx:end])
	}
	if skIdx >= 0 {
		skillsTokens = estimateTokens(prompt[skIdx:])
	}
	return
}

func breakdownMessages(messages []llm.Message) (user, assistant, tool int) {
	for _, m := range messages {
		t := estimateTokens(m.Content)
		switch m.Role {
		case "user":
			user += t
		case "assistant":
			assistant += t
		case "tool":
			tool += t
		}
	}
	return
}

func lookupContextWindow(runtime *agent.Runtime, modelID string) int {
	if runtime.CurrentProvider == nil {
		return 0
	}
	for _, m := range runtime.CurrentProvider.Models() {
		if m.ID == modelID {
			return m.ContextWindow
		}
	}
	return 0
}

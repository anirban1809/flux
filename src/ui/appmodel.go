package ui

import (
	"fmt"
	"strings"
	"zipcode/src/agent"
	"zipcode/src/config"
	"zipcode/src/events"
	"zipcode/src/ui/components"
	"zipcode/src/utils"
	"zipcode/src/workspace"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
)

type AppModel struct {
	Workspace            *workspace.Workspace
	Runtime              *agent.Runtime
	Prompt               textinput.Model
	ViewPort             viewport.Model
	Result               string
	Tasks                []components.Task
	Conversation         string
	ActiveConversation   string
	ActiveToolMessages   []string
	ToolMessagesExpanded bool
	Question             components.Question
	FileChangeViewer     components.FileChangeViewer
	CommandsMenu         components.Menu
	ModelsMenu           components.Menu
	Commands             []string
	CommandDescriptions  []string
	Models               []string
	ModelDescriptions    []string
	StatusBar            components.StatusBar
	PromptExpanded       bool
	subagent             bool
}

func Iniaitalize(workspace *workspace.Workspace) AppModel {
	width, height, _ := utils.GetTerminalSize()

	input := textinput.New()
	input.Focus()
	input.CharLimit = 65536
	input.Width = width - 2
	vp := viewport.New(width-2, height-4)
	vp.SetContent(``)
	runtime := agent.NewRuntime(workspace)

	commands := []string{"/models", "/help", "/exit"}
	commandDescriptions := []string{"Select model", "Get help", "Exit ZipCode"}

	models := config.Cfg.ModelNames

	return AppModel{
		Workspace:    workspace,
		Runtime:      &runtime,
		Prompt:       input,
		ViewPort:     vp,
		CommandsMenu: components.CreateMenu(commands, commandDescriptions),

		Commands:            commands,
		CommandDescriptions: commandDescriptions,
		Models:              models,
		StatusBar:           components.CreateStatusBar(workspace.RootPath, config.Cfg.CurrentModel),
		ActiveConversation:  "\n",
	}
}

func (a AppModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, a.StatusBar.Init())
}

func waitForRuntimeEvent() tea.Cmd {
	return func() tea.Msg {
		return events.EventManager.ReadFromChannel(events.AGENT_OUTPUT_CHANNEL)
	}
}

func waitForFileChangeEvent() tea.Cmd {
	return func() tea.Msg {
		return events.EventManager.ReadFromChannel(events.FILE_DIFF_CHANNEL)
	}
}

func (a *AppModel) ProcessQuestion() {
	if a.Question.Question == "" || !a.Question.Selected {
		return
	}

	a.Question.Visible = false
	a.FileChangeViewer.Visible = false
	events.EventManager.WriteToChannel(events.AGENT_INPUT_CHANNEL, a.Question.GetSelectedItem())
	a.Question.Selected = false
}

func (a AppModel) GetConversation() string {
	toolSection := ""
	if len(a.ActiveToolMessages) > 0 {
		dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

		var visibleMessages []string
		if a.ToolMessagesExpanded || len(a.ActiveToolMessages) <= 4 {
			visibleMessages = a.ActiveToolMessages
		} else {
			hidden := len(a.ActiveToolMessages) - 4
			hint := hintStyle.Render(fmt.Sprintf(" %s\n └── ... %d more tool calls ctrl+r: expand ▼", "subagent", hidden))
			visibleMessages = append([]string{hint}, a.ActiveToolMessages[len(a.ActiveToolMessages)-4:]...)
		}

		var rendered []string
		if a.ToolMessagesExpanded && len(a.ActiveToolMessages) > 4 {
			hint := hintStyle.Render(" └── ctrl+r: collapse ▲")
			rendered = append(rendered, hint)
		}
		for _, m := range visibleMessages {
			rendered = append(rendered, dimStyle.Render(m))
		}
		toolSection = strings.Join(rendered, "\n") + "\n"
	}

	content := a.Conversation + "\n" + a.ActiveConversation + toolSection
	return wordwrap.String(content, a.ViewPort.Width)
}

func (a AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return a, tea.Quit
		case "w":
			if a.FileChangeViewer.Visible {
				a.FileChangeViewer.ScrollUp()
				return a, tea.Batch(cmds...)
			}
		case "s":
			if a.FileChangeViewer.Visible {
				a.FileChangeViewer.ScrollDown()
				return a, tea.Batch(cmds...)
			}
		case "enter":
			if a.ModelsMenu.IsVisible() {
				selectedModel := a.Models[a.ModelsMenu.GetSelectedIndex()]
				config.Cfg.SetCurrentModel(selectedModel)
				a.StatusBar.SetModel(selectedModel)
			}

			if strings.HasPrefix(a.Prompt.Value(), "/") {
				a.CommandsMenu, _ = a.CommandsMenu.Update(msg)
				a.CommandsMenu.SetVisible(false)

				switch a.Commands[a.CommandsMenu.GetSelectedIndex()] {
				case "/models":
					a.ModelsMenu.SetVisible(true)
				case "/help":
					break
				case "/exit":
					return a, tea.Quit
				}

				a.Prompt.SetValue("")
				return a, tea.Batch(tea.Batch(cmds...),
					waitForRuntimeEvent(), waitForFileChangeEvent())
			}

			if a.Prompt.Value() != "" {
				go a.Runtime.Run(a.Prompt.Value())
				a.ActiveConversation += "⏺ " + a.Prompt.Value() + "\n"
				a.StatusBar.SetStatus(components.Status_RUNNING)
				a.ViewPort.SetContent(a.GetConversation())
				a.ViewPort.GotoBottom()
				a.Prompt.SetValue("")
				return a, tea.Batch(tea.Batch(cmds...),
					waitForRuntimeEvent(), waitForFileChangeEvent())
			}

		case "ctrl+e":
			a.PromptExpanded = !a.PromptExpanded
			return a, tea.Batch(cmds...)

		case "ctrl+r":
			a.ToolMessagesExpanded = !a.ToolMessagesExpanded
			a.ViewPort.SetContent(a.GetConversation())
			return a, tea.Batch(cmds...)

		case "shift+tab":
			if a.StatusBar.GetMode() == components.Mode_PLAN {
				a.StatusBar.SetMode(components.Mode_EDIT)
				return a, tea.Batch(tea.Batch(cmds...),
					waitForRuntimeEvent(), waitForFileChangeEvent())
			}

			a.StatusBar.SetMode(components.Mode_PLAN)
			return a, tea.Batch(tea.Batch(cmds...),
				waitForRuntimeEvent(), waitForFileChangeEvent())

		}

	case events.ResponseEvent:
		a.StatusBar.UpdateUsage(a.Runtime.InputTokens, a.Runtime.OutputTokens)

		if msg.EventType == events.Tool {
			var message string

			if msg.SubAgent && !a.subagent {
				message = msg.SubAgentName
				a.ActiveToolMessages = append(a.ActiveToolMessages, message)
				a.subagent = true
			}

			message = fmt.Sprintf(" └── %s", msg.Message)
			a.ActiveToolMessages = append(a.ActiveToolMessages, message)
			a.ViewPort.SetContent(a.GetConversation())

			a.ViewPort.GotoBottom()

			if msg.Question != "" {
				a.SetQuestion(components.CreateQuestion(msg.Question, msg.Options))
				a.Question.Visible = true
			}

		} else {
			toolHistory := strings.Join(a.ActiveToolMessages, "\n")
			if toolHistory != "" {
				toolHistory = "\n" + toolHistory
			}
			a.ActiveConversation += toolHistory + "\n" + lipgloss.NewStyle().Render(msg.Message) + "\n"
			a.Conversation += "\n" + strings.Replace(a.ActiveConversation, "⏺ ", "✔ ", 1)
			a.ActiveConversation = "\n"
			a.ActiveToolMessages = nil
			a.ToolMessagesExpanded = false
			a.StatusBar.SetStatus(components.Status_IDLE)
			a.ViewPort.SetContent(a.GetConversation())
			a.ViewPort.GotoBottom()
		}
		return a, tea.Batch(waitForRuntimeEvent(), waitForFileChangeEvent())

	case events.FileChangeEvent:
		changeType := "patch"
		switch msg.ChangeType {
		case events.FileChange_Create:
			changeType = "create"
		case events.FileChange_Append:
			changeType = "append"
		}
		a.FileChangeViewer = components.CreateFileChangeViewer(msg.FileName, changeType, msg.Content, msg.Patches)
		return a, tea.Batch(waitForRuntimeEvent(), waitForFileChangeEvent())

	case tea.WindowSizeMsg:
		ClearScreen()
		width, height, _ := utils.GetTerminalSize()
		a.ViewPort.GotoBottom()
		a.ViewPort.Width = width - 2
		a.ViewPort.Height = height - 4
	}

	a.ProcessQuestion()
	a.CommandsMenu.SetVisible(strings.HasPrefix(a.Prompt.Value(), "/"))

	var cmd tea.Cmd

	a.Prompt, cmd = a.Prompt.Update(msg)
	cmds = append(cmds, cmd)

	a.Question, cmd = a.Question.Update(msg)
	cmds = append(cmds, cmd)

	a.ViewPort, cmd = a.ViewPort.Update(msg)
	cmds = append(cmds, cmd)

	a.CommandsMenu, cmd = a.CommandsMenu.Update(msg)
	cmds = append(cmds, cmd)

	a.ModelsMenu, cmd = a.ModelsMenu.Update(msg)
	cmds = append(cmds, cmd)

	a.StatusBar, cmd = a.StatusBar.Update(msg)
	cmds = append(cmds, cmd)

	return a, tea.Batch(cmds...)
}

func (a *AppModel) SetQuestion(question components.Question) {
	a.Question = question
}

func (a AppModel) renderPromptArea(width int) string {
	promptAreaWidth := width - 2

	bgStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("235")).
		Width(promptAreaWidth)

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Background(lipgloss.Color("235"))

	value := a.Prompt.Value()

	// Wrap the raw text to count visual lines (subtract padding of 2 chars on each side)
	wrappedValue := wordwrap.String(value, promptAreaWidth-4)
	lines := strings.Split(wrappedValue, "\n")
	// Remove trailing empty line from wordwrap
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	if value == "" || len(lines) <= 3 {
		// Short prompt: input with background, no preview needed
		return bgStyle.Padding(0, 1).Render(a.Prompt.View())
	}

	// Long prompt: show preview lines above the input widget
	var visibleLines []string
	var hint string
	if a.PromptExpanded {
		visibleLines = lines
		hint = dimStyle.Render(fmt.Sprintf("  %d lines  ctrl+e: collapse ▲", len(lines)))
	} else {
		visibleLines = lines[len(lines)-3:]
		hint = dimStyle.Render(fmt.Sprintf("  %d lines  ctrl+e: expand ▼", len(lines)))
	}

	preview := bgStyle.Padding(1, 1, 0, 1).Render(strings.Join(visibleLines, "\n"))
	inputLine := bgStyle.Padding(0, 1, 1, 1).Render(a.Prompt.View())

	return preview + "\n" + hint + "\n" + inputLine
}

func (a AppModel) View() string {
	width, _, _ := utils.GetTerminalSize()
	promptView := a.renderPromptArea(width)
	viewPortView := lipgloss.NewStyle().Padding(1).Render(a.ViewPort.View())
	return fmt.Sprintf(
		"\n%s\n%s%s\n%s%s%s\n%s",
		viewPortView,
		a.FileChangeViewer.View(),
		a.Question.View(),
		promptView,
		a.CommandsMenu.View(),
		a.ModelsMenu.View(),
		a.StatusBar.View(),
	)
}

func ClearScreen() {
	fmt.Print("\033[H\033[2J\033[3J") /* clear the screen */
}

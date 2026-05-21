package view

import (
	"os"
	"strings"

	"zipcode/src/events"
	"zipcode/src/utils"
	view "zipcode/src/view/components"
	"zipcode/src/view/viewctx"

	"github.com/anirban1809/tuix/tuix"
)

type CommandKind int

const (
	CmdUnknown CommandKind = iota
	CmdView
	CmdPrompt
	CmdAction
)

type Command struct {
	Name   string
	Kind   CommandKind
	Prompt string
	Run    func()
}

func MainMenu(props tuix.Props) tuix.Element {
	activeView := props.Get("activeView").(string)
	setActiveView := props.Get("setActiveView").(func(string))
	submitPrompt := props.Get("submitPrompt").(func(string))
	prompt := props.Get("prompt").(string)
	context := tuix.UseContext(viewctx.MainContext)
	setFocusPrompt := props.Get("setFocusPrompt").(func(bool))
	clearPrompt, _ := props.Get("clearPrompt").(func())
	clearOutputs, _ := props.Get("clearOutputs").(func())
	setPrompt, _ := props.Get("setPrompt").(func(string))

	dismissMenu := func() {
		if clearPrompt != nil {
			clearPrompt()
		}
		setActiveView("")
		setFocusPrompt(true)
	}

	commands := []Command{
		{Name: "/models", Kind: CmdView},
		{Name: "/skills", Kind: CmdView},
		{Name: "/agents", Kind: CmdView},
		{Name: "/sessions", Kind: CmdView},
		{Name: "/context", Kind: CmdView},
		{Name: "/settings", Kind: CmdView},
		{
			Name:   "/about",
			Kind:   CmdPrompt,
			Prompt: "Tell me about this project.",
		},
		{Name: "/exit", Kind: CmdAction, Run: func() { tuix.Exit() }},
		{Name: "/clear", Kind: CmdAction, Run: func() {
			if context.Runtime != nil {
				context.Runtime.Clear()
			}
			if clearOutputs != nil {
				clearOutputs()
			}
			dismissMenu()
			events.EventManager.WriteToChannel(
				events.NOTIFICATION_CHANNEL,
				events.Notification{
					Type:    events.INFO,
					Message: "Conversation cleared.",
				},
			)
		}},
		{Name: "/compact", Kind: CmdAction, Run: func() {
			if context.Runtime == nil {
				return
			}
			runtime := context.Runtime
			dismissMenu()
			go func() {
				events.EventManager.WriteToChannel(
					events.NOTIFICATION_CHANNEL,
					events.Notification{
						Type:    events.INFO,
						Message: "Compacting conversation...",
					},
				)
				if _, err := runtime.Compact(); err != nil {
					events.EventManager.WriteToChannel(
						events.NOTIFICATION_CHANNEL,
						events.Notification{
							Type:    events.ERROR,
							Message: "Compact failed: " + err.Error(),
						},
					)
					return
				}
				events.EventManager.WriteToChannel(
					events.NOTIFICATION_CHANNEL,
					events.Notification{
						Type:    events.INFO,
						Message: "Conversation compacted.",
					},
				)
			}()
		}},
		{Name: "/providers", Kind: CmdView},
	}

	if context.Runtime != nil && context.Runtime.SkillRegistry != nil {
		for _, s := range context.Runtime.SkillRegistry.ListEnabled() {
			name := "/" + s.Name
			commands = append(commands, Command{
				Name:   name,
				Kind:   CmdPrompt,
				Prompt: name,
			})
		}
	}

	findCommand := func(selected string) Command {
		var toFind Command
		for _, command := range commands {
			if command.Name == selected {
				toFind = command
				break
			}
		}
		return toFind
	}

	filteredItems := utils.Filter(commands, func(item Command, index int) bool {
		return strings.HasPrefix(item.Name, prompt)
	})

	if activeView != "" && tuix.CurrentKey.Code == tuix.KeyEscape {
		setFocusPrompt(true)
		setActiveView("")
	}

	modelSelection := view.ModelSelection(tuix.Props{Values: map[string]any{
		"setActiveView": setActiveView,
		"visible":       activeView == "/models",
	}})

	skillsView := view.Skills(tuix.Props{Values: map[string]any{
		"setActiveView": setActiveView,
		"visible":       activeView == "/skills",
		"runtime":       context.Runtime,
	}})

	agentsView := view.Agent(tuix.Props{})
	sessionsView := view.Sessions(tuix.Props{Values: map[string]any{
		"setActiveView": setActiveView,
		"visible":       activeView == "/sessions",
		"runtime":       context.Runtime,
	}})

	providersView := view.Providers(
		tuix.Props{
			Values: map[string]any{
				"visible":       activeView == "/providers",
				"setActiveView": setActiveView,
			},
		},
	)

	contextView := view.Context(tuix.Props{Values: map[string]any{
		"runtime": context.Runtime,
	}})

	if activeView == "/models" {
		return modelSelection
	}

	if activeView == "/exit" {
		os.Exit(0)
	}

	if activeView == "/skills" {
		return skillsView
	}

	if activeView == "/agents" {
		return agentsView
	}

	if activeView == "/sessions" {
		return sessionsView
	}

	if activeView == "/providers" {
		return providersView
	}

	if activeView == "/context" {
		return contextView
	}

	commandNames := utils.Map(
		filteredItems,
		func(item Command, index int) string {
			return item.Name
		},
	)

	menuValues := map[string]any{
		"items":   commandNames,
		"visible": activeView == "",
	}
	if setPrompt != nil {
		menuValues["onAutoComplete"] = func(item string) {
			setPrompt(item)
		}
	}

	return view.Menu(tuix.Props{
		Values: menuValues,
	}, func(selected string, _ int) {
		if selected == "" {
			setFocusPrompt(true)
			return
		}
		cmd := findCommand(selected) // lookup in `commands`
		if cmd.Kind == CmdUnknown {
			setFocusPrompt(true)
			return
		}
		setFocusPrompt(false)
		switch cmd.Kind {
		case CmdView:
			setActiveView(cmd.Name)
		case CmdPrompt:
			submitPrompt(cmd.Prompt)
			setActiveView("") // dismiss menu
		case CmdAction:
			cmd.Run()
		}
	}, nil)
}

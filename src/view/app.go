package view

import (
	"fmt"
	"strings"

	"zipcode/src/agent"
	view "zipcode/src/view/components"
	"zipcode/src/view/viewctx"
	"zipcode/src/workspace"

	"github.com/anirban1809/tuix/tuix"
	"github.com/anirban1809/tuix/tuix/components"
)

var promptChan = make(chan string)
var clearOutputsChan = make(chan struct{}, 1)
var queuedPrompts = make(chan string, 32)

func App(props tuix.Props) tuix.Element {
	prompt, setPrompt := tuix.UseState("")
	activeSession, setActiveSession := tuix.UseState(false)
	outputs, setOutputs := tuix.UseState([]tuix.Element{})
	livePrompt, setLivePrompt := tuix.UseState("")
	livePromptIdx, setLivePromptIdx := tuix.UseState(-1)
	activeMenuView, setActiveMenuView := tuix.UseState("")
	notification, setNotification := tuix.UseState(agent.Notification{})
	activeSkillName, setActiveSkillName := tuix.UseState("")

	questionVisible, setQuestionVisible := tuix.UseState(false)
	question, setQuestion := tuix.UseState(struct {
		question string
		options  []string
	}{})
	selectedOption, setSelectedOption := tuix.UseState("")
	optionSelected, setOptionSelected := tuix.UseState(false)
	fileDiff, setFileDiff := tuix.UseState(agent.FileChangeEvent{})
	focusPrompt, setFocusPrompt := tuix.UseState(true)
	planStatus, setPlanStatus := tuix.UseState(agent.PlanStatusEvent{})
	queuedCount, setQueuedCount := tuix.UseState(0)

	runtime := props.Get("runtime").(*agent.Runtime)

	trimmed := strings.TrimSpace(prompt)
	menuVisible := strings.HasPrefix(trimmed, "/") &&
		!strings.ContainsAny(trimmed, " \t")

	submitPrompt := func(p string) {
		send := p
		if name, _, ok := runtime.ParseSkillCommand(p); ok {
			send = runtime.ExpandSkillCommand(p)
			runtime.Executor.SetActiveSkill(name)
			setActiveSkillName(name)
		}
		promptChan <- p
		setPrompt("")
		if !activeSession {
			setActiveSession(true)
		}

		go func() {
			if _, err := runtime.Run(send); err != nil {
				agent.EventManager.WriteToChannel(
					agent.NOTIFICATION_CHANNEL,
					agent.Notification{
						Type:    agent.ERROR,
						Message: err.Error(),
					},
				)
			}
		}()
	}

	queuePrompt := func(p string) {
		select {
		case queuedPrompts <- p:
			setQueuedCount(queuedCount + 1)
			setPrompt("")
		default:
			agent.EventManager.WriteToChannel(
				agent.NOTIFICATION_CHANNEL,
				agent.Notification{
					Type:    agent.ERROR,
					Message: "Prompt queue is full; wait for the active plan to finish.",
				},
			)
		}
	}

	canSubmit := tuix.CurrentKey.Code == tuix.KeyEnter && !menuVisible
	if canSubmit {
		if !activeSession {
			submitPrompt(prompt)
		} else if planStatus.Active && strings.TrimSpace(prompt) != "" {
			queuePrompt(prompt)
		}
	}

	tuix.UseEffect(func() func() {
		go func() {
			var activeSubAgent string
			var activeSkill string
			agentOut := make(chan agent.ResponseEvent)

			go func() {
				for {
					setFileDiff(
						agent.EventManager.ReadFromChannel(
							agent.FILE_DIFF_CHANNEL,
						).(agent.FileChangeEvent),
					)
				}
			}()

			go func() {
				for {
					ev := agent.EventManager.ReadFromChannel(agent.AGENT_OUTPUT_CHANNEL).(agent.ResponseEvent)
					agentOut <- ev
				}
			}()

			go func() {
				for {
					notif := agent.EventManager.ReadFromChannel(agent.NOTIFICATION_CHANNEL).(agent.Notification)

					if notif.Message == "ERR_MISSING_PROVIDER" {
						setNotification(
							agent.Notification{
								Type:    agent.ERROR,
								Message: "No providers configured, please configure a provider via /providers command in the main menu",
							},
						)
						continue
					}

					if notif.Type == agent.ERROR {
						agentOut <- agent.ResponseEvent{
							EventType: agent.Error,
						}
					}
					setNotification(notif)
				}
			}()

			go func() {
				for {
					ev := agent.EventManager.ReadFromChannel(agent.PLAN_STATUS_CHANNEL).(agent.PlanStatusEvent)
					setPlanStatus(ev)
				}
			}()

			var acc []tuix.Element
			var liveLocal string
			promptIdx := -1
			streamIdx := -1
			var streamText string
			for {
				select {

				case <-clearOutputsChan:
					acc = acc[:0]
					liveLocal = ""
					promptIdx = -1
					streamIdx = -1
					streamText = ""
					setLivePrompt("")
					setLivePromptIdx(-1)
					setOutputs(nil)
					continue

				case p := <-promptChan:
					liveLocal = p
					setLivePrompt(p)
					promptIdx = len(acc)
					streamIdx = -1
					streamText = ""
					setLivePromptIdx(promptIdx)
				case ev := <-agentOut:
					msg := ev.Message
					style := tuix.NewStyle().Foreground(tuix.Hex("#c8c8c8"))

					if ev.EventType == agent.MessageDelta {
						streamText += msg
						el := tuix.Box(
							tuix.Props{Padding: [4]int{0, 2, 0, 2}},
							tuix.NewStyle(),
							tuix.Markdown(streamText, style),
						)
						if streamIdx < 0 || streamIdx >= len(acc) {
							streamIdx = len(acc)
							acc = append(acc, el)
						} else {
							acc[streamIdx] = el
						}
						snap := make([]tuix.Element, len(acc))
						copy(snap, acc)
						setOutputs(snap)
						continue
					}

					if ev.EventType == agent.Tool {
						streamIdx = -1
						streamText = ""

						if ev.Question != "" {
							setQuestionVisible(true)
							setQuestion(struct {
								question string
								options  []string
							}{question: ev.Question, options: ev.Options})
						}

						if ev.SubAgent {
							if activeSubAgent != ev.SubAgentName {
								activeSubAgent = ev.SubAgentName
								msg = fmt.Sprintf(
									"    \nsubagent:%s\n  └──%s",
									ev.SubAgentName,
									msg,
								)
								style = style.Foreground(tuix.Hex("#64c3ff")).
									Bold(true)
							} else {
								msg = fmt.Sprintf("  └──%s", msg)
								style = style.Foreground(tuix.Hex("#848484"))
							}
						} else if ev.SkillName != "" {
							if activeSkill != ev.SkillName {
								activeSkill = ev.SkillName
								setActiveSkillName(ev.SkillName)
								msg = fmt.Sprintf("    \n[/%s]\n  └──%s", ev.SkillName, msg)
								style = style.Foreground(tuix.Hex("#b39ddb")).Bold(true)
							} else {
								msg = fmt.Sprintf("  └──%s", msg)
								style = style.Foreground(tuix.Hex("#848484"))
							}
						} else {
							if activeSkill != "" {
								setActiveSkillName("")
							}
							activeSkill = ""
							msg = fmt.Sprintf("\n⏺ %s\n", msg)
							style = style.Foreground(tuix.Hex("#848484"))
						}
					} else {
						if ev.EventType == agent.Message ||
							ev.EventType == agent.MessageComplete ||
							ev.EventType == agent.Error {
							streamIdx = -1
							streamText = ""
						}
						if liveLocal != "" && promptIdx >= 0 {
							var promptEl tuix.Element
							if ev.EventType == agent.Error {
								promptEl = view.Prompt(tuix.Props{Values: map[string]any{
									"prompt":  liveLocal,
									"running": false,
									"failed":  true,
								}})
							} else {
								promptEl = view.Prompt(tuix.Props{Values: map[string]any{
									"prompt":  liveLocal,
									"running": false,
									"failed":  false,
								}})
							}
							acc = append(acc[:promptIdx], append([]tuix.Element{promptEl}, acc[promptIdx:]...)...)
							// appending an empty line to reduce cluttering
							acc = append(acc, tuix.Text("", tuix.NewStyle()))
							liveLocal = ""
							promptIdx = -1
							setLivePrompt("")
							setLivePromptIdx(-1)
						}
						setActiveSession(false)

						select {
						case queued := <-queuedPrompts:
							setQueuedCount(queuedCount - 1)
							go submitPrompt(queued)
						default:
						}
					}

					if ev.EventType != agent.Error &&
						ev.EventType != agent.MessageComplete {
						acc = append(
							acc,
							tuix.Box(
								tuix.Props{Padding: [4]int{0, 2, 0, 2}},
								tuix.NewStyle(),
								tuix.WrappedText(
									msg,
									style,
								),
							),
						)
					}
				}

				snap := make([]tuix.Element, len(acc))
				copy(snap, acc)
				setOutputs(snap)
			}
		}()
		return func() {}
	}, []any{})

	return viewctx.MainContext.Provide(
		&viewctx.ContextType{
			Runtime:        runtime,
			SetFocusPrompt: setFocusPrompt,
		}, func() tuix.Element {

			children := []tuix.Element{view.Banner(tuix.Props{})}

			if activeSession && livePrompt != "" && livePromptIdx >= 0 &&
				livePromptIdx <= len(outputs) {
				children = append(children, outputs[:livePromptIdx]...)
				children = append(
					children,
					view.Prompt(tuix.Props{Values: map[string]any{
						"prompt":  livePrompt,
						"running": true,
					}}),
				)
				children = append(children, outputs[livePromptIdx:]...)
			} else {
				children = append(children, outputs...)
			}

			if questionVisible {
				children = append(
					children, tuix.Box(
						tuix.Props{Direction: tuix.Column},
						tuix.NewStyle(),
						tuix.Text("", tuix.NewStyle()),
						view.FileDiff(
							tuix.Props{
								Values: map[string]any{"fileDiff": fileDiff},
							},
						),
						tuix.Text("", tuix.NewStyle()),
						tuix.Text(question.question, tuix.NewStyle()),
						view.Menu(
							tuix.Props{Values: map[string]any{
								"items":            question.options,
								"setSelectedIndex": setSelectedOption,
								"visible":          questionVisible,
							}},
							func(selected string, _ int) {
								setOptionSelected(true)
								setSelectedOption(selected)
							}, nil,
						),
					))
			}

			if optionSelected {
				go agent.EventManager.WriteToChannel(
					agent.AGENT_INPUT_CHANNEL,
					selectedOption,
				)
				setOptionSelected(false)
				setQuestionVisible(false)
			}

			notificationStyle := tuix.NewStyle().Foreground(tuix.Hex("#a3edff"))

			if notification.Type == agent.ERROR {
				notificationStyle = tuix.NewStyle().
					Foreground(tuix.Hex("#ff8282"))
			}

			notificationEl := tuix.Box(
				tuix.Props{Padding: [4]int{1, 0, 0, 0}},
				tuix.NewStyle().Foreground(tuix.Hex("#9ad8ff")),
				tuix.Text(notification.Message, notificationStyle),
			)
			if notification.Message != "" {
				children = append(children, notificationEl)
			}

			if len(planStatus.Steps) > 0 {
				children = append(children, view.PlanView(tuix.Props{
					Values: map[string]any{"plan": planStatus},
				}))
				if queuedCount > 0 {
					children = append(children, tuix.Text(
						fmt.Sprintf(
							"Queued prompts: %d (will run after the plan finishes)",
							queuedCount,
						),
						tuix.NewStyle().Foreground(tuix.Hex("#9ad8ff")),
					))
				}
			}

			children = append(children, tuix.Box(
				tuix.Props{Direction: tuix.Row, Padding: [4]int{0, 1, 0, 1}},
				tuix.NewStyle().Border(tuix.Border{
					Top: true, Bottom: true,
					Color: tuix.Hex("#646464"),
				}),
				components.Input(
					">",
					"_",
					focusPrompt,
					prompt,
					func(value string) {
						setNotification(
							agent.Notification{Type: agent.INFO, Message: ""},
						)
						setPrompt(value)
					},
				),
			),
			)

			if !menuVisible && activeMenuView != "" {
				setActiveMenuView("")
			}

			if menuVisible {
				children = append(children, MainMenu(
					tuix.Props{Values: map[string]any{
						"activeView":     activeMenuView,
						"setActiveView":  setActiveMenuView,
						"prompt":         prompt,
						"submitPrompt":   submitPrompt,
						"setFocusPrompt": setFocusPrompt,
						"clearPrompt": func() {
							setPrompt("")
						},
						"setPrompt": func(value string) {
							setPrompt(value)
						},
						"clearOutputs": func() {
							select {
							case clearOutputsChan <- struct{}{}:
							default:
							}
						},
					}},
				),
				)
			}

			children = append(children, view.StatusLine(tuix.Props{
				Values: map[string]any{
					"workspacePath": workspace.AbsToTildePath(
						props.Get("wd").(string),
					),
					"running":               activeSession,
					"inputTokens":           runtime.InputTokens,
					"cachedInputTokens":     runtime.CachedInputTokens,
					"outputTokens":          runtime.OutputTokens,
					"branch":                runtime.Workspace.GetCurrentBranch(),
					"hasUncommittedChanges": runtime.Workspace.HasUncommittedChanges(),
					"activeSkill":           activeSkillName,
				},
			}))

			return tuix.Box(
				tuix.Props{
					Direction: tuix.Column,
					Padding:   [4]int{0, 2, 0, 2},
					Width:     tuix.Grow(1),
				},
				tuix.NewStyle(),
				children...,
			)
		},
	)
}

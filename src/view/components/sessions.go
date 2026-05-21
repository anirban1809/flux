package view

import (
	"fmt"

	"flux/src/agent"
	"flux/src/workspace"

	"github.com/anirban1809/tuix/tuix"
)

func Sessions(props tuix.Props) tuix.Element {
	setActiveView := props.Get("setActiveView").(func(string))
	visible := props.Get("visible").(bool)
	runtime := props.Get("runtime").(*agent.Runtime)

	sessions, _ := workspace.ListSessions(runtime.Workspace.RootPath)

	if len(sessions) == 0 {
		return tuix.Box(
			tuix.Props{
				Direction: tuix.Column,
				Padding:   [4]int{1, 1, 1, 1},
			},
			tuix.NewStyle(),
			tuix.Text("No sessions found.", tuix.NewStyle()),
			tuix.Text("Press Esc to go back.", tuix.NewStyle()),
		)
	}

	labels := make([]string, len(sessions))
	for i, s := range sessions {
		marker := "  "
		if runtime.Session == s.ID {
			marker = "(Current Session) "
		}

		labels[i] = fmt.Sprintf(
			"%s%s  (%s)",
			marker,
			s.ID,
			s.StartedAt.Local().Format("2006-01-02 15:04:05"),
		)
	}

	return tuix.Box(
		tuix.Props{
			Direction: tuix.Column,
			Padding:   [4]int{1, 1, 1, 1},
		},
		tuix.NewStyle(),
		tuix.Text("Re-open a session:", tuix.NewStyle()),
		tuix.Text("", tuix.NewStyle()),
		Menu(tuix.Props{Values: map[string]any{
			"items":    labels,
			"visible":  visible,
			"viewSize": 8,
		}}, func(selected string, i int) {
			runtime.SetSession(sessions[i])
			setActiveView("")
		}, nil),
		tuix.Text("Press Enter to open, Esc to cancel", tuix.NewStyle()),
	)
}

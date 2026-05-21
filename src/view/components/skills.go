package view

import (
	"fmt"

	"flux/src/agent"
	"flux/src/skills"
	view "flux/src/ui/components/utils"
	"flux/src/view/viewctx"

	"github.com/anirban1809/tuix/tuix"
)

func Skills(props tuix.Props) tuix.Element {
	runtime, _ := props.Get("runtime").(*agent.Runtime)
	visible, _ := props.Get("visible").(bool)
	setActiveView, _ := props.Get("setActiveView").(func(string))
	context := tuix.UseContext(viewctx.MainContext)

	if runtime == nil || runtime.SkillRegistry == nil {
		return tuix.Box(
			tuix.Props{
				Direction: tuix.Column,
				Padding:   [4]int{1, 1, 1, 1},
			},
			tuix.NewStyle(),
			tuix.Text("Skills are unavailable.", tuix.NewStyle()),
			tuix.Text("Press Esc to go back.", tuix.NewStyle()),
		)
	}

	allSkills := runtime.SkillRegistry.List()

	if len(allSkills) == 0 {
		return tuix.Box(
			tuix.Props{
				Direction: tuix.Column,
				Padding:   [4]int{1, 1, 1, 1},
			},
			tuix.NewStyle(),
			tuix.Text("All Skills", tuix.NewStyle()),
			view.NewLine(),
			tuix.Text("No skills found.", tuix.NewStyle()),
			view.NewLine(),
			tuix.Text(
				"Create skills in .flux/skills/ or ~/.flux/skills/ as Markdown files.",
				tuix.NewStyle(),
			),
			tuix.Text("Press Esc to go back.", tuix.NewStyle()),
		)
	}

	labels := make([]string, len(allSkills))
	for i, s := range allSkills {
		state := "on "
		if !s.Enabled {
			state = "off"
		}
		labels[i] = fmt.Sprintf(
			"[%s] %-20s %-10s %s",
			state,
			s.Name,
			sourceLabel(s.Source),
			s.Description,
		)
	}

	return tuix.Box(
		tuix.Props{
			Direction: tuix.Column,
			Padding:   [4]int{1, 1, 1, 1},
		},
		tuix.NewStyle(),
		tuix.Text("Skills", tuix.NewStyle()),
		view.NewLine(),
		Menu(tuix.Props{Values: map[string]any{
			"items":    labels,
			"visible":  visible,
			"viewSize": 8,
		}}, func(selected string, _ int) {
			for i, label := range labels {
				if label != selected {
					continue
				}
				s := allSkills[i]
				if s.Enabled {
					_ = runtime.SkillRegistry.Disable(s.Name)
				} else {
					_ = runtime.SkillRegistry.Enable(s.Name)
				}
				if setActiveView != nil {
					context.SetFocusPrompt(true)
					setActiveView("")
				}
				return
			}
		}, nil),
		tuix.Text(
			"Enter to toggle. Edit skill files in your editor. Esc to go back.",
			tuix.NewStyle(),
		),
	)
}

func sourceLabel(s skills.SkillSource) string {
	switch s {
	case skills.SourceInternal:
		return "internal"
	case skills.SourceGlobal:
		return "global"
	case skills.SourceProject:
		return "project"
	}
	return string(s)
}

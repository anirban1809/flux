package view

import (
	view "flux/src/ui/components/utils"

	"github.com/anirban1809/tuix/tuix"
)

func Agent(props tuix.Props) tuix.Element {
	return tuix.Box(
		tuix.Props{Direction: tuix.Column, Padding: [4]int{1, 1, 1, 1}},
		tuix.NewStyle(),
		tuix.Text("All Agents", tuix.NewStyle()),
		tuix.Text("No Agents found", tuix.NewStyle()),
		view.NewLine(),
		tuix.Text("Create New Agent", tuix.NewStyle()),
	)
}

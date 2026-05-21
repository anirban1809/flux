package view

import "github.com/anirban1809/tuix/tuix"

func YoloConfirm(props tuix.Props) tuix.Element {
	onChange := props.Get("onChange").(func(string, int))

	titleStyle := tuix.NewStyle().Bold(true).Foreground(tuix.Hex("#ff8c00"))
	dimStyle := tuix.NewStyle().Foreground(tuix.Hex("#a2a2a2"))
	bodyStyle := tuix.NewStyle().Foreground(tuix.Hex("#e0e0e0"))

	return tuix.Box(
		tuix.Props{Direction: tuix.Column, Padding: [4]int{3, 4, 2, 4}},
		tuix.NewStyle(),
		tuix.Text("⚠  YOLO Mode", titleStyle),
		tuix.Text("", tuix.NewStyle()),
		tuix.Text("With YOLO mode enabled, all file changes will be applied", bodyStyle),
		tuix.Text("automatically without asking for confirmation.", bodyStyle),
		tuix.Text("", tuix.NewStyle()),
		tuix.Text("You can toggle YOLO mode at any time with ctrl+y.", dimStyle),
		tuix.Text("", tuix.NewStyle()),
		tuix.Text("Enable YOLO mode?", bodyStyle),
		Menu(
			tuix.Props{Values: map[string]any{
				"items":   []string{"No, start normally", "Yes, enable YOLO mode"},
				"visible": true,
			}},
			onChange,
			nil,
		),
	)
}

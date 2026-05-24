package view

import "github.com/anirban1809/tuix/tuix"

func TrustConfirm(props tuix.Props) tuix.Element {
	onChange := props.Get("onChange").(func(string, int))
	dir := props.Get("dir").(string)

	titleStyle := tuix.NewStyle().Bold(true).Foreground(tuix.Hex("#ffd700"))
	bodyStyle := tuix.NewStyle().Foreground(tuix.Hex("#e0e0e0"))
	dimStyle := tuix.NewStyle().Foreground(tuix.Hex("#a2a2a2"))

	return tuix.Box(
		tuix.Props{Direction: tuix.Column, Padding: [4]int{3, 4, 2, 4}},
		tuix.NewStyle(),
		tuix.Text("  Trust this directory?", titleStyle),
		tuix.Text("", tuix.NewStyle()),
		tuix.Text(dir, dimStyle),
		tuix.Text("", tuix.NewStyle()),
		tuix.Text("Flux has not been granted access to operate in this directory.", bodyStyle),
		tuix.Text("Do you trust the authors of the files in this directory?", bodyStyle),
		tuix.Text("", tuix.NewStyle()),
		Menu(
			tuix.Props{Values: map[string]any{
				"items":   []string{"No, exit", "Yes, trust directory"},
				"visible": true,
			}},
			onChange,
			nil,
		),
	)
}

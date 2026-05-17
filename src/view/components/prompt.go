package view

import "github.com/anirban1809/tuix/tuix"

func Prompt(props tuix.Props) tuix.Element {
	prompt, _ := props.Get("prompt").(string)
	running, _ := props.Get("running").(bool)
	failed, _ := props.Get("failed").(bool)

	marker := "  "
	markerStyle := tuix.NewStyle()

	if running && tuix.CurrentTick {
		marker = "⏺ "
		markerStyle = tuix.NewStyle().Foreground(tuix.Hex("#6d798a"))
	}

	if !running {
		marker = "✔ "
	}

	if failed {
		marker = "✕ "
	}

	return tuix.Box(
		tuix.Props{Direction: tuix.Column},
		tuix.NewStyle(),

		tuix.Text("", tuix.NewStyle()),
		tuix.Box(
			tuix.Props{Direction: tuix.Row, Padding: [4]int{1, 1, 1, 1}},
			tuix.NewStyle().Background(
				tuix.Hex("#222222"),
			),
			tuix.Text(marker, markerStyle),
			tuix.WrappedText(prompt, tuix.NewStyle()),
		),
	)
}

package view

import (
	"flux/src/config"
	"fmt"

	"github.com/anirban1809/tuix/tuix"
)

func Banner(props tuix.Props) tuix.Element {
	return tuix.Box(
		tuix.Props{
			Direction: tuix.Column,
		},
		tuix.NewStyle().Foreground(tuix.Hex("#a2a2a2")),
		tuix.Text(
			fmt.Sprintf("Flux v%s", config.Cfg.AppVersion),
			tuix.NewStyle(),
		),
		tuix.Text("Press / for options", tuix.NewStyle()),
	)
}

package view

import (
	"flux/src/events"
	"fmt"

	"github.com/anirban1809/tuix/tuix"
)

func PlanView(props tuix.Props) tuix.Element {
	plan, ok := props.Get("plan").(events.PlanStatusEvent)
	if !ok || len(plan.Steps) == 0 {
		return tuix.Box(tuix.Props{}, tuix.NewStyle())
	}

	completed := 0
	for _, s := range plan.Steps {
		if s.Status == events.PlanStepCompleted {
			completed++
		}
	}

	title := plan.Title
	if title == "" {
		title = "Plan"
	}

	header := tuix.Box(
		tuix.Props{Direction: tuix.Row, Gap: 1},
		tuix.NewStyle(),
		tuix.Text(
			fmt.Sprintf("Plan: %s", title),
			tuix.NewStyle().Foreground(tuix.Hex("#c8c8c8")).Bold(true),
		),
		tuix.Text(
			fmt.Sprintf("(%d / %d)", completed, len(plan.Steps)),
			tuix.NewStyle().Foreground(tuix.Hex("#848484")),
		),
	)

	rows := []tuix.Element{header, tuix.Text("", tuix.NewStyle())}

	for i, s := range plan.Steps {
		marker, style := stepGlyph(s.Status)
		rows = append(rows, tuix.Box(
			tuix.Props{Direction: tuix.Row, Width: tuix.Fit()},
			tuix.NewStyle(),
			tuix.Box(
				tuix.Props{Width: tuix.Fixed(4)},
				tuix.NewStyle(),
				tuix.WrappedText(marker, style),
			),
			tuix.Box(
				tuix.Props{Width: tuix.Grow(20)},
				tuix.NewStyle(),
				tuix.WrappedText(
					fmt.Sprintf("%d. %s", i+1, s.Outline),
					stepLabelStyle(s.Status),
				),
			),
		))
	}

	return tuix.Box(
		tuix.Props{Direction: tuix.Column, Padding: [4]int{0, 1, 0, 1}},
		tuix.NewStyle().Border(tuix.Border{
			Top: true, Bottom: true,
			Color: tuix.Hex("#3a3a3a"),
		}),
		rows...,
	)
}

func stepGlyph(s events.PlanStepStatus) (string, tuix.Style) {
	switch s {
	case events.PlanStepCompleted:
		return "✓", tuix.NewStyle().Foreground(tuix.Hex("#67c27a")).Bold(true)
	case events.PlanStepRunning:
		glyph := "·"
		if tuix.CurrentTick {
			glyph = "▸"
		}
		return glyph, tuix.NewStyle().Foreground(tuix.Hex("#64c3ff")).Bold(true)
	case events.PlanStepFailed:
		return "✕", tuix.NewStyle().Foreground(tuix.Hex("#e06c75")).Bold(true)
	default:
		return "·", tuix.NewStyle().Foreground(tuix.Hex("#5a5a5a"))
	}
}

func stepLabelStyle(s events.PlanStepStatus) tuix.Style {
	switch s {
	case events.PlanStepCompleted:
		return tuix.NewStyle().Foreground(tuix.Hex("#a8a8a8"))
	case events.PlanStepRunning:
		return tuix.NewStyle().Foreground(tuix.Hex("#e8e8e8")).Bold(true)
	case events.PlanStepFailed:
		return tuix.NewStyle().Foreground(tuix.Hex("#e06c75"))
	default:
		return tuix.NewStyle().Foreground(tuix.Hex("#6e6e6e"))
	}
}

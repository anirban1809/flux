package agent

import "flux/src/events"

type Plan struct {
	Title   string
	Steps   []events.PlanStep
	Current int
	Active  bool
}

func newPlan(title string, outlines []string) *Plan {
	steps := make([]events.PlanStep, len(outlines))
	for i, o := range outlines {
		steps[i] = events.PlanStep{Outline: o, Status: events.PlanStepPending}
	}
	return &Plan{
		Title:  title,
		Steps:  steps,
		Active: true,
	}
}

func (p *Plan) snapshot() events.PlanStatusEvent {
	steps := make([]events.PlanStep, len(p.Steps))
	copy(steps, p.Steps)
	return events.PlanStatusEvent{
		Title:   p.Title,
		Steps:   steps,
		Current: p.Current,
		Active:  p.Active,
	}
}

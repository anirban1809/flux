package view

import (
	"fmt"

	"zipcode/src/config"
	"zipcode/src/events"
	llm "zipcode/src/llm/provider"
	"zipcode/src/view/viewctx"

	"github.com/anirban1809/tuix/tuix"
)

func ModelSelection(props tuix.Props) tuix.Element {
	setActiveView := props.Get("setActiveView").(func(string))
	visible := props.Get("visible").(bool)
	context := tuix.UseContext(viewctx.MainContext)

	items := []string{}
	provider := context.Runtime.Registry.GetProvider(
		llm.ProviderName(config.Cfg.ActiveProviderName),
	)
	if provider != nil {
		for _, m := range provider.Models() {
			items = append(items, m.ID)
		}
	}

	return tuix.Box(
		tuix.Props{
			Direction: tuix.Column,
			Padding:   [4]int{1, 1, 1, 1},
		},
		tuix.NewStyle(),
		tuix.Text("Choose your model:", tuix.NewStyle()),
		tuix.Text("", tuix.NewStyle()),
		Menu(tuix.Props{Values: map[string]any{
			"items":    items,
			"visible":  visible,
			"viewSize": 6,
		}}, func(selected string, _ int) {
			config.Cfg.CurrentModel = selected
			if config.Cfg.ProviderModels == nil {
				config.Cfg.ProviderModels = map[string]string{}
			}
			config.Cfg.ProviderModels[config.Cfg.ActiveProviderName] = selected
			config.Cfg.Save()
			events.EventManager.WriteToChannel(
				events.NOTIFICATION_CHANNEL,
				events.Notification{
					Type: events.INFO,
					Message: fmt.Sprintf(
						"Current model changed to: %s",
						selected,
					),
				},
			)
			context.SetFocusPrompt(true)
			setActiveView("")
		}, nil),
		tuix.Text("Press Enter to confirm, Esc to cancel", tuix.NewStyle()),
	)
}

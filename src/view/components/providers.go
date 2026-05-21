package view

import (
	"fmt"

	"flux/src/config"
	"flux/src/credentials"
	"flux/src/events"
	llm "flux/src/llm/provider"
	view "flux/src/ui/components/utils"
	"flux/src/utils"
	"flux/src/view/viewctx"

	"github.com/anirban1809/tuix/tuix"
	"github.com/anirban1809/tuix/tuix/components"
	"github.com/mattn/go-runewidth"
)

func resolveModelFor(provider llm.Provider, name string) string {
	if saved, ok := config.Cfg.ProviderModels[name]; ok && saved != "" {
		return saved
	}
	if provider != nil {
		if models := provider.Models(); len(models) > 0 {
			return models[0].ID
		}
	}
	return config.Cfg.CurrentModel
}

func ProviderView(props tuix.Props) tuix.Element {
	name, _ := props.Get("name").(string)
	setProviderView, _ := props.Get("setProviderView").(func(bool))
	visible, _ := props.Get("visible").(bool)
	apiKeyValue, setApiKeyValue := tuix.UseState("")
	valueCheck, setValueCheck := tuix.UseState(false)
	valueCheckStatus, setValueCheckStatus := tuix.UseState("")
	context := tuix.UseContext(viewctx.MainContext)

	if !visible {
		return view.Empty()
	}

	if tuix.CurrentKey.Code == tuix.KeyEscape {
		setApiKeyValue("")
		setProviderView(false)
	}

	if tuix.CurrentKey.Code == tuix.KeyEnter {
		result := context.Runtime.Validator.Validate(
			llm.ProviderName(name),
			apiKeyValue,
		)

		if result.Status != credentials.Valid {
			setValueCheckStatus(
				fmt.Sprintf("Failed to check api key: %s", result.LastError),
			)
		} else {
			err := context.Runtime.CredStore.Set(
				llm.ProviderName(name),
				apiKeyValue,
			)

			if err != nil {
				setValueCheckStatus(
					fmt.Sprintf("Failed to save api key: %s", err.Error()),
				)
			} else {
				setValueCheckStatus(
					fmt.Sprintf("Successfully configured api key for %s", name),
				)
			}
		}

		events.EventManager.WriteToChannel(
			events.NOTIFICATION_CHANNEL,
			events.Notification{
				Type: events.INFO,
				Message: fmt.Sprintf(
					"Successfully configured api key for %s",
					name,
				),
			},
		)
		setValueCheck(true)
	}

	return tuix.Box(
		tuix.Props{Direction: tuix.Column},
		tuix.NewStyle(),
		tuix.Text(fmt.Sprintf("Configure %s provider", name), tuix.NewStyle()),
		view.NewLine(),
		view.NewLine(),
		tuix.Text("Enter API Key ", tuix.NewStyle()),
		components.Input(
			">",
			"_",
			true,
			apiKeyValue,
			func(value string) { setApiKeyValue(value) },
		),
		view.NewLine(),
		tuix.If(
			valueCheck,
			tuix.Text(valueCheckStatus, tuix.NewStyle()), view.Empty()),
		view.NewLine(),
		view.NewLine(),
		tuix.If(valueCheck, tuix.Text(
			"Esc to go back",
			tuix.NewStyle().Foreground(tuix.Hex("#cbcbcb")),
		), tuix.Text(
			"Esc to cancel, Enter to confirm",
			tuix.NewStyle().Foreground(tuix.Hex("#cbcbcb")),
		)),
	)
}

func Providers(props tuix.Props) tuix.Element {
	selectedItem, setSelectedItem := tuix.UseState(0)
	visible := props.Get("visible").(bool)
	providerView, setProviderView := tuix.UseState(false)
	setActiveView := props.Get("setActiveView").(func(string))
	focussedIndex, setFocussedIndex := tuix.UseState(0)
	selectedProvider, setSelectedProvider := tuix.UseState(-1)
	context := tuix.UseContext(viewctx.MainContext)

	type item struct {
		name          string
		status        string
		lastValidated string
	}

	allProviders := context.Runtime.Registry.ProviderList()

	items := []item{}

	for _, k := range allProviders {
		result := context.Runtime.Validator.Status(k)
		items = append(items, item{
			name: string(
				k,
			), status: string(result.Status), lastValidated: utils.HumanTime(result.CheckedAt),
		})
	}

	maxNameLen := 0
	for _, it := range items {
		if len(it.name) > maxNameLen {
			maxNameLen = runewidth.StringWidth(it.name)
		}
	}

	labels := utils.Map(items, func(item item, index int) string {
		var timestamplabel string

		if item.lastValidated == "" {
			timestamplabel = ""
		} else {
			timestamplabel = fmt.Sprintf("(last checked %s)", item.lastValidated)
		}

		return fmt.Sprintf(
			"%-*s         %s %s %s",
			maxNameLen,
			item.name,
			item.status,
			timestamplabel,
			utils.If(selectedProvider == index, "[selected]", ""),
		)
	})

	if tuix.CurrentKey.Code == tuix.KeyEscape {
		setActiveView("")
	}

	providerEl := ProviderView(tuix.Props{Values: map[string]any{
		"name":            items[selectedItem].name,
		"setProviderView": setProviderView,
		"visible":         providerView,
	}})

	list := tuix.Box(tuix.Props{Direction: tuix.Column}, tuix.NewStyle(),
		tuix.Text("Configure your providers", tuix.NewStyle()),
		view.NewLine(),
		Menu(tuix.Props{Values: map[string]any{
			"items":    labels,
			"viewSize": 6,
			"visible":  visible && !providerView,
		}}, func(selected string, index int) {
			setSelectedItem(index)
			setProviderView(true)
		}, func(index int) {
			setFocussedIndex(index)
		}),
	)

	if tuix.CurrentKey.Code == tuix.KeySpace {
		setSelectedProvider(focussedIndex)
		config.Cfg.ActiveProviderName = (items[focussedIndex].name)
		context.Runtime.CurrentProvider = context.Runtime.Registry.GetProvider(
			llm.ProviderName(config.Cfg.ActiveProviderName),
		)
		config.Cfg.CurrentModel = resolveModelFor(
			context.Runtime.CurrentProvider,
			config.Cfg.ActiveProviderName,
		)
		config.Cfg.Save()
	}

	return tuix.Box(
		tuix.Props{
			Direction: tuix.Column,
			Padding:   [4]int{1, 1, 1, 1},
		},
		tuix.NewStyle(),
		tuix.If(
			providerView,
			providerEl,
			list,
		),
	)
}

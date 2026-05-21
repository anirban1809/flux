package view

import (
	"flux/src/config"
	llm "flux/src/llm/provider"
	"flux/src/view/viewctx"
	"fmt"

	"github.com/anirban1809/tuix/tuix"
)

func StatusLine(props tuix.Props) tuix.Element {
	context := tuix.UseContext(viewctx.MainContext)
	workspacePath, _ := props.Get("workspacePath").(string)
	status := "Idle"
	if running, _ := props.Get("running").(bool); running {
		status = "Running"
	}

	if skill, _ := props.Get("activeSkill").(string); skill != "" {
		status = fmt.Sprintf("Running /%s", skill)
	}

	branch := "main"
	if v, ok := props.Get("branch").(string); ok && v != "" {
		branch = v
	}

	hasUncommittedChanges := props.Get("hasUncommittedChanges").(bool)

	inputTokens := 0
	cachedInputTokens := 0
	cacheWriteTokens := 0
	outputTokens := 0
	if v, ok := props.Get("inputTokens").(int); ok {
		inputTokens = v
	}
	if v, ok := props.Get("cachedInputTokens").(int); ok {
		cachedInputTokens = v
	}
	if v, ok := props.Get("cacheWriteTokens").(int); ok {
		cacheWriteTokens = v
	}
	if v, ok := props.Get("outputTokens").(int); ok {
		outputTokens = v
	}
	if cachedInputTokens > inputTokens {
		cachedInputTokens = inputTokens
	}
	totalTokens := inputTokens + outputTokens

	branchStyle := tuix.NewStyle()
	if hasUncommittedChanges {
		branchStyle = branchStyle.Foreground(
			tuix.Hex("#0097d7"),
		) // Yellow/orange for uncommitted changes
	} else {
		branchStyle = branchStyle.Foreground(tuix.Hex("#00c732"))
	}

	var providerName string
	var modelName string
	contextWindow := 0
	var inputCostPerM, outputCostPerM float64
	var cacheReadCostPerM, cacheWriteCostPerM float64

	if config.Cfg.ActiveProviderName == "" {
		providerName = "Unconfigured"
		modelName = "Unconfigured"
	} else {
		providerName = string(config.Cfg.ActiveProviderName)
		modelName = config.Cfg.CurrentModel
		if context != nil && context.Runtime != nil {
			contextWindow = context.Runtime.Registry.ContextWindowFor(
				llm.ProviderName(config.Cfg.ActiveProviderName),
				config.Cfg.CurrentModel,
			)
			inputCostPerM, outputCostPerM = context.Runtime.Registry.CostFor(
				llm.ProviderName(config.Cfg.ActiveProviderName),
				config.Cfg.CurrentModel,
			)
			cacheReadCostPerM, cacheWriteCostPerM = context.Runtime.Registry.CacheRatesFor(
				llm.ProviderName(config.Cfg.ActiveProviderName),
				config.Cfg.CurrentModel,
			)
		}
	}

	regularInputTokens := inputTokens - cachedInputTokens - cacheWriteTokens
	if regularInputTokens < 0 {
		regularInputTokens = 0
	}
	sessionCost := (float64(regularInputTokens)*inputCostPerM +
		float64(cacheWriteTokens)*cacheWriteCostPerM +
		float64(cachedInputTokens)*cacheReadCostPerM +
		float64(outputTokens)*outputCostPerM) / 1_000_000
	var costText string
	if inputCostPerM > 0 || outputCostPerM > 0 {
		costText = fmt.Sprintf(" | $%.4f", sessionCost)
	}

	var contextPctText string
	if contextWindow > 0 {
		contextPctText = fmt.Sprintf(
			"Context: %0.2f%% (of %d)",
			float32(totalTokens*100)/float32(contextWindow),
			contextWindow,
		)
	} else {
		contextPctText = "Context: -"
	}

	line1 := tuix.Box(
		tuix.Props{Direction: tuix.Row, Justify: tuix.JustifySpaceBetween},
		tuix.NewStyle(),
		tuix.Text(
			fmt.Sprintf("%s | %s (%s)", status, workspacePath, branch),
			branchStyle,
		),
		tuix.Text(
			fmt.Sprintf(
				"Tokens:  %d\u2191 / %d\u2193 (%d)%s",
				inputTokens,
				outputTokens,
				totalTokens,
				costText,
			),
			tuix.NewStyle(),
		),
	)

	yoloText := ""
	yoloStyle := tuix.NewStyle()
	if config.Cfg.YoloMode {
		yoloText = " | YOLO"
		yoloStyle = yoloStyle.Foreground(tuix.Hex("#ff8c00")).Bold(true)
	}

	line2 := tuix.Box(
		tuix.Props{Direction: tuix.Row, Justify: tuix.JustifySpaceBetween},
		tuix.NewStyle(),
		tuix.Box(
			tuix.Props{Direction: tuix.Row},
			tuix.NewStyle(),
			tuix.Text(
				fmt.Sprintf(
					"Model: %s (%s)",
					modelName,
					providerName,
				),
				tuix.NewStyle(),
			),
			tuix.Text(yoloText, yoloStyle),
		),
		tuix.Text(
			contextPctText,
			tuix.NewStyle(),
		),
	)

	return tuix.Box(
		tuix.Props{
			Direction: tuix.Column,
			Padding:   [4]int{1, 1, 1, 1},
			Justify:   tuix.JustifySpaceBetween,
		},
		tuix.NewStyle().Foreground(tuix.Hex("#a2a2a2")),
		tuix.Box(
			tuix.Props{Direction: tuix.Column},
			tuix.NewStyle(),
			line1, line2,
		),
	)
}

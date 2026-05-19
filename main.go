package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"zipcode/src/agent"
	"zipcode/src/config"
	llm "zipcode/src/llm/provider"
	"zipcode/src/utils"
	"zipcode/src/view"
	"zipcode/src/workspace"

	"github.com/anirban1809/tuix/tuix"
)

func main() {
	if err := config.Load(); err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	var (
		promptFlag    string
		workspaceFlag string
		providerFlag  string
		modelFlag     string
		maxTurnsFlag  int
		verboseFlag   bool
	)
	flag.StringVar(&promptFlag, "prompt", "", "run a single prompt headless and exit")
	flag.StringVar(&promptFlag, "p", "", "alias for --prompt")
	flag.StringVar(&workspaceFlag, "workspace", "", "override the working directory")
	flag.StringVar(&workspaceFlag, "C", "", "alias for --workspace")
	flag.StringVar(&providerFlag, "provider", "", "override the active provider (headless only, not persisted)")
	flag.StringVar(&modelFlag, "model", "", "override the active model (headless only, not persisted)")
	flag.IntVar(&maxTurnsFlag, "max-turns", 0, "headless agent-loop turn cap (0 = use config/env default)")
	flag.BoolVar(&verboseFlag, "verbose", false, "headless: log every tool call, arguments, and result preview to stderr")
	flag.BoolVar(&verboseFlag, "v", false, "alias for --verbose")
	flag.Parse()

	if promptFlag != "" {
		runHeadless(promptFlag, workspaceFlag, providerFlag, modelFlag, maxTurnsFlag, verboseFlag)
		return
	}

	width, height, _ := utils.GetTerminalSize()
	dir, _ := os.Getwd()
	ws := workspace.Load(dir)

	runtime := agent.NewRuntime(&ws)

	app := tuix.NewApp(width, height)
	app.Run(view.App, tuix.Props{Values: map[string]any{"runtime": &runtime, "wd": dir}})
}

func runHeadless(prompt, workspaceOverride, providerOverride, modelOverride string, maxTurnsOverride int, verbose bool) {
	config.Cfg.Headless = true
	config.Cfg.Verbose = verbose

	if maxTurnsOverride > 0 {
		config.Cfg.MaxHeadlessTurns = maxTurnsOverride
	}

	if providerOverride != "" {
		canonical, err := llm.GetProviderName(providerOverride)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unsupported provider %q\n", providerOverride)
			os.Exit(1)
		}
		config.Cfg.ActiveProviderName = string(canonical)
	}
	if modelOverride != "" {
		// Strip a `<provider>/` prefix that matches the active provider so
		// callers can pass Harbor-style "openrouter/minimax/minimax-m2.5"
		// without it leaking into the wire-level model id.
		if config.Cfg.ActiveProviderName != "" {
			prefix := strings.ToLower(config.Cfg.ActiveProviderName) + "/"
			if strings.HasPrefix(strings.ToLower(modelOverride), prefix) {
				modelOverride = modelOverride[len(prefix):]
			}
		}
		if config.Cfg.ProviderModels == nil {
			config.Cfg.ProviderModels = map[string]string{}
		}
		config.Cfg.ProviderModels[config.Cfg.ActiveProviderName] = modelOverride
	}

	dir := workspaceOverride
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to determine working directory: %v\n", err)
			os.Exit(1)
		}
		dir = cwd
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to resolve workspace %q: %v\n", dir, err)
		os.Exit(1)
	}
	if err := os.Chdir(absDir); err != nil {
		fmt.Fprintf(os.Stderr, "failed to enter workspace %q: %v\n", absDir, err)
		os.Exit(1)
	}
	dir = absDir

	ws := workspace.Load(dir)
	runtime := agent.NewRuntime(&ws)

	if runtime.CurrentProvider == nil {
		fmt.Fprintln(os.Stderr, "no active provider configured: set ActiveProviderName in ~/.zipcode/config.toml or pass --provider")
		os.Exit(1)
	}
	providerName := runtime.CurrentProvider.Name()
	if _, ok := runtime.CredStore.Get(providerName); !ok {
		fmt.Fprintf(os.Stderr, "no credentials for provider %q: configure ~/.zipcode/credentials.toml or set the provider's API key env var\n", providerName)
		os.Exit(1)
	}

	go func() {
		for {
			agent.EventManager.ReadFromChannel(agent.NOTIFICATION_CHANNEL)
		}
	}()
	go func() {
		for {
			agent.EventManager.ReadFromChannel(agent.PLAN_STATUS_CHANNEL)
		}
	}()

	send := prompt
	if name, _, ok := runtime.ParseSkillCommand(prompt); ok {
		send = runtime.ExpandSkillCommand(prompt)
		runtime.Executor.SetActiveSkill(name)
	}

	msg, err := runtime.Run(send)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if msg != nil {
		fmt.Println(msg.Content)
	}
}

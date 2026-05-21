package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"flux/src/agent"
	"flux/src/config"
	"flux/src/events"
	llm "flux/src/llm/provider"
	"flux/src/utils"
	"flux/src/view"
	"flux/src/workspace"

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
		debugFlag     bool
		yoloflag      bool
	)
	flag.StringVar(
		&promptFlag,
		"prompt",
		"",
		"run a single prompt headless and exit",
	)
	flag.StringVar(&promptFlag, "p", "", "alias for --prompt")
	flag.StringVar(
		&workspaceFlag,
		"workspace",
		"",
		"override the working directory",
	)
	flag.StringVar(&workspaceFlag, "C", "", "alias for --workspace")
	flag.StringVar(
		&providerFlag,
		"provider",
		"",
		"override the active provider (headless only, not persisted)",
	)
	flag.StringVar(
		&modelFlag,
		"model",
		"",
		"override the active model (headless only, not persisted)",
	)
	flag.IntVar(
		&maxTurnsFlag,
		"max-turns",
		0,
		"headless agent-loop turn cap (0 = use config/env default)",
	)
	flag.BoolVar(
		&debugFlag,
		"debug",
		false,
		"write verbose debug logs to ~/.flux/debug.log (headless only)",
	)
	flag.BoolVar(&debugFlag, "d", false, "alias for --debug")
	flag.BoolVar(
		&yoloflag,
		"yolo",
		false,
		"auto-accept all file changes without prompting",
	)
	flag.BoolVar(&yoloflag, "y", false, "alias for --yolo")
	flag.Parse()

	if promptFlag != "" {
		runHeadless(
			promptFlag,
			workspaceFlag,
			providerFlag,
			modelFlag,
			maxTurnsFlag,
			debugFlag,
			yoloflag,
		)
		return
	}

	width, height, _ := utils.GetTerminalSize()
	dir, _ := os.Getwd()
	ws := workspace.Load(dir)

	runtime := agent.NewRuntime(&ws)

	app := tuix.NewApp(width, height)
	app.Run(
		view.App,
		tuix.Props{
			Values: map[string]any{
				"runtime":       &runtime,
				"wd":            dir,
				"yoloRequested": yoloflag,
			},
		},
	)
}

func setupDebugLog() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "debug: cannot determine home dir: %v\n", err)
		return
	}
	path := filepath.Join(home, ".flux", "debug.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Fprintf(
			os.Stderr,
			"debug: cannot open log file %s: %v\n",
			path,
			err,
		)
		return
	}
	log.SetOutput(f)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	fmt.Fprintf(os.Stderr, "debug log: %s\n", path)
}

func runHeadless(
	prompt, workspaceOverride, providerOverride, modelOverride string,
	maxTurnsOverride int,
	debug bool, yolo bool,
) {

	config.Cfg.YoloMode = yolo

	if debug {
		setupDebugLog()
	}
	config.Cfg.Headless = true

	if maxTurnsOverride > 0 {
		config.Cfg.MaxHeadlessTurns = maxTurnsOverride
	}

	if providerOverride != "" {
		canonical, err := llm.GetProviderName(providerOverride)
		if err != nil {
			fmt.Fprintf(
				os.Stderr,
				"unsupported provider %q\n",
				providerOverride,
			)
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
			fmt.Fprintf(
				os.Stderr,
				"failed to determine working directory: %v\n",
				err,
			)
			os.Exit(1)
		}
		dir = cwd
	}

	ws := workspace.Load(dir)
	runtime := agent.NewRuntime(&ws)

	if runtime.CurrentProvider == nil {
		fmt.Fprintln(
			os.Stderr,
			"no active provider configured: set ActiveProviderName in ~/.flux/config.toml or pass --provider",
		)
		os.Exit(1)
	}
	providerName := runtime.CurrentProvider.Name()
	if _, ok := runtime.CredStore.Get(providerName); !ok {
		fmt.Fprintf(
			os.Stderr,
			"no credentials for provider %q: configure ~/.flux/credentials.toml or set the provider's API key env var\n",
			providerName,
		)
		os.Exit(1)
	}

	go func() {
		for {
			events.EventManager.ReadFromChannel(events.NOTIFICATION_CHANNEL)
		}
	}()
	go func() {
		for {
			events.EventManager.ReadFromChannel(events.PLAN_STATUS_CHANNEL)
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

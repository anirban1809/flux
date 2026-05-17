package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Headless              bool              `toml:"headless"`
	AppVersion            string            `toml:"app_version"`
	ModelNames            []string          `toml:"model_names"`
	CurrentModel          string            `toml:"current_model"`
	InternalToolPath      string            `toml:"internal_tool_path"`
	ExternalToolPath      string            `toml:"external_tool_path"`
	InternalSubagentsPath string            `toml:"internal_subagents_path"`
	ExternalSubagentsPath string            `toml:"external_subagents_path"`
	InternalSkillsPath    string            `toml:"internal_skills_path"`
	GlobalSkillsPath      string            `toml:"global_skills_path"`
	ProjectSkillsPath     string            `toml:"project_skills_path"`
	SkillsStatePath       string            `toml:"skills_state_path"`
	HomeDir               string            `toml:"home_dir"`
	CredentialsPath       string            `toml:"credentials_path"`
	AccountPath           string            `toml:"account_path"`
	ConfigPath            string            `toml:"config_path"`
	ActiveProviderName    string            `toml:"active_provider_name"`
	ProviderModels        map[string]string `toml:"provider_models"`
}

var Cfg = &Config{}

func defaults() *Config {
	return &Config{
		Headless:   false,
		AppVersion: "0.0.1",
		ModelNames: []string{
			"openai/gpt-5.2",
			"openai/gpt-5.5",
			"minimax/minimax-m2.5",
			"minimax/minimax-m2.7",
			"anthropic/claude-sonnet-4.6",
			"anthropic/claude-haiku-4.5",
			"openai/gpt-5.1-codex-mini",
			"moonshotai/kimi-k2.5",
			"meta-llama/llama-3.3-70b-instruct",
			"z-ai/glm-4.7",
			"qwen/qwen3-coder-flash",
			"openai/gpt-5-nano",
			"z-ai/glm-5",
			"openai/gpt-5.4-nano",
			"deepseek/deepseek-v3.2",
			"openai/gpt-5.4",
			"openai/gpt-5.3-codex",
			"z-ai/glm-5v-turbo",
		},
		CurrentModel:          "minimax/minimax-m2.5",
		InternalToolPath:      "/Users/anirban/Documents/Code/zipcode/src/tools",
		ExternalToolPath:      "~/.zipcode/tools",
		InternalSubagentsPath: "/Users/anirban/Documents/Code/zipcode/src/subagents",
		ExternalSubagentsPath: "~/.zipcode/tools",
		InternalSkillsPath:    "/Users/anirban/Documents/Code/zipcode/src/skills/builtin",
		GlobalSkillsPath:      "~/.zipcode/skills",
		ProjectSkillsPath:     ".zipcode/skills",
		SkillsStatePath:       "~/.zipcode/skills.state.json",
		HomeDir:               "~/.zipcode",
		CredentialsPath:       "~/.zipcode/credentials.toml",
		ConfigPath:            "~/.zipcode/config.toml",
		ProviderModels:        map[string]string{},
	}
}

func zipcodeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".zipcode"), nil
}

func Load() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".zipcode")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	defaultsPath := filepath.Join(dir, "defaults.toml")
	configPath := filepath.Join(dir, "config.toml")

	*Cfg = *defaults()

	if _, err := os.Stat(defaultsPath); errors.Is(err, os.ErrNotExist) {
		if err := writeTOML(defaultsPath, Cfg); err != nil {
			return err
		}
	} else if err == nil {
		if _, err := toml.DecodeFile(defaultsPath, Cfg); err != nil {
			return err
		}
	} else {
		return err
	}

	if _, err := os.Stat(configPath); err == nil {
		if _, err := toml.DecodeFile(configPath, Cfg); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	Cfg.expandPaths(home)
	return nil
}

func (c *Config) Save() error {
	dir, err := zipcodeDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {

		return err
	}
	return writeTOML(filepath.Join(dir, "config.toml"), c)
}

func (c *Config) SetCurrentModel(model string) {
	c.CurrentModel = model
}

func (c *Config) expandPaths(home string) {
	c.InternalToolPath = expand(c.InternalToolPath, home)
	c.ExternalToolPath = expand(c.ExternalToolPath, home)
	c.InternalSubagentsPath = expand(c.InternalSubagentsPath, home)
	c.ExternalSubagentsPath = expand(c.ExternalSubagentsPath, home)
	c.InternalSkillsPath = expand(c.InternalSkillsPath, home)
	c.GlobalSkillsPath = expand(c.GlobalSkillsPath, home)
	c.SkillsStatePath = expand(c.SkillsStatePath, home)
	c.HomeDir = expand(c.HomeDir, home)
	c.CredentialsPath = expand(c.CredentialsPath, home)
	c.ConfigPath = expand(c.ConfigPath, home)
}

func expand(p, home string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

func writeTOML(path string, v any) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(v); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- AWS Bedrock provider with full Converse API support, streaming, SigV4 auth, and model listing (`src/llm/provider/bedrock.go`, `src/llm/provider/provider.go`, `src/llm/provider/registry.go`)
- `git` tool definition with JSON schema for structured git operations (`src/tools/git.go`, `src/tools/git/git.json`, `src/tools/git/git.py`)

### Changed
- System prompt now instructs the agent to use `git` tool instead of `bash_tool` for git commands (`src/llm/prompts/mainprompt.go`)
- Banner version display is now dynamic, reading from `config.Cfg.AppVersion` instead of a hardcoded string (`src/view/components/banner.go`)
- Default `AppVersion` in config bumped to `0.0.3` (`src/config/config.go`)

### Fixed
- Prevent form submission when prompt is empty (`src/view/app.go`)

## [0.0.3] - 2026-05-24

### Added
- `install.sh` — one-command install script that resolves the latest GitHub release, downloads the platform binary, and verifies SHA256 checksum
- Directory trust prompt on first run in a new directory (`src/view/components/trustconfirm.go`, `src/config/config.go`)
- `AllowedDirs` config field with `IsDirTrusted()` and `TrustDir()` helpers (`src/config/config.go`)
- Panic recovery writing timestamped crash logs to `~/.flux/panic-<timestamp>.log` (`src/config/panic.go`, `main.go`, `src/view/app.go`)
- `CompactionEvent` type and `COMPACTION_CHANNEL` in the event bus (`src/events/eventbus.go`)
- Compaction badge in the statusline showing tokens condensed after auto-compact (`src/view/components/statusline.go`)
- `Executor.PlanActive` flag and `EmitMessage()` helper to suppress mid-plan messages and emit the final response on plan completion (`src/agent/executor.go`)
- "Further instructions…" free-text entry option in the question-response menu (`src/view/app.go`)
- `StreamOptions{IncludeUsage: true}` forwarded to OpenRouter when streaming is enabled (`src/llm/provider/openrouter.go`)
- `OpenRouterApiError` type to parse and surface structured API error messages on HTTP 4xx/5xx (`src/llm/provider/openrouter.go`)

### Changed
- Renamed project to flux across all packages, config, docs, and build files
- Default internal tool, subagent, and skills paths changed from absolute to relative (`src/config/config.go`)
- Internal tools now take priority over external tools during resolution (`src/agent/executor.go`)
- Tool call argument handling supports non-string JSON types (numbers, booleans, objects) (`src/agent/executor.go`)
- Plan view is hidden once the plan completes; step labels use wrapped text (`src/view/components/plan.go`)
- Notification text uses wrapped text to prevent truncation (`src/view/app.go`)
- Auto-compact notification now includes the token count condensed (`src/agent/runtime.go`)
- OpenRouter `Complete()` removes the infinite retry loop; returns a structured error on empty choices (`src/llm/provider/openrouter.go`)
- `ProviderView` constructs the `Input` element unconditionally to preserve tuix hook ordering (`src/view/components/providers.go`)
- Plan completion now emits the final assistant message to the UI (`src/agent/runtime.go`)

### Removed
- `CHANGES.md` (superseded by `CHANGELOG.md`)

## [0.0.2] - 2026-05-21

### Added
- Headless/YOLO mode with CLI flags (`--provider`, `--model`, `--max-turns`, `--debug`, `--yolo` auto-accept) (`main.go`, `src/view/components/yoloconfirm.go`)
- GitHub Actions release workflow building cross-platform binaries with SHA256SUMS (`.github/workflows/release.yml`)
- Autocomplete for `/` commands in the prompt menu (`src/view/components/menu.go`)
- Plan display and question-response support in the UI (`src/view/components/plan.go`)
- Workspace context view with cached token accounting (`src/view/components/context.go`)
- Conversation compaction and per-model cost metadata (`src/llm/provider/`)
- Multi-provider support (Anthropic, OpenAI, OpenRouter) with a credentials store (`src/credentials/`, `src/llm/provider/`)
- Provider registry and structured event notifications (`src/llm/provider/registry.go`)
- Skills support with a resolver (`src/skills/resolver.go`)
- Session history and re-opening past sessions (`src/workspace/session.go`)
- Git branch and token tracking in statusline (`src/view/components/statusline.go`)
- Subagent support converted to individual tools (`src/agent/`)
- Model switcher UI (`src/view/components/modelselection.go`)
- External and internal plugin/tool support, including a Python-based file read tool (`src/tools/`)
- Parallel and sequential tool call execution (`src/agent/executor.go`)
- Basic file diff viewer (`src/view/components/filediff.go`)
- Token counter and multi-conversation context (`src/view/`)
- Streaming LLM responses (`src/llm/provider/`)
- OpenRouter provider integration (`src/llm/provider/openrouter.go`)
- Event bus for async messaging (`src/events/eventbus.go`)

### Changed
- Migrated UI from bubbletea to tuix (`src/view/`, `src/ui/`)
- Simplified runtime and prompt handling (`src/agent/runtime.go`, `src/llm/prompts/mainprompt.go`)
- Updated system prompt (`src/llm/prompts/mainprompt.go`)
- Refactored event bus out of `src/agent` into `src/events` package

### Fixed
- LLM response edge cases in provider layer
- Token counting bug in statusline (`src/view/components/statusline.go`)
- Removed local tuix module replace directive to unblock CI (`go.mod`)

## [0.0.1] - 2026-02-06

### Added
- Initial bubbletea-based TUI with dynamic viewport and prompt field sizing
- `/` command menu (`src/view/`)
- Status bar with mode switching and auto-update
- Spinner for running prompts
- Message passing from executor to app model (`src/agent/executor.go`)
- Basic tool calling and autonomous change support (`src/agent/`)
- LLM streaming support and basic LLM integration (`src/llm/`)
- Initial runtime and plan validation stubs (`src/agent/runtime.go`)

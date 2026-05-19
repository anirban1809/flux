# Changelog

## Uncommitted changes

### Overview

- Added provider-level streaming support and live TUI rendering for assistant response deltas.
- Added headless `--verbose` / `-v` logging for tool calls, tool results, skills, and subagents.
- Added new `code_search` and `file_search` tool definitions and Python implementations.
- Simplified the main system prompt and strengthened headless-mode operating instructions.
- Improved OpenAI conversation recovery for missing tool-response errors.
- Removed the static `src/llm/models.json` model list.

### File-by-file summary

#### `.gitignore`

- Added `__pycache__/` to ignore generated Python bytecode cache directories.

#### `go.mod`

- Updated `github.com/anirban1809/tuix` from `v0.0.15` to `v0.0.17`.

#### `main.go`

- Added headless `--verbose` and `-v` flags.
- Passes the verbose setting into runtime config for headless runs.
- Resolves the headless workspace to an absolute path and changes the process working directory before loading the workspace.
- Imports `path/filepath` for workspace path normalization.

#### `src/agent/agent.go`

- Added an `OnStream` callback field to `Agent`.
- Enables streaming chat requests when a stream callback is registered.

#### `src/agent/executor.go`

- Added `MessageDelta` and `MessageComplete` response event types for streaming output.
- Suppresses duplicate full-message emission for streamed responses.
- Returns tool display names from tool-command resolution.
- Logs tool calls and result previews when headless verbose mode is enabled.
- Updates tool-call UI event text to use display names instead of raw function names.

#### `src/agent/runtime.go`

- Added `verboseLog` and `truncate` helpers for headless verbose diagnostics.
- Wires non-headless runtimes to stream text deltas into the agent output channel.
- Emits `MessageComplete` after streamed top-level responses finish.
- Adds verbose logging around subagent execution and skill invocation.
- Keeps existing runtime fields but reformats alignment after the new stream-related additions.

#### `src/config/config.go`

- Added a non-persisted `Verbose` config field for headless verbose logging.

#### `src/llm/models.json`

- Deleted the static JSON model registry containing OpenAI, Anthropic, Minimax, Moonshot, Meta, Z.ai, Qwen, and DeepSeek model descriptions.

#### `src/llm/prompts/mainprompt.go`

- Replaced the long-form main system prompt with a shorter structured version covering hard rules, work style, planning, careful actions, tools, and replies.
- Rewrote the headless-mode prompt to be more concise while preserving autonomous execution and validation requirements.
- Added Python-specific validation guidance to use `python3` unless project files specify otherwise.
- Added stronger explicit validation guidance for malformed inputs, boundaries, no-mutation behavior, output formats, and error classes.
- Corrected inline code formatting in the `HeadlessSystemPrompt` comment.

#### `src/llm/provider/provider.go`

- Added `Streamed` metadata to provider messages.
- Added an `OnStream` callback to chat requests.
- Added stream event types and payload structure for streamed text, streamed tool calls, stop events, and errors.
- Reformatted `ModelDescriptor` field alignment.

#### `src/llm/provider/stream.go`

- Added shared SSE reading support for provider streaming implementations.
- Added helpers to merge incremental tool-call deltas and finalize ordered tool calls.
- Added stream callback emission and JSON event decoding helpers.

#### `src/llm/provider/anthropic.go`

- Adds streaming support for Anthropic Messages API requests.
- Sends `Accept: text/event-stream` and `stream: true` for streamed completions.
- Parses Anthropic stream event types including message starts, content block starts, text deltas, input JSON deltas, message deltas, stops, and errors.
- Emits text stream events as deltas arrive and emits finalized tool-call events after the stream completes.
- Normalizes Anthropic usage data, including cache creation and cache read token accounting.

#### `src/llm/provider/openai.go`

- Adds streaming support for OpenAI chat completions with `stream_options.include_usage`.
- Parses streamed chunks for text deltas, tool-call deltas, finish reasons, model/id metadata, and usage.
- Emits text stream events and finalized tool-call events.
- Adds retry-time sanitization for OpenAI `tool_calls` messages that are not followed by matching `tool` messages.
- Reuses the missing-tool-response sanitization path for both streamed and non-streamed requests.

#### `src/llm/provider/openrouter.go`

- Adds streaming support for OpenRouter chat completions.
- Sends `stream_options.include_usage` for streamed responses.
- Parses OpenRouter stream chunks for text deltas, tool-call deltas, finish reasons, metadata, and usage.
- Emits text stream events and finalized tool-call events.

#### `src/tools/common.go`

- Added a `display_name` field to tool function metadata.

#### `src/tools/bash/bash.json`

- Added the display name `Bash` to the Bash tool definition.

#### `src/tools/bash/bash.py`

- Changed command results so `ok` reflects whether the command exit code is zero.
- Adds an `error` field when the command exits with a non-zero status.
- Preserves timeout and exception handling behavior.

#### `src/tools/file_read/file_read.json`

- Added the display name `Read` to the file-read tool definition.

#### `src/tools/code_search/code_search.json`

- Added a new `code_search` tool schema for searching source contents by query and path.
- Requires `message`, `query`, and `path` arguments.

#### `src/tools/code_search/code_search.py`

- Added a Python implementation for `code_search` backed by `rg --json`.
- Validates non-empty queries and existing directory paths.
- Reports missing ripgrep as a structured JSON error.
- Returns up to 200 matches with file, line, column, and content fields plus `truncated` and `count` metadata.

#### `src/tools/file_search/file_search.json`

- Added a new `file_search` tool schema for locating files by name, fragment, regex, or glob.
- Requires `message`, `query`, and `path` arguments.

#### `src/tools/file_search/file_search.py`

- Added a Python implementation for `file_search`.
- Uses `rg --files` when available and falls back to walking the filesystem.
- Skips common generated or dependency directories during fallback traversal.
- Supports glob, substring, and regex matching.
- Returns up to 200 matches with `truncated` and `count` metadata.

#### `src/view/app.go`

- Adds live rendering of streamed assistant text deltas in the TUI.
- Maintains and updates a single streaming output element while deltas arrive.
- Resets streaming state when prompts, tool events, errors, full messages, or completion events occur.
- Updates tool-call display formatting to use a compact symbol-prefixed block.
- Avoids appending visible output for `MessageComplete` events.

#### `src/view/components/statusline.go`

- Changes model status formatting from `Model: <model> (<provider>)` to `<provider>/<model>`.
- Lowercases provider names in the status line.

# flux

A terminal-based AI coding assistant built with Go and tuix. flux provides a TUI (Terminal User Interface) for interacting with AI models through OpenRouter, OpenAI, and Anthropic to perform code exploration, file manipulation, and task execution.

## Overview

flux is a Go TUI application that provides an AI coding assistant interface with:
- Interactive terminal UI built with tuix
- Real AI model integration via OpenRouter, OpenAI, and Anthropic
- Tool execution (file operations, shell commands, code search)
- Sub-agent support for specialized tasks
- Skill support for reusable prompt templates
- Workspace awareness and management

## Features

- **Terminal-first design**: Built with tuix for a smooth TUI experience
- **Real AI integration**: Connects to OpenRouter, OpenAI, and Anthropic for LLM interactions
- **Tool system**: Execute file operations, shell commands, and code searches
- **Sub-agents**: Specialized agents for code exploration and bug investigation
- **Skills**: Reusable prompt templates that can be invoked from the assistant
- **Multiple model support**: Switch between supported models per provider
- **Workspace management**: Track and manage the current working directory
- **Interactive UI**: Command input, file diffs, status display

## Installation

### Prerequisites

- Go 1.26.1 or later
- Git (for build metadata)
- Python 3 (used by external tool scripts)
- ripgrep (`rg`) for search tools

### Building from Source

```bash
# Clone the repository
git clone <repository-url>
cd flux

# Build the binary
make build

# Run the application
./bin/flux
```

### Development

```bash
# Run in development mode with race detector
make dev

# Run tests
make test

# Format code
make fmt

# Run linter
make lint

# Clean build artifacts
make clean

# Build release binaries for multiple platforms
make dist
```

## Usage

### Starting the Application

```bash
# Run from source
make run

# Or run the built binary
./bin/flux
```

### Basic Controls

- **Enter**: Submit prompt or execute selected command
- **Shift+Enter**: Add newline to prompt
- **↑/↓ Arrow Keys**: Navigate menus or scroll results
- **Tab**: Select highlighted menu item
- **Ctrl+C / q / Esc**: Quit application

### Available Models

flux supports multiple providers. The default configuration uses `minimax/minimax-m2.5`.

OpenRouter models include:
- openai/gpt-5.2
- openai/gpt-5.5
- minimax/minimax-m2.5 (default)
- minimax/minimax-m2.7
- anthropic/claude-sonnet-4.6
- anthropic/claude-haiku-4.5
- deepseek/deepseek-v3.2
- meta-llama/llama-3.3-70b-instruct
- qwen/qwen3-coder-flash
- And more configured in `src/llm/provider/openrouter.go` and `src/llm/models.json`

OpenAI and Anthropic providers also expose their own provider-native model IDs.

### Tools

The assistant can use various tools to help with tasks:

| Tool | Description |
|------|-------------|
| `file_read` | Read file contents from the workspace |
| `file_write` | Write or create files in the workspace |
| `bash` | Execute shell commands |
| `code_search` | Search for code patterns in files |
| `file_search` | Find files by name pattern |
| `invoke_skill` | Invoke a registered reusable prompt template |
| `subagent_code_explorer` | Run the code exploration sub-agent |
| `subagent_bug_investigator` | Run the bug investigation sub-agent |

### Sub-agents

Specialized agents for complex tasks:

- **code_explorer**: Analyze and understand codebase structure
- **bug_investigator**: Identify bugs and suggest fixes

The tool manifests exposed to the model are `subagent_code_explorer` and `subagent_bug_investigator`.

## Architecture

### Project Structure

```
flux/
├── main.go              # Application entry point
├── go.mod               # Go module definition
├── Makefile             # Build and development targets
├── bin/                 # Compiled binaries
└── src/
    ├── agent/           # Core agent runtime and executor
    ├── bootstrap/       # Application initialization and flags
    ├── config/          # Configuration (models, paths)
    ├── credentials/     # API key storage, validation, and watching
    ├── llm/             # LLM providers and prompts
    ├── secrets/         # Secret detection and redaction
    ├── skills/          # Skill loading, resolving, and state
    ├── subagents/       # Sub-agent definitions
    ├── tools/           # Tool implementations and manifests/scripts
    ├── ui/              # Deprecated bubbletea UI
    ├── utils/           # Utility functions
    ├── view/            # tuix UI components
    └── workspace/       # Workspace management
```

### Key Components

#### Main Application (`main.go`)
- Initializes terminal size
- Creates workspace from current directory
- Initializes agent runtime
- Launches tuix-based TUI application

#### Agent Runtime (`src/agent/`)
- **Runtime**: Manages the tool-calling loop with LLM
- **Executor**: Processes and executes tool calls
- **Planner**: Coordinates task execution

#### LLM Integration (`src/llm/`)
- OpenRouter, OpenAI, and Anthropic provider integration
- Prompt management
- Model configuration

#### UI Framework (`src/view/`)
- Built with **tuix** for reactive TUI
- Uses **Lipgloss** for styling
- Components: app, menu, prompt, statusline, filediff

## Dependencies

### Core Dependencies
- `github.com/anirban1809/tuix` - TUI framework
- `github.com/charmbracelet/lipgloss` - Styling
- `github.com/BurntSushi/toml` - TOML configuration
- `golang.org/x/term` - Terminal handling

### Development Dependencies
- `golangci-lint` - Go linting and static analysis

## Configuration

Configuration is stored under `~/.flux`:
- `~/.flux/defaults.toml` - generated defaults
- `~/.flux/config.toml` - user configuration
- `~/.flux/credentials.toml` - stored provider API keys

Supported API key environment variables:
- `OPENROUTER_API_KEY` - API key for OpenRouter
- `OPENAI_API_KEY` - API key for OpenAI
- `ANTHROPIC_API_KEY` - API key for Anthropic

Internal/external tool, sub-agent, and skill paths are configured in `src/config/config.go` and can be overridden through the generated config files.

## Build Metadata

The Makefile injects build metadata into the binary:
- Version: Git tag or "dev"
- Commit: Short Git hash or "none"
- Build time: UTC timestamp

## Notes

- The main app builds as a Go binary, but dynamically loaded tool manifests/scripts are read from configured tool paths. External tool execution currently uses Python scripts.
- The current development module includes a local `replace` directive for `github.com/anirban1809/tuix`; remove or update it if building outside the author's local workspace.

## License

[Add your license information here]

## Contributing

[Add contribution guidelines here]
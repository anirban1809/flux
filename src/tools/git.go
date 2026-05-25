package tools

var GitTool = Tool{
	Type: "function",
	Function: ToolFunction{
		Name:        "git",
		Description: "Execute git operations in the workspace repository. Use for all version control tasks: checking status, staging files, committing, viewing diffs, browsing history, managing branches, stashing, etc. Prefer this over the bash tool for any git command.",
		Parameters: JSONSchema{
			Type: "object",
			Properties: map[string]Schema{
				"message": {
					Type:        "string",
					Description: "A short description of the git operation being performed",
				},
				"command": {
					Type:        "string",
					Description: "The git subcommand and arguments (e.g. 'status', 'diff HEAD', 'log --oneline -10', 'add src/main.go', 'commit -m \"fix: handle edge case\"'). Do not include the 'git' prefix.",
				},
				"working_directory": {
					Type:        "string",
					Description: "Path to the git repository root (or any subdirectory within it)",
				},
			},
			Required: []string{"message", "command", "working_directory"},
		},
	},
}

package skills

import (
	"flux/src/workspace"
	"strings"
)

func Resolve(template, args string, ws *workspace.Workspace) string {
	out := template

	out = strings.ReplaceAll(out, "{{args}}", args)

	wsPath := ""
	branch := ""
	if ws != nil {
		wsPath = ws.RootPath
		branch = ws.GetCurrentBranch()
	}

	out = strings.ReplaceAll(out, "{{workspace}}", wsPath)
	out = strings.ReplaceAll(out, "{{git_branch}}", branch)

	return out
}

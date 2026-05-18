package tools

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var FileWriteTool = Tool{
	Type: "function",
	Function: ToolFunction{
		Name:        "file_write",
		Description: "Create or modify files in the workspace. This tool supports multiple operations for editing code safely. Supported operations: create → create a new file, replace → replace entire file contents, append → append content to a file, patch → modify specific parts of a file.",
		Parameters: JSONSchema{
			Type: "object",
			Properties: map[string]Schema{
				"message": {
					Type:        "string",
					Description: "Explanation of why the file write operation is required",
				},
				"file_path": {
					Type:        "string",
					Description: "Path of the file to create or modify",
				},
				"operation": {
					Type:        "string",
					Description: "Type of file write operation",
					Enum: []any{
						"create",
						"replace",
						"append",
						"patch",
					},
				},
				"content": {
					Type:        "string",
					Description: "Content to write for create, replace, or append operations",
				},
				"patches": {
					Type:        "array",
					Description: "List of patch operations to modify specific parts of a file",
					Items: &Schema{
						Type: "object",
						Properties: map[string]Schema{
							"target": {
								Type:        "string",
								Description: "Text or location in the file to be replaced",
							},
							"content": {
								Type:        "string",
								Description: "New content that replaces the target",
							},
						},
					},
				},
			},
			Required: []string{
				"message",
				"file_path",
				"operation",
			},
		},
	},
}

type FileWriteInput struct {
	Message   string      `json:"message"`
	FilePath  string      `json:"file_path"`
	Operation string      `json:"operation"`
	Content   string      `json:"content,omitempty"`
	Patches   []FilePatch `json:"patches,omitempty"`
}

type FilePatch struct {
	Target  string `json:"target"`
	Content string `json:"content"`
}

type FileWriteOutput struct {
	Success      bool `json:"success"`
	BytesWritten int  `json:"bytes_written"`
}

func RunFileWrite(input FileWriteInput) (FileWriteOutput, error) {

	if input.FilePath == "" {
		return FileWriteOutput{}, errors.New("file path missing")
	}

	switch input.Operation {
	case "create":
		err := os.MkdirAll(filepath.Dir(input.FilePath), 0755)
		if err != nil {
			return FileWriteOutput{}, err
		}

		err = os.WriteFile(input.FilePath, []byte(input.Content), 0644)
		if err != nil {
			return FileWriteOutput{}, err
		}

	case "replace":
		err := os.WriteFile(input.FilePath, []byte(input.Content), 0644)

		if err != nil {
			return FileWriteOutput{}, err
		}

	case "append":
		f, err := os.OpenFile(input.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

		if err != nil {
			return FileWriteOutput{}, err
		}

		defer f.Close()

		n, err := f.WriteString(input.Content)

		if err != nil {
			return FileWriteOutput{}, err
		}

		return FileWriteOutput{
			Success:      true,
			BytesWritten: n,
		}, nil

	case "patch":
		data, err := os.ReadFile(input.FilePath)

		if err != nil {
			return FileWriteOutput{}, err
		}

		text := string(data)

		for _, p := range input.Patches {

			if !strings.Contains(text, p.Target) {
				return FileWriteOutput{}, fmt.Errorf("patch target not found: %s", p.Target)
			}

			text = strings.Replace(text, p.Target, p.Content, 1)

		}

		err = os.WriteFile(input.FilePath, []byte(text), 0644)
		if err != nil {
			return FileWriteOutput{}, err
		}

		return FileWriteOutput{
			Success:      true,
			BytesWritten: len(text),
		}, nil
	default:
		return FileWriteOutput{}, fmt.Errorf(
			"unsupported operation %q (must be one of: create, replace, append, patch)",
			input.Operation,
		)
	}

	return FileWriteOutput{
		Success:      true,
		BytesWritten: len(input.Content),
	}, nil
}

type ParsedDiff struct {
	FileName string
	Hunks    []Hunk
}

type Hunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []DiffLine
}

type DiffLine struct {
	Kind    DiffLineKind
	Content string
}

type DiffLineKind string

const (
	DiffLineUnchanged DiffLineKind = "unchanged"
	DiffLineRemoved   DiffLineKind = "removed"
	DiffLineAdded     DiffLineKind = "added"
)

var hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

func ParseUnifiedDiff(input string) (ParsedDiff, error) {
	var result ParsedDiff
	var currentHunk *Hunk

	scanner := bufio.NewScanner(strings.NewReader(input))

	for scanner.Scan() {
		line := scanner.Text()

		switch {
		case strings.HasPrefix(line, "--- "):
			// old filename, usually can ignore for now

		case strings.HasPrefix(line, "+++ "):
			result.FileName = strings.TrimPrefix(line, "+++ ")

		case strings.HasPrefix(line, "@@ "):
			matches := hunkHeaderRe.FindStringSubmatch(line)
			if matches == nil {
				return ParsedDiff{}, fmt.Errorf("invalid hunk header: %q", line)
			}

			oldStart, _ := strconv.Atoi(matches[1])
			oldCount := 1
			if matches[2] != "" {
				oldCount, _ = strconv.Atoi(matches[2])
			}

			newStart, _ := strconv.Atoi(matches[3])
			newCount := 1
			if matches[4] != "" {
				newCount, _ = strconv.Atoi(matches[4])
			}

			result.Hunks = append(result.Hunks, Hunk{
				OldStart: oldStart,
				OldCount: oldCount,
				NewStart: newStart,
				NewCount: newCount,
				Lines:    []DiffLine{},
			})
			currentHunk = &result.Hunks[len(result.Hunks)-1]

		case strings.HasPrefix(line, "+"):
			if currentHunk == nil {
				return ParsedDiff{}, fmt.Errorf("added line found outside hunk: %q", line)
			}
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Kind:    DiffLineAdded,
				Content: strings.TrimPrefix(line, "+"),
			})

		case strings.HasPrefix(line, "-"):
			if currentHunk == nil {
				return ParsedDiff{}, fmt.Errorf("removed line found outside hunk: %q", line)
			}
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Kind:    DiffLineRemoved,
				Content: strings.TrimPrefix(line, "-"),
			})

		case strings.HasPrefix(line, " "):
			if currentHunk == nil {
				return ParsedDiff{}, fmt.Errorf("context line found outside hunk: %q", line)
			}
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Kind:    DiffLineUnchanged,
				Content: strings.TrimPrefix(line, " "),
			})

		case line == `\ No newline at end of file`:
			// optional metadata line, can ignore or store

		case line == "":
			// in unified diff, an empty context/add/remove line is still prefixed.
			// a truly empty raw line usually can be ignored.

		default:
			return ParsedDiff{}, fmt.Errorf("unrecognized diff line: %q", line)
		}
	}

	if err := scanner.Err(); err != nil {
		return ParsedDiff{}, err
	}

	return result, nil
}

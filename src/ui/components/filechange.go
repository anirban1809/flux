package components

import (
	"flux/src/tools"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	diffAddedBg = lipgloss.NewStyle().
			Background(lipgloss.Color("22")).
			Foreground(lipgloss.Color("10"))
	diffRemovedBg = lipgloss.NewStyle().
			Background(lipgloss.Color("52")).
			Foreground(lipgloss.Color("9"))
	diffUnchanged = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	diffHunkSep   = lipgloss.NewStyle().
			Foreground(lipgloss.Color("69")).
			Italic(true)
	diffFileLabel = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15"))
	diffOpBadge = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)
	diffGutter = lipgloss.NewStyle().
			Foreground(lipgloss.Color("239")).
			Width(4)
	diffAddSign = lipgloss.NewStyle().
			Background(lipgloss.Color("22")).
			Foreground(lipgloss.Color("2")).
			Width(2)
	diffRemSign = lipgloss.NewStyle().
			Background(lipgloss.Color("52")).
			Foreground(lipgloss.Color("1")).
			Width(2)
	diffCtxSign = lipgloss.NewStyle().
			Foreground(lipgloss.Color("239")).
			Width(2)
)

type FileChangeViewer struct {
	FileName     string
	ChangeType   string
	Content      string
	Patches      []tools.ParsedDiff
	Visible      bool
	ScrollOffset int
	MaxVisible   int
}

func CreateFileChangeViewer(
	fileName, changeType, content string,
	patches []tools.ParsedDiff,
) FileChangeViewer {
	return FileChangeViewer{
		FileName:     fileName,
		ChangeType:   changeType,
		Content:      content,
		Patches:      patches,
		Visible:      true,
		ScrollOffset: 0,
		MaxVisible:   20,
	}
}

func (f *FileChangeViewer) ScrollDown() {
	lines := f.getLines()
	max := len(lines) - f.MaxVisible
	if max < 0 {
		max = 0
	}
	if f.ScrollOffset < max {
		f.ScrollOffset++
	}
}

func (f *FileChangeViewer) ScrollUp() {
	if f.ScrollOffset > 0 {
		f.ScrollOffset--
	}
}

func renderDiffLine(kind tools.DiffLineKind, content string) string {
	switch kind {
	case tools.DiffLineAdded:
		return diffAddSign.Render("+") + diffAddedBg.Render(" "+content)
	case tools.DiffLineRemoved:
		return diffRemSign.Render("-") + diffRemovedBg.Render(" "+content)
	default:
		return diffCtxSign.Render(" ") + diffUnchanged.Render(" "+content)
	}
}

func (f FileChangeViewer) getLines() []string {
	var lines []string

	switch f.ChangeType {
	case "patch":
		for _, diff := range f.Patches {
			for _, hunk := range diff.Hunks {
				hunkHeader := fmt.Sprintf(
					"  @@ -%d,%d +%d,%d @@",
					hunk.OldStart,
					hunk.OldCount,
					hunk.NewStart,
					hunk.NewCount,
				)
				lines = append(lines, diffHunkSep.Render(hunkHeader))

				oldLine := hunk.OldStart
				newLine := hunk.NewStart
				for _, line := range hunk.Lines {
					switch line.Kind {
					case tools.DiffLineAdded:
						lines = append(
							lines,
							renderDiffLine(line.Kind, line.Content),
						)
						newLine++
					case tools.DiffLineRemoved:
						lines = append(
							lines,
							renderDiffLine(line.Kind, line.Content),
						)
						oldLine++
					default:
						lines = append(
							lines,
							renderDiffLine(line.Kind, line.Content),
						)
						oldLine++
						newLine++
					}
				}
			}
		}

	case "create", "append", "replace":
		contentLines := strings.Split(f.Content, "\n")
		for _, line := range contentLines {
			lines = append(lines, renderDiffLine(tools.DiffLineAdded, line))
		}
	}

	return lines
}

func (f FileChangeViewer) View() string {
	if !f.Visible {
		return ""
	}

	lines := f.getLines()
	total := len(lines)

	start := f.ScrollOffset
	if start > total {
		start = total
	}
	end := start + f.MaxVisible
	if end > total {
		end = total
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(
		diffFileLabel.Render(
			f.FileName,
		) + "  " + diffOpBadge.Render(
			f.ChangeType,
		) + "\n",
	)
	sb.WriteString(diffUnchanged.Render(strings.Repeat("─", 60)) + "\n")

	for _, line := range lines[start:end] {
		sb.WriteString(line + "\n")
	}

	sb.WriteString(diffUnchanged.Render(strings.Repeat("─", 60)) + "\n")

	if total > f.MaxVisible {
		indicator := fmt.Sprintf(
			"  lines %d-%d of %d  (w/s to scroll)",
			start+1,
			end,
			total,
		)
		sb.WriteString(diffUnchanged.Render(indicator) + "\n")
	}

	return sb.String()
}

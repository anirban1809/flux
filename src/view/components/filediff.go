package view

import (
	"flux/src/events"
	"flux/src/tools"
	"fmt"
	"strconv"
	"strings"

	"github.com/anirban1809/tuix/tuix"
)

const fileDiffWindowSize = 10

type renderedLine struct {
	oldNum   int
	newNum   int
	prefix   string
	content  string
	style    tuix.Style
	isHeader bool
}

func FileDiff(props tuix.Props) tuix.Element {
	fileDiff, ok := props.Get("fileDiff").(events.FileChangeEvent)
	if !ok {
		return tuix.Box(tuix.Props{}, tuix.NewStyle())
	}

	lines := buildRenderedLines(fileDiff)

	viewFloor, setViewFloor := tuix.UseState(0)

	maxFloor := len(lines) - fileDiffWindowSize
	if maxFloor < 0 {
		maxFloor = 0
	}
	if viewFloor > maxFloor {
		viewFloor = maxFloor
		setViewFloor(maxFloor)
	}

	if tuix.CurrentKey.Code == tuix.KeyTab && viewFloor < maxFloor {
		setViewFloor(viewFloor + 1)
	}
	if tuix.CurrentKey.Code == tuix.KeyShiftTab && viewFloor > 0 {
		setViewFloor(viewFloor - 1)
	}

	ceil := min(viewFloor+fileDiffWindowSize, len(lines))
	window := lines[viewFloor:ceil]

	oldW, newW := gutterWidths(lines)

	body := []tuix.Element{
		diffHeader(fileDiff),
		tuix.Text("", tuix.NewStyle()),
	}
	for _, rl := range window {
		body = append(body, renderLine(rl, oldW, newW))
	}

	if len(lines) > fileDiffWindowSize {
		body = append(body, tuix.WrappedText("", tuix.NewStyle()))
		body = append(body, scrollIndicator(viewFloor, ceil, len(lines)))
	}

	return tuix.Box(
		tuix.Props{Direction: tuix.Column, Padding: [4]int{0, 1, 0, 1}},
		tuix.NewStyle(),
		body...,
	)
}

func diffHeader(fd events.FileChangeEvent) tuix.Element {
	op, opColor := opLabel(fd.ChangeType)

	name := fd.FileName
	if name == "" && len(fd.Patches) > 0 {
		name = fd.Patches[0].FileName
	}
	if name == "" {
		name = "(unknown)"
	}

	return tuix.Box(
		tuix.Props{Direction: tuix.Row, Gap: 1},
		tuix.NewStyle(),
		tuix.Text(op, tuix.NewStyle().Foreground(opColor).Bold(true)),
		tuix.Text(name, tuix.NewStyle().Foreground(tuix.Hex("#cbcbcb"))),
	)
}

func opLabel(t events.FileChangeType) (string, tuix.Color) {
	switch t {
	case events.FileChange_Create:
		return "create", tuix.Hex("#67c27a")
	case events.FileChange_Append:
		return "append", tuix.Hex("#64c3ff")
	case events.FileChange_Patch:
		return "patch", tuix.Hex("#e5c07b")
	}
	return "change", tuix.Hex("#cbcbcb")
}

func buildRenderedLines(fd events.FileChangeEvent) []renderedLine {
	addedStyle := tuix.NewStyle().Foreground(tuix.Hex("#67c27a"))
	removedStyle := tuix.NewStyle().Foreground(tuix.Hex("#e06c75"))
	contextStyle := tuix.NewStyle().Foreground(tuix.Hex("#a8a8a8"))
	hunkStyle := tuix.NewStyle().Foreground(tuix.Hex("#56b6c2"))

	switch fd.ChangeType {
	case events.FileChange_Create, events.FileChange_Append:
		if fd.Content == "" {
			return nil
		}
		raw := strings.Split(strings.TrimRight(fd.Content, "\n"), "\n")
		out := make([]renderedLine, 0, len(raw))
		for i, line := range raw {
			out = append(out, renderedLine{
				newNum:  i + 1,
				prefix:  "+",
				content: line,
				style:   addedStyle,
			})
		}
		return out

	case events.FileChange_Patch:
		var out []renderedLine
		for _, p := range fd.Patches {
			for _, h := range p.Hunks {
				out = append(out, renderedLine{
					isHeader: true,
					prefix:   "  ",
					content: fmt.Sprintf(
						"@@ -%d,%d +%d,%d @@",
						h.OldStart,
						h.OldCount,
						h.NewStart,
						h.NewCount,
					),
					style: hunkStyle,
				})
				oldNum := h.OldStart
				newNum := h.NewStart
				for _, dl := range h.Lines {
					switch dl.Kind {
					case tools.DiffLineAdded:
						out = append(out, renderedLine{
							newNum:  newNum,
							prefix:  "+",
							content: dl.Content,
							style:   addedStyle,
						})
						newNum++
					case tools.DiffLineRemoved:
						out = append(out, renderedLine{
							oldNum:  oldNum,
							prefix:  "-",
							content: dl.Content,
							style:   removedStyle,
						})
						oldNum++
					default:
						out = append(out, renderedLine{
							oldNum:  oldNum,
							newNum:  newNum,
							prefix:  " ",
							content: dl.Content,
							style:   contextStyle,
						})
						oldNum++
						newNum++
					}
				}
			}
		}
		return out
	}
	return nil
}

func gutterWidths(lines []renderedLine) (int, int) {
	maxOld, maxNew := 0, 0
	for _, rl := range lines {
		if rl.oldNum > maxOld {
			maxOld = rl.oldNum
		}
		if rl.newNum > maxNew {
			maxNew = rl.newNum
		}
	}
	oldW := 0
	newW := 0
	if maxOld > 0 {
		oldW = len(strconv.Itoa(maxOld))
	}
	if maxNew > 0 {
		newW = len(strconv.Itoa(maxNew))
	}
	return oldW, newW
}

func renderLine(rl renderedLine, oldW, newW int) tuix.Element {
	gutterStyle := tuix.NewStyle().Foreground(tuix.Hex("#5a5a5a"))

	gutter := formatGutter(rl, oldW, newW)
	body := rl.prefix + " " + rl.content
	if rl.isHeader {
		body = rl.content
	}

	return tuix.Box(
		tuix.Props{Direction: tuix.Row},
		tuix.NewStyle(),
		tuix.Text(gutter, gutterStyle),
		tuix.Text(body, rl.style),
	)
}

func formatGutter(rl renderedLine, oldW, newW int) string {
	var b strings.Builder
	if oldW > 0 {
		if rl.oldNum > 0 && !rl.isHeader {
			b.WriteString(fmt.Sprintf("%*d", oldW, rl.oldNum))
		} else {
			b.WriteString(strings.Repeat(" ", oldW))
		}
		b.WriteString(" ")
	}
	if newW > 0 {
		if rl.newNum > 0 && !rl.isHeader {
			b.WriteString(fmt.Sprintf("%*d", newW, rl.newNum))
		} else {
			b.WriteString(strings.Repeat(" ", newW))
		}
		b.WriteString(" │ ")
	}
	return b.String()
}

func scrollIndicator(floor, ceil, total int) tuix.Element {
	style := tuix.NewStyle().Foreground(tuix.Hex("#848484"))
	hint := " (Tab / Shift+Tab to scroll)"
	return tuix.Text(
		fmt.Sprintf("lines %d-%d of %d%s", floor+1, ceil, total, hint),
		style,
	)
}

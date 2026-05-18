package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"
	"zipcode/src/config"
	"zipcode/src/tools"

	"golang.org/x/term"
)

func HumanTime(str string) string {
	t, err := time.Parse(time.RFC3339, str)

	if err != nil {
		return ""
	}

	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "less than a minute ago"
	case d < time.Hour:
		mins := int(d / time.Minute)
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case d < 24*time.Hour:
		hrs := int(d / time.Hour)
		if hrs == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hrs)
	default:
		return t.Format("at 3:04PM on 01/02/2006")
	}
}

func PrintStruct(value any) {
	result, _ := json.Marshal(value)
	fmt.Fprintln(os.Stderr, string(result)+"\n")
}

func If[T any](condition bool, thenValue T, elseValue T) T {
	if condition {
		return thenValue
	} else {
		return elseValue
	}
}

func isStruct(v any) bool {
	// If the value is a pointer, you must dereference it using .Elem()
	// before checking if it's a struct.
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	return val.Kind() == reflect.Struct
}

func GetTerminalSize() (int, int, error) {
	fd := (os.Stdout.Fd())
	width, height, err := term.GetSize(int(fd))
	if err != nil {
		return 0, 0, err
	}

	return width, height, nil
}

func FlexGap(totalWidth int, subWidth int) string {
	gap := totalWidth - subWidth
	gapText := ""

	for range gap {
		gapText += " "
	}

	return gapText
}

func LogValue(value any) {
	if !config.Cfg.Headless {
		return
	}

	if isStruct(value) {
		PrintStruct(value)
		return
	}

	fmt.Println(value)

}

func Log(a ...any) {
	f, err := os.OpenFile(
		"./logs.txt",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0644,
	)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\n", a)
}

func GetTool(path string, toolname string) (tools.Tool, error) {
	name := strings.ReplaceAll(toolname, "_tool", "")
	content, err := os.ReadFile(fmt.Sprintf("%s/%s/%s.json", path, name, name))

	if err != nil {
		return tools.Tool{}, errors.New("failed to read tool manifest")
	}

	var tool tools.Tool
	err = json.Unmarshal([]byte(content), &tool)

	if err != nil {
		return tools.Tool{}, errors.New("invalid tool manifest")
	}

	return tool, nil
}

func Map[T any, U any](ts []T, f func(T, int) U) []U {
	us := make([]U, len(ts))
	for i, v := range ts {
		us[i] = f(v, i)
	}
	return us
}

func Filter[T any](ts []T, f func(T, int) bool) []T {
	us := []T{}
	for i, v := range ts {
		if f(v, i) {
			us = append(us, ts[i])
		}
	}
	return us
}

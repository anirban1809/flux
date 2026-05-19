package tools

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
)

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	DisplayName string     `json:"display_name"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  JSONSchema `json:"parameters"`
}

type JSONSchema struct {
	Type        string            `json:"type,omitempty"`
	Properties  map[string]Schema `json:"properties,omitempty"`
	Required    []string          `json:"required,omitempty"`
	Items       *Schema           `json:"items,omitempty"`
	Description string            `json:"description,omitempty"`
}

type Schema struct {
	Type        string            `json:"type,omitempty"`
	Description string            `json:"description,omitempty"`
	Properties  map[string]Schema `json:"properties,omitempty"`
	Items       *Schema           `json:"items,omitempty"`
	Required    []string          `json:"required,omitempty"`
	Enum        []interface{}     `json:"enum,omitempty"`
}

type grepEvent struct {
	Type string     `json:"type"`
	Data grepOutput `json:"data"`
}

type grepOutput struct {
	Path struct {
		Text string `json:"text"`
	} `json:"path"`

	Lines struct {
		Text string `json:"text"`
	} `json:"lines"`

	LineNumber int `json:"line_number"`
}

func RunBashCommand(command string) (string, error) {
	var cmd *exec.Cmd

	if strings.HasPrefix(command, "bash") || strings.HasPrefix(command, "sh") {
		cmd = exec.Command(command)
	} else {
		cmd = exec.Command("bash", "-c", command)
	}

	cmd.Dir = "."
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	if err != nil {
		return "", errors.New(stderr.String())
	}

	return stdout.String(), nil
}

type grepMatch struct {
	File    string `json:"file"`
	Content string `json:"content"`
}

func ParseGrepMatch(output []byte) ([]grepMatch, error) {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	var matches []grepMatch

	for scanner.Scan() {
		line := scanner.Bytes()
		var event grepEvent
		err := json.Unmarshal(line, &event)

		if err != nil {
			continue
		}

		if event.Type != "match" {
			continue
		}

		match := grepMatch{
			File:    event.Data.Path.Text,
			Content: strings.TrimSpace(event.Data.Lines.Text),
		}

		matches = append(matches, match)
	}

	return matches, nil
}

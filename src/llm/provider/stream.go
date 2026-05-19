package llm

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type sseHandler func(event, data string) error

func readSSE(r io.Reader, handle sseHandler) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var event string
	var data []string
	flush := func() error {
		if len(data) == 0 {
			event = ""
			return nil
		}
		if err := handle(event, strings.Join(data, "\n")); err != nil {
			return err
		}
		event = ""
		data = nil
		return nil
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimPrefix(value, " ")
		switch key {
		case "event":
			event = value
		case "data":
			data = append(data, value)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return flush()
}

type streamingToolCall struct {
	Type      string
	ID        string
	Name      string
	Arguments strings.Builder
}

func mergeToolCallDelta(calls map[int]*streamingToolCall, delta ToolCall) {
	call := calls[delta.Index]
	if call == nil {
		call = &streamingToolCall{}
		calls[delta.Index] = call
	}
	if delta.Type != "" {
		call.Type = delta.Type
	}
	if delta.ID != "" {
		call.ID = delta.ID
	}
	if delta.Function.Name != "" {
		call.Name = delta.Function.Name
	}
	if delta.Function.Arguments != "" {
		call.Arguments.WriteString(delta.Function.Arguments)
	}
}

func finalizeToolCalls(calls map[int]*streamingToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(calls))
	for i := 0; i < len(calls); i++ {
		call, ok := calls[i]
		if !ok {
			continue
		}
		callType := call.Type
		if callType == "" {
			callType = "function"
		}
		out = append(out, ToolCall{
			Type:  callType,
			Index: i,
			ID:    call.ID,
			Function: ToolCallFunction{
				Name:      call.Name,
				Arguments: call.Arguments.String(),
			},
		})
	}
	return out
}

func emitStream(cb func(StreamEvent), ev StreamEvent) {
	if cb != nil {
		cb(ev)
	}
}

func decodeJSONEvent(data string, v any) error {
	if err := json.Unmarshal([]byte(data), v); err != nil {
		return fmt.Errorf("decode stream event: %w", err)
	}
	return nil
}

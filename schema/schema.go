// Package schema provides helpers for building tool input schemas.
package schema

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
)

// Parse unmarshals tool input JSON into the given type.
func Parse[T any](input string) (T, error) {
	var v T
	err := json.Unmarshal([]byte(input), &v)
	return v, err
}

// Props builds an InputSchema from a map of property definitions.
func Props(required []string, properties map[string]any) anthropic.ToolInputSchemaParam {
	s := anthropic.ToolInputSchemaParam{
		Properties: properties,
	}
	if len(required) > 0 {
		s.Required = required
	}
	return s
}

// Str creates a string property definition.
func Str(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

// Integer creates an integer property definition.
func Integer(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

// Bool creates a boolean property definition.
func Bool(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

// Array creates an array property definition.
func Array(desc string, itemType string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": desc,
		"items":       map[string]string{"type": itemType},
	}
}

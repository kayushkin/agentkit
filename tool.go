// Package agentkit provides reusable tools for Claude-based AI agents.
package agentkit

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"
)

// Tool represents an executable capability that can be called by an AI agent.
type Tool struct {
	Name        string
	Description string
	InputSchema anthropic.ToolInputSchemaParam
	Run         func(ctx context.Context, input string) (string, error)
}

# agentkit

**Reusable tools for Claude-based AI agents**

A collection of tool implementations for use with Anthropic's Claude API. Designed to work with any agent framework that follows the standard `agent.Tool` pattern.

## Features

- **Shell execution** — Run bash commands with proper error handling
- **File operations** — Read, write, edit, and list files
- **Schema helpers** — Build tool input schemas easily

## Installation

```bash
go get github.com/kayushkin/agentkit
```

## Usage

```go
import (
    "github.com/anthropics/anthropic-sdk-go"
    "github.com/kayushkin/agentkit/tools"
)

// Create agent and add tools
agent := NewAgent(client, systemPrompt)
agent.AddTool(tools.Shell())
agent.AddTool(tools.ReadFile())
agent.AddTool(tools.WriteFile())
agent.AddTool(tools.EditFile())
agent.AddTool(tools.ListFiles())

// Tools work with standard hooks
agent.SetHooks(&Hooks{
    OnToolCall: func(toolID, name string, input []byte) {
        fmt.Printf("Calling %s\n", name)
    },
})
```

## Available Tools

### `tools.Shell()`
Execute bash commands via `bash -c`. Returns stdout+stderr combined.

### `tools.ReadFile()`
Read file contents with optional offset/limit for large files.

### `tools.WriteFile()`
Create or overwrite files with automatic directory creation.

### `tools.EditFile()`
Surgical text replacement using exact string matching.

### `tools.ListFiles()`
List directory contents with optional recursive mode (respects .gitignore).

## Tool Interface

All tools implement:

```go
type Tool struct {
    Name        string
    Description string
    InputSchema anthropic.ToolInputSchemaParam
    Run         func(ctx context.Context, input string) (string, error)
}
```

## Schema Helpers

Build input schemas easily:

```go
import "github.com/kayushkin/agentkit/schema"

schema.Props(
    []string{"command"}, // required fields
    map[string]any{
        "command": schema.Str("Shell command to execute"),
        "workdir": schema.Str("Working directory (optional)"),
    },
)
```

## License

MIT

package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kayushkin/agentkit"
	"github.com/kayushkin/agentkit/schema"
)

// Shell returns a tool that executes shell commands via bash.
func Shell() agentkit.Tool {
	type input struct {
		Command string `json:"command"`
		Workdir string `json:"workdir"`
	}
	return agentkit.Tool{
		Name:        "shell",
		Description: "Execute a shell command via bash -c. Returns stdout+stderr combined. Use for running programs, git, builds, etc.",
		InputSchema: schema.Props([]string{"command"}, map[string]any{
			"command": schema.Str("Shell command to execute"),
			"workdir": schema.Str("Working directory (optional, defaults to cwd)"),
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			in, err := schema.Parse[input](raw)
			if err != nil {
				return "", err
			}
			cmd := exec.CommandContext(ctx, "bash", "-c", in.Command)
			if in.Workdir != "" {
				cmd.Dir = in.Workdir
			}
			// Ensure common dev tools are on PATH (mise, go, node, etc.)
			cmd.Env = ensureDevToolsOnPath()
			out, err := cmd.CombinedOutput()
			result := string(out)
			if err != nil {
				result = fmt.Sprintf("%s\nexit: %s", result, err)
			}
			if result == "" {
				result = "(no output)"
			}
			
			// Apply smart truncation to keep context manageable
			result = truncateShellOutput(result)
			return result, nil
		},
	}
}

// truncateShellOutput applies intelligent truncation to shell output
func truncateShellOutput(s string) string {
	const (
		maxLines     = 500
		maxChars     = 50000
		headLines    = 250
		tailLines    = 200
		headChars    = 25000
		tailChars    = 20000
	)

	// Check character limit first
	if len(s) > maxChars {
		if len(s) <= headChars+tailChars {
			return s
		}
		head := s[:headChars]
		tail := s[len(s)-tailChars:]
		omitted := len(s) - headChars - tailChars
		return fmt.Sprintf("%s\n\n[... %d characters omitted ...]\n\n%s", head, omitted, tail)
	}

	// Check line limit
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s
	}

	head := strings.Join(lines[:headLines], "\n")
	tail := strings.Join(lines[len(lines)-tailLines:], "\n")
	omitted := len(lines) - headLines - tailLines
	return fmt.Sprintf("%s\n\n[... %d lines omitted ...]\n\n%s", head, omitted, tail)
}

// ensureDevToolsOnPath returns os.Environ() with mise shim paths prepended to PATH.
func ensureDevToolsOnPath() []string {
	env := os.Environ()
	home, _ := os.UserHomeDir()
	if home == "" {
		return env
	}

	// Paths where mise installs tool shims/binaries
	extraPaths := []string{
		filepath.Join(home, ".local", "share", "mise", "shims"),
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, "bin"),
		filepath.Join(home, "go", "bin"),
	}

	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			existing := e[5:]
			env[i] = "PATH=" + strings.Join(extraPaths, ":") + ":" + existing
			return env
		}
	}
	return env
}

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kayushkin/agentkit"
	"github.com/kayushkin/agentkit/schema"
)

// ReadFile returns a tool that reads file contents.
// Supports reading multiple files at once via the "paths" parameter.
func ReadFile() agentkit.Tool {
	type input struct {
		Path   string   `json:"path"`
		Paths  []string `json:"paths"`
		Offset int      `json:"offset"`
		Limit  int      `json:"limit"`
	}
	return agentkit.Tool{
		Name:        "read_file",
		Description: "Read file contents. Use 'path' for a single file, or 'paths' (array) to read multiple files at once — prefer batching reads to save turns. For large files, use offset (1-indexed line) and limit (max lines).",
		InputSchema: schema.Props([]string{}, map[string]any{
			"path":   schema.Str("Path to a single file"),
			"paths":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Multiple file paths to read at once (preferred for batching)"},
			"offset": schema.Integer("Line number to start from (1-indexed, optional, single file only)"),
			"limit":  schema.Integer("Maximum number of lines to return (optional, single file only)"),
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			in, err := schema.Parse[input](raw)
			if err != nil {
				return "", err
			}

			// Collect all paths to read.
			paths := in.Paths
			if in.Path != "" {
				if len(paths) == 0 {
					paths = []string{in.Path}
				} else {
					// Both specified — prepend single path.
					paths = append([]string{in.Path}, paths...)
				}
			}
			if len(paths) == 0 {
				return "error: provide 'path' or 'paths'", nil
			}

			// Single file with offset/limit support.
			if len(paths) == 1 {
				return readSingleFile(paths[0], in.Offset, in.Limit), nil
			}

			// Multiple files — batch read.
			var results []string
			for _, p := range paths {
				content := readSingleFile(p, 0, 0)
				results = append(results, fmt.Sprintf("=== %s ===\n%s", p, content))
			}
			return strings.Join(results, "\n\n"), nil
		},
	}
}

// readSingleFile reads one file with optional offset/limit.
func readSingleFile(path string, offset, limit int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("error: %s", err)
	}
	content := string(data)
	truncatedByUser := false

	if offset > 0 || limit > 0 {
		lines := strings.Split(content, "\n")
		start := 0
		if offset > 0 {
			start = offset - 1
		}
		if start > len(lines) {
			return fmt.Sprintf("offset %d beyond file length (%d lines)", offset, len(lines))
		}
		end := len(lines)
		if limit > 0 && start+limit < end {
			end = start + limit
		}
		content = strings.Join(lines[start:end], "\n")
		truncatedByUser = true
	}

	const maxBytes = 100_000
	if len(content) > maxBytes {
		content = content[:maxBytes] + "\n... (truncated)"
	}

	content = schema.TruncateFileRead(content, truncatedByUser)
	return content
}

// WriteFile returns a tool that creates or overwrites a file.
func WriteFile() agentkit.Tool {
	type input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	return agentkit.Tool{
		Name:        "write_file",
		Description: "Create or overwrite a file with the given content. Creates parent directories automatically.",
		InputSchema: schema.Props([]string{"path", "content"}, map[string]any{
			"path":    schema.Str("Path to the file to write"),
			"content": schema.Str("Content to write to the file"),
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			in, err := schema.Parse[input](raw)
			if err != nil {
				return "", err
			}
			if err := os.MkdirAll(filepath.Dir(in.Path), 0755); err != nil {
				return fmt.Sprintf("error creating directory: %s", err), nil
			}
			if err := os.WriteFile(in.Path, []byte(in.Content), 0644); err != nil {
				return fmt.Sprintf("error writing file: %s", err), nil
			}
			return fmt.Sprintf("wrote %d bytes to %s", len(in.Content), in.Path), nil
		},
	}
}

// EditFile returns a tool that does exact string replacement in a file.
func EditFile() agentkit.Tool {
	type input struct {
		Path    string `json:"path"`
		OldText string `json:"old_text"`
		NewText string `json:"new_text"`
	}
	return agentkit.Tool{
		Name:        "edit_file",
		Description: "Edit a file by replacing an exact text match with new text. The old_text must match exactly (including whitespace). Use for precise, surgical edits.",
		InputSchema: schema.Props([]string{"path", "old_text", "new_text"}, map[string]any{
			"path":     schema.Str("Path to the file to edit"),
			"old_text": schema.Str("Exact text to find and replace"),
			"new_text": schema.Str("New text to replace the old text with"),
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			in, err := schema.Parse[input](raw)
			if err != nil {
				return "", err
			}
			data, err := os.ReadFile(in.Path)
			if err != nil {
				return fmt.Sprintf("error: %s", err), nil
			}
			content := string(data)

			count := strings.Count(content, in.OldText)
			if count == 0 {
				return "error: old_text not found in file", nil
			}
			if count > 1 {
				return fmt.Sprintf("error: old_text matches %d times — must be unique", count), nil
			}

			newContent := strings.Replace(content, in.OldText, in.NewText, 1)
			if err := os.WriteFile(in.Path, []byte(newContent), 0644); err != nil {
				return fmt.Sprintf("error writing file: %s", err), nil
			}
			return fmt.Sprintf("edited %s", in.Path), nil
		},
	}
}

// ListFiles returns a tool that lists directory contents.
func ListFiles() agentkit.Tool {
	type input struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	return agentkit.Tool{
		Name:        "list_files",
		Description: "List files and directories at a path. Use recursive=true for a tree listing (respects .gitignore patterns).",
		InputSchema: schema.Props([]string{"path"}, map[string]any{
			"path":      schema.Str("Directory path to list"),
			"recursive": schema.Bool("List recursively (default: false)"),
		}),
		Run: func(ctx context.Context, raw string) (string, error) {
			in, err := schema.Parse[input](raw)
			if err != nil {
				return "", err
			}
			if in.Path == "" {
				in.Path = "."
			}

			if !in.Recursive {
				entries, err := os.ReadDir(in.Path)
				if err != nil {
					return fmt.Sprintf("error: %s", err), nil
				}
				var lines []string
				for _, e := range entries {
					name := e.Name()
					if e.IsDir() {
						name += "/"
					}
					lines = append(lines, name)
				}
				return schema.TruncateList(lines, 50), nil
			}

			var lines []string
			const maxEntries = 1000
			filepath.WalkDir(in.Path, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if len(lines) >= maxEntries {
					return filepath.SkipAll
				}
				name := d.Name()
				if d.IsDir() && strings.HasPrefix(name, ".") && path != in.Path {
					return filepath.SkipDir
				}
				rel, _ := filepath.Rel(in.Path, path)
				if rel == "." {
					return nil
				}
				if d.IsDir() {
					rel += "/"
				}
				lines = append(lines, rel)
				return nil
			})
			if len(lines) >= maxEntries {
				lines = append(lines, fmt.Sprintf("... (truncated at %d entries)", maxEntries))
			}
			return schema.TruncateList(lines, 50), nil
		},
	}
}

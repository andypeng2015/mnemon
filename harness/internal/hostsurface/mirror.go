package hostsurface

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/projection"
)

func WriteMemoryMirror(path string, proj projection.Projection) error {
	content := strings.TrimSpace(scopedMemoryContent(proj))
	if content == "" {
		content = "# Local Memory\n\n_No scoped memory entries._"
	}
	body := "# MEMORY.md\n\n" +
		"<!-- Non-authoritative mirror generated from Local Mnemon scoped memory. Do not edit directly; use memory-set. -->\n\n" +
		content + "\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Atomic on POSIX: the prime hook (another process) and the background driver may both
	// regenerate this derived view; a reader must never observe a truncated mirror mid-write.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func scopedMemoryContent(proj projection.Projection) string {
	for _, item := range proj.Content {
		if item.Ref.Kind != "memory" {
			continue
		}
		if content, ok := item.Fields["content"].(string); ok {
			return content
		}
	}
	return ""
}

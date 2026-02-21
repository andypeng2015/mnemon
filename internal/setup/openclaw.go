package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mnemon-dev/mnemon/internal/setup/assets"
)

// OpenClawWriteSkill writes the SKILL.md to the OpenClaw skills directory.
func OpenClawWriteSkill(configDir string) (string, error) {
	skillDir := filepath.Join(configDir, "skills", "mnemon")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return "", err
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, assets.OpenClawSkill, 0644); err != nil {
		return "", err
	}
	return skillPath, nil
}

// OpenClawWritePackage writes the static package.json to the plugin directory.
func OpenClawWritePackage(configDir string) (string, error) {
	extensionDir := filepath.Join(configDir, "extensions", "mnemon")
	if err := os.MkdirAll(extensionDir, 0755); err != nil {
		return "", err
	}
	pkgPath := filepath.Join(extensionDir, "package.json")
	if err := os.WriteFile(pkgPath, assets.OpenClawPackageJSON, 0644); err != nil {
		return "", err
	}
	return extensionDir, nil
}

// OpenClawWritePlugin generates and writes index.ts + manifest based on hook selection.
func OpenClawWritePlugin(configDir string, sel HookSelection) (string, error) {
	extensionDir := filepath.Join(configDir, "extensions", "mnemon")
	if err := os.MkdirAll(extensionDir, 0755); err != nil {
		return "", err
	}

	indexPath := filepath.Join(extensionDir, "index.ts")
	if err := os.WriteFile(indexPath, GenerateOpenClawPlugin(sel), 0644); err != nil {
		return "", err
	}

	manifestPath := filepath.Join(extensionDir, "openclaw.plugin.json")
	if err := os.WriteFile(manifestPath, GenerateOpenClawManifest(sel), 0644); err != nil {
		return "", err
	}

	return extensionDir, nil
}

// OpenClawRegister registers the mnemon plugin in openclaw.json under plugins.entries.
// Returns the config path and whether registration succeeded.
func OpenClawRegister(configDir string) (string, bool) {
	configPath := filepath.Join(configDir, "openclaw.json")
	ok := registerOpenClawConfig(configPath)
	return configPath, ok
}

// registerOpenClawConfig attempts to add mnemon to openclaw.json plugins.entries.
// Returns false if the file uses JSON5 or other non-standard format
// that can't be safely round-tripped.
func registerOpenClawConfig(configPath string) bool {
	// Check if file exists and is JSON5 (has comments)
	raw, err := os.ReadFile(configPath)
	if err == nil && hasJSON5Comments(string(raw)) {
		// Can't safely roundtrip JSON5 — skip to avoid destroying comments
		return false
	}

	data, err := ReadJSONFile(configPath)
	if err != nil {
		return false
	}
	AddOpenClawPlugin(data)
	return WriteJSONFile(configPath, data) == nil
}

// hasJSON5Comments checks if a string contains // line comments outside of quoted strings.
func hasJSON5Comments(s string) bool {
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			escaped = false
			continue
		}
		if inString {
			if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		if ch == '/' && i+1 < len(s) && s[i+1] == '/' {
			return true
		}
	}
	return false
}

// OpenClawEject removes mnemon plugin from the given OpenClaw config dir.
func OpenClawEject(configDir string) []error {
	var errs []error

	fmt.Printf("\nRemoving OpenClaw integration (%s)...\n", configDir)

	// Step 1: Remove skill directory
	skillDir := filepath.Join(configDir, "skills", "mnemon")
	if err := os.RemoveAll(skillDir); err != nil {
		StatusError(1, 3, "Skill", err)
		errs = append(errs, err)
	} else {
		StatusOK(1, 3, "Skill", skillDir+" removed")
	}
	removeIfEmpty(filepath.Join(configDir, "skills"))

	// Step 2: Remove plugin directory
	extensionDir := filepath.Join(configDir, "extensions", "mnemon")
	if err := os.RemoveAll(extensionDir); err != nil {
		StatusError(2, 3, "Plugin", err)
		errs = append(errs, err)
	} else {
		StatusOK(2, 3, "Plugin", extensionDir+" removed")
	}
	removeIfEmpty(filepath.Join(configDir, "extensions"))

	// Step 3: Clean openclaw.json (only if standard JSON)
	configPath := filepath.Join(configDir, "openclaw.json")
	raw, _ := os.ReadFile(configPath)
	if len(raw) > 0 && hasJSON5Comments(string(raw)) {
		// JSON5 — can't safely modify, skip
		StatusSkipped(3, 3, "Config", configPath+" (JSON5, manual cleanup may be needed)")
	} else {
		data, err := ReadJSONFile(configPath)
		if err != nil {
			StatusSkipped(3, 3, "Config", configPath+" (not found)")
		} else {
			RemoveOpenClawPlugin(data)
			if err := WriteOrRemoveJSONFile(configPath, data); err != nil {
				StatusError(3, 3, "Config", err)
				errs = append(errs, err)
			} else {
				StatusOK(3, 3, "Config", configPath+" cleaned")
			}
		}
	}

	// Clean up configDir itself if empty
	removeIfEmpty(configDir)

	return errs
}

// displayPath replaces home directory with ~ for cleaner display.
func displayPath(path string) string {
	return strings.Replace(path, HomeDir(), "~", 1)
}

// ─── Plugin Generation ──────────────────────────────────────────────

// GenerateOpenClawPlugin builds index.ts using the real OpenClaw plugin SDK.
// Prime (before_agent_start) is always included; recall, nudge, compact are optional.
func GenerateOpenClawPlugin(sel HookSelection) []byte {
	var b strings.Builder

	// Imports
	b.WriteString("import type { OpenClawPluginDefinition } from \"openclaw/plugin-sdk\";\n")
	b.WriteString("import { emptyPluginConfigSchema } from \"openclaw/plugin-sdk\";\n")
	b.WriteString("import { execSync } from \"child_process\";\n")
	b.WriteString("import { readFileSync } from \"fs\";\n")
	b.WriteString("import { join } from \"path\";\n")
	b.WriteString("import { homedir } from \"os\";\n\n")

	// Helpers
	b.WriteString("const TIMEOUT = 5000;\n")
	b.WriteString("const GUIDE_PATH = join(homedir(), \".mnemon\", \"prompt\", \"guide.md\");\n\n")

	b.WriteString("function run(cmd: string): string {\n")
	b.WriteString("  try {\n")
	b.WriteString("    return execSync(cmd, { timeout: TIMEOUT, encoding: \"utf-8\" }).trim();\n")
	b.WriteString("  } catch {\n")
	b.WriteString("    return \"\";\n")
	b.WriteString("  }\n")
	b.WriteString("}\n")

	b.WriteString("\nfunction readGuide(): string {\n")
	b.WriteString("  try {\n")
	b.WriteString("    return readFileSync(GUIDE_PATH, \"utf-8\").trim();\n")
	b.WriteString("  } catch {\n")
	b.WriteString("    return \"\";\n")
	b.WriteString("  }\n")
	b.WriteString("}\n")

	if sel.Recall {
		b.WriteString("\nfunction escapeShell(s: string): string {\n")
		b.WriteString("  return s.replace(/'/g, \"'\\\\''\");\n")
		b.WriteString("}\n")
	}

	// Plugin definition
	b.WriteString("\nconst plugin: OpenClawPluginDefinition = {\n")
	b.WriteString("  id: \"mnemon\",\n")
	b.WriteString("  name: \"Mnemon\",\n")
	b.WriteString("  description: \"Persistent memory for LLM agents\",\n")
	b.WriteString("  configSchema: emptyPluginConfigSchema(),\n")
	b.WriteString("  register(api) {\n")

	// before_agent_start — guide + optional recall
	b.WriteString("    // Memory guidance + auto-recall before each agent run\n")
	b.WriteString("    api.on(\"before_agent_start\", (event) => {\n")
	b.WriteString("      const parts: string[] = [\"[mnemon] Memory active\"];\n")
	b.WriteString("      const guide = readGuide();\n")
	b.WriteString("      if (guide) parts.push(guide);\n")

	if sel.Recall {
		b.WriteString("      if (event.prompt && event.prompt.length >= 5) {\n")
		b.WriteString("        const recall = run(" + "`mnemon recall '${escapeShell(event.prompt)}' --limit 5`" + ");\n")
		b.WriteString("        if (recall && !/no insights found/i.test(recall)) {\n")
		b.WriteString("          parts.push(" + "`[Past memory] ${recall}`" + ");\n")
		b.WriteString("        }\n")
		b.WriteString("      }\n")
	}

	b.WriteString("      if (parts.length > 0) {\n")
	b.WriteString("        return { systemPrompt: parts.join(\"\\n\\n\") };\n")
	b.WriteString("      }\n")
	b.WriteString("    });\n")

	// agent_end — nudge: remind to save memories at session end
	if sel.Nudge {
		b.WriteString("\n    // Nudge — remind about memory on session end\n")
		b.WriteString("    api.on(\"agent_end\", (event) => {\n")
		b.WriteString("      const count = event.messages?.length ?? 0;\n")
		b.WriteString("      if (count >= 4) {\n")
		b.WriteString("        run(\"mnemon remember 'Session ended with \" + count + \" messages — review for insights' --cat context --imp 1 --source agent\");\n")
		b.WriteString("      }\n")
		b.WriteString("    });\n")
	}

	// before_compaction — compact
	if sel.Compact {
		b.WriteString("\n    // Remind to save key insights before context compaction\n")
		b.WriteString("    api.on(\"before_compaction\", () => {\n")
		b.WriteString("      return { systemPrompt: \"[mnemon] Context compaction starting. Save valuable insights before context is compressed.\" };\n")
		b.WriteString("    });\n")
	}

	b.WriteString("  },\n")
	b.WriteString("};\n\n")
	b.WriteString("export default plugin;\n")

	return []byte(b.String())
}

// GenerateOpenClawManifest builds openclaw.plugin.json in the format OpenClaw expects.
func GenerateOpenClawManifest(_ HookSelection) []byte {
	manifest := `{
  "id": "mnemon",
  "configSchema": {
    "type": "object",
    "additionalProperties": false,
    "properties": {}
  }
}
`
	return []byte(manifest)
}

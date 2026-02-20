package setup

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mnemon-dev/mnemon/internal/setup/assets"
)

// HookSelection describes which optional hooks to install.
// Prime is always installed (mandatory).
type HookSelection struct {
	Recall  bool // UserPromptSubmit — auto-recall memories
	Nudge   bool // Stop — remind about memory on session end
	Compact bool // PreCompact — save insights before context compaction
}

// ClaudeWriteSkill writes the mnemon skill to the config dir.
func ClaudeWriteSkill(configDir string) (string, error) {
	skillDir := filepath.Join(configDir, "skills", "mnemon")
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(skillPath, assets.ClaudeSkill, 0644); err != nil {
		return "", err
	}
	return skillPath, nil
}

// ClaudeWriteHook writes a hook script to the hooks dir.
func ClaudeWriteHook(configDir, filename string, content []byte) (string, error) {
	hooksDir := filepath.Join(configDir, "hooks", "mnemon")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return "", err
	}
	hookPath := filepath.Join(hooksDir, filename)
	if err := os.WriteFile(hookPath, content, 0755); err != nil {
		return "", err
	}
	return hookPath, nil
}

// ClaudeRegisterHooks registers selected hooks in settings.json.
// Prime (SessionStart) is always registered.
func ClaudeRegisterHooks(configDir string, sel HookSelection) (string, error) {
	hooksDir := filepath.Join(configDir, "hooks", "mnemon")
	settingsPath := filepath.Join(configDir, "settings.json")
	data, err := ReadJSONFile(settingsPath)
	if err != nil {
		return "", err
	}
	addClaudeHooksSelective(data, hooksDir, sel)
	if err := WriteJSONFile(settingsPath, data); err != nil {
		return "", err
	}
	return settingsPath, nil
}

// ClaudeEject removes mnemon integration from the given Claude Code config dir.
func ClaudeEject(configDir string) []error {
	var errs []error

	fmt.Printf("\nRemoving Claude Code integration (%s)...\n", configDir)

	// Step 1: Remove hooks directory
	hooksDir := filepath.Join(configDir, "hooks", "mnemon")
	if err := os.RemoveAll(hooksDir); err != nil {
		StatusError(1, 3, "Hooks", err)
		errs = append(errs, err)
	} else {
		StatusOK(1, 3, "Hooks", hooksDir+" removed")
	}
	removeIfEmpty(filepath.Join(configDir, "hooks"))

	// Step 2: Clean settings.json
	settingsPath := filepath.Join(configDir, "settings.json")
	data, err := ReadJSONFile(settingsPath)
	if err != nil {
		StatusError(2, 3, "Settings", err)
		errs = append(errs, err)
	} else {
		RemoveClaudeHooks(data)
		if err := WriteOrRemoveJSONFile(settingsPath, data); err != nil {
			StatusError(2, 3, "Settings", err)
			errs = append(errs, err)
		} else {
			StatusOK(2, 3, "Settings", settingsPath+" cleaned")
		}
	}

	// Step 3: Remove skill directory
	skillDir := filepath.Join(configDir, "skills", "mnemon")
	if err := os.RemoveAll(skillDir); err != nil {
		StatusError(3, 3, "Skill", err)
		errs = append(errs, err)
	} else {
		StatusOK(3, 3, "Skill", skillDir+" removed")
	}
	removeIfEmpty(filepath.Join(configDir, "skills"))

	// Clean up configDir itself if empty
	removeIfEmpty(configDir)

	return errs
}

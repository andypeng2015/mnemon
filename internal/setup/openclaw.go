package setup

import (
	"fmt"
	"os"
	"path/filepath"

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

// OpenClawEject removes mnemon skill from the given OpenClaw config dir.
func OpenClawEject(configDir string) []error {
	var errs []error

	fmt.Printf("\nRemoving OpenClaw integration (%s)...\n", configDir)

	skillDir := filepath.Join(configDir, "skills", "mnemon")
	if err := os.RemoveAll(skillDir); err != nil {
		StatusError(1, 1, "Skill", err)
		errs = append(errs, err)
	} else {
		StatusOK(1, 1, "Skill", skillDir+" removed")
	}
	removeIfEmpty(filepath.Join(configDir, "skills"))
	removeIfEmpty(configDir)

	return errs
}

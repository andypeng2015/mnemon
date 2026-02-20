package cmd

import (
	"fmt"
	"strings"

	"github.com/mnemon-dev/mnemon/internal/config"
	"github.com/mnemon-dev/mnemon/internal/setup"
	"github.com/mnemon-dev/mnemon/internal/setup/assets"
	"github.com/spf13/cobra"
)

var (
	setupTarget string
	setupEject  bool
	setupYes    bool
	setupGlobal bool
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Deploy mnemon into LLM CLI environments",
	Long: `Detect installed LLM CLIs and deploy mnemon integration.

By default, installs to project-local config (.claude/, .openclaw/).
Use --global to install to user-wide config (~/.claude/, ~/.openclaw/).

Supported environments: Claude Code, OpenClaw.

Examples:
  mnemon setup                              # Interactive: project-local install
  mnemon setup --global                     # Interactive: user-wide install
  mnemon setup --target claude-code         # Non-interactive: Claude Code only
  mnemon setup --eject                      # Interactive: remove integrations
  mnemon setup --eject --target claude-code # Non-interactive: remove Claude Code only
  mnemon setup --yes                        # Auto-confirm all prompts`,
	RunE: runSetup,
}

func init() {
	setupCmd.Flags().StringVar(&setupTarget, "target", "", "target environment (claude-code, openclaw)")
	setupCmd.Flags().BoolVar(&setupEject, "eject", false, "remove mnemon integrations")
	setupCmd.Flags().BoolVar(&setupYes, "yes", false, "auto-confirm all prompts (CI-friendly)")
	setupCmd.Flags().BoolVar(&setupGlobal, "global", false, "install to user-wide config (~/.claude/) instead of project-local (.claude/)")
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	if setupTarget != "" && setupTarget != "claude-code" && setupTarget != "openclaw" {
		return fmt.Errorf("invalid target %q (must be claude-code or openclaw)", setupTarget)
	}

	envs := setup.DetectEnvironments(setupGlobal)

	if setupEject {
		return runEjectFlow(envs)
	}
	return runInstallFlow(envs)
}

func runInstallFlow(envs []setup.Environment) error {
	// Non-interactive with --target: install specific target directly
	if setupTarget != "" {
		for i := range envs {
			if envs[i].Name == setupTarget {
				return installEnv(&envs[i])
			}
		}
		return fmt.Errorf("unknown target %q", setupTarget)
	}

	// Detection display
	fmt.Println("Detecting LLM CLI environments...")
	fmt.Println()

	var detected []setup.Environment
	for _, env := range envs {
		setup.DetectionLine(env.Detected, env.Display, env.Version, env.ConfigDir)
		if env.Detected {
			detected = append(detected, env)
		}
	}

	if len(detected) == 0 {
		fmt.Println("\nNo supported LLM CLI environments detected.")
		fmt.Println("Install Claude Code or OpenClaw, then run 'mnemon setup' again.")
		return nil
	}

	// Select environment
	var selected []setup.Environment
	if setupYes {
		selected = detected
	} else if setup.IsInteractive() {
		options := make([]string, len(detected))
		for i, env := range detected {
			options[i] = env.Display
		}
		idx := setup.SelectOne("Select environment", options, 0)
		selected = []setup.Environment{detected[idx]}
	} else {
		selected = detected
	}

	if len(selected) == 0 {
		fmt.Println("\nNo environments selected.")
		return nil
	}

	var errCount int
	for i := range selected {
		if err := installEnv(&selected[i]); err != nil {
			errCount++
		}
	}

	if errCount > 0 {
		return fmt.Errorf("%d error(s) during setup", errCount)
	}
	return nil
}

func installEnv(env *setup.Environment) error {
	switch env.Name {
	case "claude-code":
		return installClaudeCode(env)
	case "openclaw":
		return installOpenClaw(env)
	}
	return nil
}

// ─── Claude Code ────────────────────────────────────────────────────

func installClaudeCode(env *setup.Environment) error {
	configDir := env.ConfigDir

	// Scope selection (only when interactive and --global not explicitly set)
	if !setupGlobal && !setupYes && setup.IsInteractive() {
		home := setup.HomeDir()
		localDir := ".claude"
		globalDir := home + "/.claude"
		idx := setup.SelectOne("Install scope",
			[]string{
				fmt.Sprintf("Local — this project only (%s/)", localDir),
				fmt.Sprintf("Global — all projects (%s/)", globalDir),
			}, 0)
		if idx == 1 {
			configDir = globalDir
		} else {
			configDir = localDir
		}
	}

	fmt.Printf("\nSetting up Claude Code (%s)...\n", configDir)

	// Phase 1: Skill
	fmt.Println("\n[1/3] Skill")
	if path, err := setup.ClaudeWriteSkill(configDir); err != nil {
		setup.StatusError(0, 0, "Skill", err)
		return err
	} else {
		setup.StatusOK(0, 0, "Skill", path)
	}

	// Phase 2: Prime (mandatory)
	fmt.Println("\n[2/3] Prime — session start guidance (required)")
	cfg := configurePrimeConfig("claude-code")
	if err := config.Save(dataDir, cfg); err != nil {
		setup.StatusError(0, 0, "Config", err)
		return err
	}

	// Summary line for config
	secNames := strings.Join(cfg.Prime.Sections, ", ")
	if secNames == "" {
		secNames = "none"
	}
	setup.StatusOK(0, 0, "Config", fmt.Sprintf("%s sections, model=%s", secNames, cfg.Prime.DelegationModel))

	if path, err := setup.ClaudeWriteHook(configDir, "prime.sh", assets.ClaudePrimeHook); err != nil {
		setup.StatusError(0, 0, "Hook: prime", err)
		return err
	} else {
		setup.StatusOK(0, 0, "Hook: prime", path)
	}

	// Phase 3: Optional hooks
	fmt.Println("\n[3/3] Optional hooks")
	sel := selectOptionalHooks()

	if sel.Recall {
		if path, err := setup.ClaudeWriteHook(configDir, "user_prompt.sh", assets.ClaudeUserPromptHook); err != nil {
			setup.StatusError(0, 0, "Hook: recall", err)
		} else {
			setup.StatusOK(0, 0, "Hook: recall", path)
		}
	}
	if sel.Nudge {
		if path, err := setup.ClaudeWriteHook(configDir, "stop.sh", assets.ClaudeStopHook); err != nil {
			setup.StatusError(0, 0, "Hook: nudge", err)
		} else {
			setup.StatusOK(0, 0, "Hook: nudge", path)
		}
	}
	if sel.Compact {
		if path, err := setup.ClaudeWriteHook(configDir, "compact.sh", assets.ClaudeCompactHook); err != nil {
			setup.StatusError(0, 0, "Hook: compact", err)
		} else {
			setup.StatusOK(0, 0, "Hook: compact", path)
		}
	}

	// Register hooks
	if path, err := setup.ClaudeRegisterHooks(configDir, sel); err != nil {
		setup.StatusError(0, 0, "Settings", err)
	} else {
		setup.StatusUpdated(0, 0, "Settings", path)
	}

	// Summary
	var hookNames []string
	hookNames = append(hookNames, "prime")
	if sel.Recall {
		hookNames = append(hookNames, "recall")
	}
	if sel.Nudge {
		hookNames = append(hookNames, "nudge")
	}
	if sel.Compact {
		hookNames = append(hookNames, "compact")
	}
	fmt.Println()
	fmt.Println("Setup complete!")
	fmt.Printf("  Hooks  %s\n", strings.Join(hookNames, ", "))
	fmt.Printf("  Config %s sections, model=%s\n", secNames, cfg.Prime.DelegationModel)
	fmt.Println()
	fmt.Println("Start a new Claude Code session to activate.")
	fmt.Println("Run 'mnemon setup --eject' to remove.")

	return nil
}

// configurePrimeConfig returns a Config based on user choices (default or manual).
// target controls which delegation options are shown (claude-code uses sub-agents, openclaw uses direct CLI).
func configurePrimeConfig(target string) *config.Config {
	cfg := config.DefaultConfig()

	if setupYes || !setup.IsInteractive() {
		return cfg
	}

	mode := setup.SelectOne("Guidance mode",
		[]string{"Default (recommended)", "Manual — configure each section"}, 0)
	if mode == 0 {
		return cfg
	}

	// Manual mode: select sections
	sectionOpts := []string{
		"Recall — auto-recall past memories",
		"Remember — what/when to remember",
		"Delegation — sub-agent memory writes",
	}
	sectionDefs := []bool{true, true, true}
	choices := setup.SelectMulti("Select guidance sections", sectionOpts, sectionDefs)

	var sections []string
	if choices[0] {
		sections = append(sections, "recall")
	}
	if choices[1] {
		sections = append(sections, "remember")
	}
	if choices[2] {
		sections = append(sections, "delegation")
	}
	cfg.Prime.Sections = sections

	// Remember types
	if choices[1] {
		cfg.Prime.RememberText = configureRememberTypes()
	}

	// Delegation model (Claude Code only — OpenClaw uses direct CLI)
	if choices[2] && target == "claude-code" {
		models := []string{"sonnet", "haiku", "opus"}
		idx := setup.SelectOne("Delegation model",
			[]string{"sonnet (recommended)", "haiku (fastest, cheapest)", "opus (most capable)"}, 0)
		cfg.Prime.DelegationModel = models[idx]
	}

	// Custom guidance
	if setup.Confirm("Add custom guidance text?", false) {
		cfg.Prime.Custom = setup.ReadMultiLine("Enter custom guidance (blank line to finish):")
	}

	return cfg
}

// configureRememberTypes lets user select which memory types to include.
// Returns the composed remember section text, or "" to use the default.
func configureRememberTypes() string {
	types := setup.DefaultRememberTypes()

	options := make([]string, len(types)+1)
	defaults := make([]bool, len(types)+1)
	for i, t := range types {
		options[i] = t.Name + " (" + t.Detail + ")"
		defaults[i] = true
	}
	options[len(types)] = "Custom — add your own type"
	defaults[len(types)] = false

	choices := setup.SelectMulti("Select memory types", options, defaults)

	var customType string
	if choices[len(types)] {
		name := setup.ReadLine("Type name: ")
		if name != "" {
			detail := setup.ReadLine("Examples: ")
			if detail != "" {
				customType = fmt.Sprintf("**%s** (%s)", name, detail)
			} else {
				customType = fmt.Sprintf("**%s**", name)
			}
		}
	}

	composed := setup.ComposeRememberSection(types, choices[:len(types)], customType)
	if composed == "" {
		return ""
	}

	// If all defaults selected and no custom type, return "" to use the default
	allDefault := choices[0] && choices[1] && choices[2] && customType == ""
	if allDefault {
		return ""
	}
	return composed
}

// selectOptionalHooks prompts user for which optional hooks to enable.
func selectOptionalHooks() setup.HookSelection {
	sel := setup.HookSelection{Recall: true, Nudge: true, Compact: false}

	if setupYes || !setup.IsInteractive() {
		return sel
	}

	opts := []string{
		"Recall  — auto-recall memories on each message (recommended)",
		"Nudge   — remind about memory on session end",
		"Compact — save key insights before context compaction",
	}
	defs := []bool{true, true, false}
	choices := setup.SelectMulti("Select hooks to enable", opts, defs)

	sel.Recall = choices[0]
	sel.Nudge = choices[1]
	sel.Compact = choices[2]
	return sel
}

// ─── OpenClaw ───────────────────────────────────────────────────────

func installOpenClaw(env *setup.Environment) error {
	// OpenClaw always uses global config (no per-project support)
	configDir := setup.HomeDir() + "/.openclaw"

	fmt.Printf("\nSetting up OpenClaw (%s)...\n", configDir)

	// Phase 1: Skill + Plugin base
	fmt.Println("\n[1/3] Skill + Plugin")
	if path, err := setup.OpenClawWriteSkill(configDir); err != nil {
		setup.StatusError(0, 0, "Skill", err)
		return err
	} else {
		setup.StatusOK(0, 0, "Skill", path)
	}
	if path, err := setup.OpenClawWritePackage(configDir); err != nil {
		setup.StatusError(0, 0, "Plugin", err)
		return err
	} else {
		setup.StatusOK(0, 0, "Plugin", path)
	}

	// Phase 2: Prime (mandatory)
	fmt.Println("\n[2/3] Prime — session start guidance (required)")
	cfg := configurePrimeConfig("openclaw")
	if err := config.Save(dataDir, cfg); err != nil {
		setup.StatusError(0, 0, "Config", err)
		return err
	}

	secNames := strings.Join(cfg.Prime.Sections, ", ")
	if secNames == "" {
		secNames = "none"
	}
	setup.StatusOK(0, 0, "Config", fmt.Sprintf("%s sections", secNames))

	// Phase 3: Optional hooks
	fmt.Println("\n[3/3] Optional hooks")
	sel := selectOptionalHooks()

	// Generate and deploy plugin (index.ts + manifest based on hook selection)
	if path, err := setup.OpenClawWritePlugin(configDir, sel); err != nil {
		setup.StatusError(0, 0, "Plugin", err)
	} else {
		setup.StatusOK(0, 0, "Plugin", path+" (index.ts + manifest)")
	}

	// Register in openclaw.json
	if path, ok := setup.OpenClawRegister(configDir); ok {
		setup.StatusUpdated(0, 0, "Config", path)
	} else {
		setup.StatusSkipped(0, 0, "Config", path+" (manual registration may be needed)")
	}

	// Summary
	var hookNames []string
	hookNames = append(hookNames, "prime")
	if sel.Recall {
		hookNames = append(hookNames, "recall")
	}
	if sel.Nudge {
		hookNames = append(hookNames, "nudge")
	}
	if sel.Compact {
		hookNames = append(hookNames, "compact")
	}
	fmt.Println()
	fmt.Println("Setup complete!")
	fmt.Printf("  Hooks  %s\n", strings.Join(hookNames, ", "))
	fmt.Printf("  Config %s sections\n", secNames)
	fmt.Println()
	fmt.Println("Start a new OpenClaw session to activate.")
	fmt.Println("Run 'mnemon setup --eject' to remove.")

	return nil
}

// ─── Eject ──────────────────────────────────────────────────────────

func runEjectFlow(envs []setup.Environment) error {
	if setupTarget != "" {
		for i := range envs {
			if envs[i].Name == setupTarget {
				return ejectEnv(&envs[i])
			}
		}
		return fmt.Errorf("unknown target %q", setupTarget)
	}

	fmt.Println("Detecting LLM CLI environments...")
	fmt.Println()

	var installed []setup.Environment
	for _, env := range envs {
		setup.DetectionLine(env.Detected, env.Display, env.Version, env.ConfigDir)
		if env.Detected {
			installed = append(installed, env)
		}
	}

	if len(installed) == 0 {
		fmt.Println("\nNo environments detected.")
		return nil
	}

	var selected []setup.Environment
	if setupYes {
		selected = installed
	} else if setup.IsInteractive() {
		options := make([]string, len(installed))
		for i, env := range installed {
			options[i] = env.Display
		}
		idx := setup.SelectOne("Select environment to remove", options, 0)
		selected = []setup.Environment{installed[idx]}
	} else {
		selected = installed
	}

	if len(selected) == 0 {
		fmt.Println("\nNo environments selected.")
		return nil
	}

	var errCount int
	for i := range selected {
		if err := ejectEnv(&selected[i]); err != nil {
			errCount++
		}
	}

	fmt.Println()
	fmt.Println("Done! All selected integrations removed.")

	if errCount > 0 {
		return fmt.Errorf("%d error(s) during eject", errCount)
	}
	return nil
}

func ejectEnv(env *setup.Environment) error {
	switch env.Name {
	case "claude-code":
		errs := setup.ClaudeEject(env.ConfigDir)
		ejectMarkdown("CLAUDE.md", "Remove memory guidance from ./CLAUDE.md?")
		if len(errs) > 0 {
			return errs[0]
		}

	case "openclaw":
		errs := setup.OpenClawEject(env.ConfigDir)
		ejectMarkdown("AGENTS.md", "Remove memory guidance from ./AGENTS.md?")
		if len(errs) > 0 {
			return errs[0]
		}
	}
	return nil
}

func ejectMarkdown(filePath string, prompt string) {
	if setupYes {
		if changed, err := setup.EjectMemoryBlock(filePath); err != nil {
			fmt.Printf("  Warning: could not clean %s: %v\n", filePath, err)
		} else if changed {
			fmt.Printf("  Memory guidance removed from %s\n", filePath)
		}
	} else if setup.IsInteractive() {
		if setup.Confirm(prompt, true) {
			if changed, err := setup.EjectMemoryBlock(filePath); err != nil {
				fmt.Printf("  Warning: could not clean %s: %v\n", filePath, err)
			} else if changed {
				fmt.Printf("  Memory guidance removed from %s\n", filePath)
			}
		}
	}
}

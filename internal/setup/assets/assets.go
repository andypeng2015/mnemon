package assets

import "embed"

//go:embed claude/user_prompt.sh
var ClaudeUserPromptHook []byte

//go:embed claude/stop.sh
var ClaudeStopHook []byte

//go:embed claude/prime.sh
var ClaudePrimeHook []byte

//go:embed claude/compact.sh
var ClaudeCompactHook []byte

//go:embed claude/SKILL.md
var ClaudeSkill []byte

//go:embed claude/guide.md
var ClaudeGuide []byte

//go:embed openclaw/SKILL.md
var OpenClawSkill []byte

// All returns the embedded filesystem for inspection.
//
//go:embed claude openclaw
var All embed.FS

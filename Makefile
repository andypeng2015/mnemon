# ──────────────────────────────────────────────────────────────────────
# Mnemon Makefile
# ──────────────────────────────────────────────────────────────────────

BINARY      := mnemon
GOBIN       := $(shell go env GOBIN)
ifeq ($(GOBIN),)
  GOBIN     := $(shell go env GOPATH)/bin
endif

SKILL_SRC   := skills/mnemon
SKILL_DST   := $(HOME)/.claude/skills/mnemon

HOOKS_SRC   := scripts/hooks
HOOKS_DST   := $(HOME)/.claude/hooks/mnemon
CLAUDE_SETTINGS := $(HOME)/.claude/settings.json

.PHONY: build install uninstall inject eject inject-hooks eject-hooks setup test clean help

.DEFAULT_GOAL := help

# ── Build ────────────────────────────────────────────────────────────

build: ## Build the mnemon binary
	go build -o $(BINARY) .

# ── Install / Uninstall ─────────────────────────────────────────────

install: build ## Build and install mnemon to $GOBIN
	@mkdir -p $(GOBIN)
	cp $(BINARY) $(GOBIN)/$(BINARY)
	@echo "Installed: $(GOBIN)/$(BINARY)"

uninstall: eject eject-hooks ## Remove binary, skill, and hooks
	rm -f $(GOBIN)/$(BINARY)
	@echo "Removed: $(GOBIN)/$(BINARY)"

# ── Skill ────────────────────────────────────────────────────────────

inject: ## Install mnemon skill to ~/.claude/skills/mnemon/
	@mkdir -p $(SKILL_DST)
	cp $(SKILL_SRC)/SKILL.md $(SKILL_DST)/SKILL.md
	@echo "  Skill → $(SKILL_DST)/SKILL.md"

eject: ## Remove mnemon skill
	@if [ -d "$(SKILL_DST)" ]; then \
		rm -rf "$(SKILL_DST)"; \
		echo "Removed: $(SKILL_DST)"; \
	else \
		echo "No mnemon skill found at $(SKILL_DST)"; \
	fi

# ── Hooks (Claude Code only) ────────────────────────────────────────

define JQ_REMOVE_MNEMON
def has_mnemon: ((.command? // "") | test("mnemon")) or ((.prompt? // "") | test("mnemon"));
def no_mnemon: (has_mnemon | not) and ((.hooks? // []) | all(has_mnemon | not));
(.hooks.SessionStart // []) |= map(select(no_mnemon)) |
(.hooks.Stop // []) |= map(select(no_mnemon))
endef
export JQ_REMOVE_MNEMON

inject-hooks: ## Install Claude Code hooks for auto-recall/remember
	@mkdir -p $(HOOKS_DST)
	@cp $(HOOKS_SRC)/session_start.sh $(HOOKS_DST)/session_start.sh
	@cp $(HOOKS_SRC)/stop_prompt.txt $(HOOKS_DST)/stop_prompt.txt
	@chmod +x $(HOOKS_DST)/*.sh
	@if [ ! -f "$(CLAUDE_SETTINGS)" ]; then echo '{}' > "$(CLAUDE_SETTINGS)"; fi
	@jq "$$JQ_REMOVE_MNEMON" "$(CLAUDE_SETTINGS)" | \
	jq --rawfile prompt "$(HOOKS_DST)/stop_prompt.txt" '.hooks //= {} | .hooks.SessionStart += [{"matcher": "startup", "hooks": [{"type": "command", "command": "$(HOOKS_DST)/session_start.sh"}]}] | .hooks.Stop += [{"hooks": [{"type": "prompt", "prompt": $$prompt}]}]' \
		> "$(CLAUDE_SETTINGS).tmp" && mv "$(CLAUDE_SETTINGS).tmp" "$(CLAUDE_SETTINGS)"
	@echo "  Hooks → $(HOOKS_DST)/"
	@echo "  Config → $(CLAUDE_SETTINGS)"

eject-hooks: ## Remove Claude Code hooks
	@if [ -d "$(HOOKS_DST)" ]; then rm -rf "$(HOOKS_DST)"; echo "Removed: $(HOOKS_DST)"; fi
	@if [ -f "$(CLAUDE_SETTINGS)" ]; then \
		jq "$$JQ_REMOVE_MNEMON" "$(CLAUDE_SETTINGS)" > "$(CLAUDE_SETTINGS).tmp" && \
		mv "$(CLAUDE_SETTINGS).tmp" "$(CLAUDE_SETTINGS)"; \
		echo "Cleaned: $(CLAUDE_SETTINGS)"; \
	fi

# ── Setup (one-command) ─────────────────────────────────────────────

setup: install inject inject-hooks ## Full setup: binary + skill + hooks
	@echo ""
	@echo "Setup complete:"
	@echo "  Binary → $(GOBIN)/$(BINARY)"
	@echo "  Skill  → $(SKILL_DST)/SKILL.md"
	@echo "  Hooks  → $(HOOKS_DST)/"
	@echo ""
	@echo "Start a new Claude Code session to verify."

# ── Test ─────────────────────────────────────────────────────────────

test: build ## Run E2E test suite
	bash scripts/e2e_test.sh

# ── Clean ────────────────────────────────────────────────────────────

clean: ## Remove build artifacts and test data
	rm -f $(BINARY)
	rm -rf .testdata

# ── Help ─────────────────────────────────────────────────────────────

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

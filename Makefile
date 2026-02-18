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

.PHONY: build install uninstall inject eject test clean help

.DEFAULT_GOAL := help

# ── Build ────────────────────────────────────────────────────────────

build: ## Build the mnemon binary
	go build -o $(BINARY) .

# ── Install / Uninstall ─────────────────────────────────────────────

install: build ## Build and install mnemon to $GOBIN
	@mkdir -p $(GOBIN)
	cp $(BINARY) $(GOBIN)/$(BINARY)
	@echo "Installed: $(GOBIN)/$(BINARY)"

uninstall: eject ## Remove binary and eject skill
	rm -f $(GOBIN)/$(BINARY)
	@echo "Removed: $(GOBIN)/$(BINARY)"

# ── Skill ────────────────────────────────────────────────────────────

inject: ## Install mnemon skill to ~/.claude/skills/mnemon/
	@mkdir -p $(SKILL_DST)
	cp $(SKILL_SRC)/SKILL.md $(SKILL_DST)/SKILL.md
	@echo "  Skill → $(SKILL_DST)/SKILL.md"

eject: ## Remove mnemon skill from ~/.claude/skills/
	@if [ -d "$(SKILL_DST)" ]; then \
		rm -rf "$(SKILL_DST)"; \
		echo "Removed: $(SKILL_DST)"; \
	else \
		echo "No mnemon skill found at $(SKILL_DST)"; \
	fi

# ── Setup (one-command) ─────────────────────────────────────────────

setup: install inject ## Full setup: binary + skill
	@echo ""
	@echo "Setup complete:"
	@echo "  Binary → $(GOBIN)/$(BINARY)"
	@echo "  Skill  → $(SKILL_DST)/SKILL.md"
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

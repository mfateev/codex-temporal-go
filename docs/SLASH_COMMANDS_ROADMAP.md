# Slash Commands Roadmap: tcx vs Codex

Prioritized plan for closing the slash command gap between tcx and Codex.

**Last updated:** 2026-02-25

---

## Current State

**tcx has 7 commands:** `/exit`, `/quit`, `/end`, `/compact`, `/model`, `/plan`, `/done`
**Codex has ~30 commands** (excluding debug/aliases)

---

## Tier 1 — Easy Wins, High Value (infrastructure exists)

| Command | What it does | Effort | Rationale |
|---------|-------------|--------|-----------|
| `/diff` | Show `git diff` (incl. untracked) | Small | Just shell out to `git diff` + `git diff --cached` + untracked. Extremely useful — users constantly want to see what changed. |
| `/status` | Show session config + token usage | Small | Data already available via `get_turn_status` query. Just format and display. |
| `/ps` | List background exec sessions | Small | `execsession.Store` already tracks sessions. Just list active ones. |
| `/clean` | Stop all background exec sessions | Small | Calls `Store.CloseAll()` or similar. Pairs with `/ps`. |
| `/mcp` | List configured MCP tools | Small | MCP is implemented (PR #26). Just query tool registry and list `mcp__*` tools. |

---

## Tier 2 — Moderate Effort, Solid Value

| Command | What it does | Effort | Rationale |
|---------|-------------|--------|-----------|
| `/review` | Review current changes for issues | Medium | Injects a review prompt + `git diff` as user message. The LLM does the work. Very useful workflow. |
| `/init` | Create AGENTS.md interactively | Medium | Scaffolds a project instruction file. Self-contained, no new infra needed. |
| `/approvals` (`/permissions`) | Change approval mode mid-session | Medium | Approval infrastructure exists. Needs an `UpdateApprovalMode` handler on the workflow. |
| `/new` | Start a fresh chat | Medium | Start a new workflow, switch the CLI's `workflowID`. Needs harness integration. |
| `/resume` | Resume a saved chat (picker) | Medium | `--session` flag exists. This adds an interactive picker mid-session. Session picker already implemented. |
| `/personality` | Set communication style | Small | Just prepend a personality string to system prompt. Needs config plumbing + `UpdatePersonality` handler. |

---

## Tier 3 — Depends on Unimplemented Features

| Command | What it does | Effort | Blocker |
|---------|-------------|--------|---------|
| `/fork` | Branch the conversation | Medium | Needs session fork (start new workflow with partial history). Not hard with Temporal but not built yet. |
| `/rename` | Rename current thread | Small | Needs thread naming support (Temporal search attributes or metadata). |
| `/mention` | Attach a file to context (`@`) | Medium | Needs file fuzzy search UI. More of a TUI feature than just a slash command. |
| `/agent` | Switch active agent thread | Medium | Multi-agent thread switching UI. Subagent infra exists but no CLI thread switching. |
| `/collab` | Change collaboration mode | Small | `/plan` exists but no `/collab` mode switching. Needs role-switch update handler. |

---

## Tier 4 — Low Priority / Niche

| Command | What it does | Effort | Rationale |
|---------|-------------|--------|-----------|
| `/skills` | Manage skill packages | Large | Skills system not started. |
| `/apps` | Manage apps/connectors | Large | Connectors not started. |
| `/experimental` | Toggle experimental features | Small | No feature flag system. |
| `/logout` | Log out | Small | No auth system (API key env var only). |
| `/feedback` | Send logs to maintainers | Medium | Needs log collection + upload endpoint. |
| `/debug-config` | Show config layers | Small | Useful for debugging but niche. |
| `/statusline` | Configure status line items | Small | Low priority UX. |
| `/rollout` | Print rollout file path | Trivial | Temporal replaces rollout files. Could print workflow ID instead. |

---

## Codex Source Reference

- Enum definition: `codex-rs/tui/src/slash_command.rs` (lines 12–56)
- Handler dispatch: `codex-rs/tui/src/chatwidget.rs` (lines 3245+)
- Popup/autocomplete: `codex-rs/tui/src/bottom_pane/slash_commands.rs`

### Codex Command Availability During Task

Commands available while a turn is running: `/diff`, `/rename`, `/mention`, `/skills`, `/status`, `/debug-config`, `/ps`, `/clean`, `/mcp`, `/apps`, `/feedback`, `/quit`, `/exit`, `/collab`, `/agent`, `/statusline`

Commands NOT available during task: `/new`, `/resume`, `/fork`, `/init`, `/compact`, `/model`, `/personality`, `/approvals`, `/permissions`, `/review`, `/plan`, `/logout`, `/experimental`

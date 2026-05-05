# AGENT INSTRUCTIONS

Vendor-independent behavioral rules for any agent working on this project.
The `specs/` directory is the source of truth for all implementation decisions.

---

## Session Start

Before writing any code:

1. Read `specs/AGENT_STATE.md` to find the current phase and task
2. Read `specs/TASKS.md` and the spec files referenced by the current task
3. Report your position to the human:
   - Current phase and task
   - Last completed task
   - Next task
   - Which spec sections you will reference
4. Wait for explicit approval before proceeding

---

## Task Execution

**Before starting a task:**
- Read the spec sections listed for that task in `TASKS.md`
- State which sections you are using
- If the spec does not cover something you need to decide, stop and ask — do not guess

**While executing:**
- Implement exactly what the spec says, nothing more
- Do not add features, abstractions, or patterns not in the specs
- Search the codebase before implementing anything — never reinvent what already exists
- If you find a conflict between spec files, stop and report it

**After completing a task:**
- Run `go build ./...` and `go vet ./...` if any Go file changed
- Run the task-specific check from `TASKS.md` if one is listed
- If any check fails: set `BLOCKED_REASON` in `AGENT_STATE.md`, fix the issue, do not mark the task complete
- If all checks pass: update `AGENT_STATE.md`, report completion, wait for confirmation before starting the next task

---

## Phase Completion

When the last task in a phase is done and all checks pass:

1. Run the phase verification checklist from `TASKS.md`
2. Report results to the human
3. Set `PHASE_STATUS: awaiting_next_phase` in `AGENT_STATE.md`
4. Do not begin the next phase until the human explicitly approves

---

## CLAUDE.md as Living Decision Log

After each phase, update `CLAUDE.md` with:
- Patterns established during implementation (naming, error handling, etc.)
- Any decision made that the spec left open
- What was explicitly decided NOT to do and why

This protects against context loss across sessions.

---

## Hard Rules

- Never start a new phase without explicit human approval
- Never skip a verification checklist item
- Never add code not specified in the specs
- Never silently resolve a spec conflict — always surface it
- Never assume a previous session's work is correct — verify it compiles and tests pass
- Always search before implementing — grep for existing functions before writing new ones

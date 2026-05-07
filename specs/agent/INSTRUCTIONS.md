# AGENT INSTRUCTIONS

The `specs/` directory is the source of truth for all implementation decisions.

---

## Session Start

1. Read `specs/agent/STATE.md`, `specs/agent/DECISIONS.md`, and `specs/agent/CODEINDEX.md`
2. Check if `specs/agent/TASKS.md` exists:
   - If yes: a previous session was interrupted — read it, report the remaining tasks, and ask the human whether to resume or restart
   - If no: proceed normally
3. Read the spec diff:
   - **First session:** treat the full content of all spec files as the diff
   - **Subsequent sessions:** read the diff of `specs/` since the last build commit
3. From the diff, derive a task list and present it for approval:
   - What will be built
   - Dependency order (derived from the architecture spec)
   - Which spec sections each task references
4. **Gate 1 — Task approval:** wait for explicit human approval before continuing
5. Write the approved task list to `specs/agent/TASKS.md` (create if absent, overwrite if present):
   - One task per line as an unchecked markdown checkbox
   - Include the date and spec diff source in the header
6. Present the build plan for the approved tasks:
   - Files and packages to be created
   - Key interfaces, types, and implementation decisions
   - Any ambiguities or open decisions that need resolving before building
7. **Gate 2 — Build plan approval:** wait for explicit human approval before writing any code

---

## Implementation

- Implement exactly what the spec says, nothing more
- Search the codebase before implementing anything — never reinvent what already exists
- If the spec does not cover something you need to decide, stop and ask
- If you find a conflict between spec files, stop and report it

After writing code: run the build and lint commands defined in the project's architecture spec.
After writing code in a component: run its tests.
If any check fails: set `BLOCKED_REASON` in `specs/agent/STATE.md`, fix before continuing.

---

## After Each Task

1. Run the relevant verification checklist from `specs/VERIFICATION.md`
2. If all checks pass:
   - Mark the task complete in `specs/agent/TASKS.md`
   - Commit with a descriptive message (include TASKS.md in the commit)
   - Update `specs/agent/DECISIONS.md` if a pattern emerged or an open decision was made
   - Update `specs/agent/STATE.md`
3. Report completion and move to the next approved task
4. When all tasks in the session are done: delete `specs/agent/TASKS.md` and commit the deletion

---

## Hard Rules

- Never pass Gate 1 or Gate 2 without explicit human approval
- Never skip a verification checklist item
- Never add code not specified in the specs
- Never silently resolve a spec conflict — always surface it
- Never assume a previous session's work is correct — verify it builds and tests pass
- Always search before implementing — grep for existing code before writing new

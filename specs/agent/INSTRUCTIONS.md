# AGENT INSTRUCTIONS

`specs/` is the source of truth for all implementation decisions.

---

## Session Start

1. Read `STATE.md`, `DECISIONS.md`, `CODEINDEX.md` (all under `specs/agent/`)
2. If `specs/agent/tasks/INDEX.md` exists: read it and any in-progress task files, report remaining tasks, ask to resume or restart
3. Read the spec diff (first session: full spec content; subsequent: diff of `specs/` since last build commit)
4. Derive a task list — what to build, dependency order, which specs each task references
5. **Gate 1 — wait for explicit approval** before continuing
6. Write approved tasks to `specs/agent/tasks/`:
   - `INDEX.md`: ordered sequence, dependency map, status checklist, date, diff source
   - One file per task: specs referenced, what to build, tests to write, verification, status
7. Present the build plan: files/packages, key interfaces and types, open decisions
8. **Gate 2 — wait for explicit approval** before writing any code

---

## Implementation

- Implement exactly what the spec says, nothing more
- Grep for existing code before writing anything new
- If the spec doesn't cover a needed decision, stop and ask
- If spec files conflict, stop and report it

**TDD (mandatory for every Go package):** red → green → refactor. Infrastructure files (docker-compose, Dockerfiles, SQL, config) have no tests; verify by running the stack.

**Git:** commit to the working branch after each task passes all checks; include updated task files in every commit.

After writing code: run build and lint. After writing a component: run its tests. On failure: set `BLOCKED_REASON` in `STATE.md` and `Status: blocked` in the task file; fix before continuing.

---

## After Each Task

1. Run the relevant `VERIFICATION.md` checklist
2. On pass: mark complete in `INDEX.md` and task file; commit; update `DECISIONS.md` and `STATE.md`
3. Move to the next approved task
4. When all tasks are done: delete `specs/agent/tasks/` and commit

---

## Hard Rules

- Never pass Gate 1 or Gate 2 without explicit human approval
- Never skip a verification checklist item
- Never add code not in the specs
- Never silently resolve a spec conflict — surface it
- Never assume previous session's work is correct — verify build and tests pass
- Never write implementation code without a failing test first

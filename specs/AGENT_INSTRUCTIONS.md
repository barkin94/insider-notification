# AGENT INSTRUCTIONS

The `specs/` directory is the source of truth for all implementation decisions.

---

## Session Start

1. Read `specs/AGENT_STATE.md` and `specs/DECISIONS.md`
2. Read the domain specs relevant to what comes next (derive from current status)
3. Propose the next logical unit of work:
   - What you will build
   - Which spec sections you are referencing
   - What the completion check will be
4. Wait for explicit approval before writing any code

---

## Implementation

- Implement exactly what the spec says, nothing more
- Search the codebase before implementing anything — never reinvent what already exists
- If the spec does not cover something you need to decide, stop and ask
- If you find a conflict between spec files, stop and report it

After writing any Go file: `go build ./...` and `go vet ./...` must pass.
After writing code in a package: run that package's tests.
If any check fails: set `BLOCKED_REASON` in `specs/AGENT_STATE.md`, fix before continuing.

---

## After Each Unit of Work

1. Commit with a descriptive message
2. Update `specs/DECISIONS.md` if a pattern emerged or an open spec decision was made
3. Update `specs/AGENT_STATE.md`
4. Report what was built and propose the next unit
5. Wait for approval before proceeding

---

## Component Completion

When a component is substantially complete, read `specs/VERIFICATION.md` and run
the checklist for that component. Report results before proposing the next component.

---

## Hard Rules

- Never start implementing without explicit human approval
- Never skip a verification checklist item
- Never add code not specified in the specs
- Never silently resolve a spec conflict — always surface it
- Never assume a previous session's work is correct — verify it compiles and tests pass
- Always search before implementing — grep for existing code before writing new

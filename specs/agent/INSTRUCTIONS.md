# AGENT INSTRUCTIONS

`specs/` is the source of truth for all implementation decisions.

---

## Session Start

1. Read `STATE.md` (pointer file, minimal)
2. Read `STANDARDS.md` (coding rules, cached from previous sessions)
3. Read only the spec file(s) relevant to the current task — not the whole corpus
4. Verify the spec is current: grep the code packages for key symbols mentioned in the spec; if reality diverges, note it in STATE.md divergences section

---

## Implementation: Spec-First Approach

1. Read and understand the task spec (see STATE.md TODO)
2. **Update the spec** if needed to reflect the decision you're about to make
3. Write the code to implement the spec
4. Write tests (TDD: red → green → refactor for Go packages; infra files verified by running the stack)
5. Run `go build ./...`, `go vet ./...`, `go test ./...`
6. Commit both spec and code together in one commit

**Example commit message:**
```
spec: api/repo.notification → api/internal/db: add FindByStatus method

Updates repo.notification.md interface and implements the method with testcontainers test.
```

---

## Verification: What Passes

- Build passes: `go build ./...`, `go vet ./...` succeed
- Tests pass: `go test ./...` (testcontainers tests must run; never skip)
- Lint passes: `golangci-lint run`
- Code matches spec: spec file shows the actual interface, table schema, rules that the code implements
- No type errors or runtime panics

If any fail: fix before committing. Update STATE.md `BLOCKED_REASON` if stuck.

---

## After Each Commit

1. Verify build, tests, and lint passed
2. Update STATE.md: TODO item marked complete, next item started
3. Push to branch (human reviews the diff: spec + code together)

---

## Hard Rules (Never Violate)

1. Never add code that is not in the spec
2. Never skip a verification checklist item
3. Never assume a previous session's work is correct — verify build and tests pass before proceeding
4. Never commit without spec and code together
5. Always read STANDARDS.md at session start (coding rules are binding)

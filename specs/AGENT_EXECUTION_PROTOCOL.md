# AGENT EXECUTION PROTOCOL — Read this file completely before taking any action

## 0. Identity

You are a Go backend engineer implementing a notification system.
Your source of truth for every implementation decision is the /specs directory.
You do not make assumptions. If something is not in the specs, you stop and ask.

---

## 1. First Action on Every Session Start

Before writing a single line of code, you MUST:

1. Read `AGENT_STATE.md` to determine current phase and task position
2. Read the spec files listed under that phase in TASKS.md
3. Report your current state to the human:
   ```
   📍 Current phase: [X]
   ✅ Last completed task: [task ID and name]
   ⏭️  Next task: [task ID and name]
   📄 Specs I will reference: [list]
   Awaiting your go-ahead.
   ```
4. Wait for explicit human approval before proceeding

You never auto-start. You always report state first.

---

## 2. AGENT_STATE.md — The Source of Truth

`AGENT_STATE.md` tracks your progress. You read it at session start and update it
only after a task is **verified** (required checks in §5 pass). Its schema is:

```
CURRENT_PHASE: 1
CURRENT_TASK: 1.3
PHASE_STATUS: in_progress   # in_progress | awaiting_verification | awaiting_next_phase
LAST_COMPLETED_TASK: 1.2
LAST_COMPLETED_TASK_NAME: Write Dockerfile
COMPLETED_TASKS: [1.1, 1.2]
BLOCKED_REASON:             # filled only when you stop and ask
```

Rules:

- Update AGENT_STATE.md only after all required check commands for that task succeed (see §3 and §5). If checks fail, do not mark the task complete; set `BLOCKED_REASON` and fix or ask
- PHASE_STATUS state machine (authoritative):
  - `in_progress`: agent is actively executing tasks in CURRENT_PHASE
  - `awaiting_verification`: agent finished all tasks in CURRENT_PHASE and is running/reporting phase verification checklist
  - `awaiting_next_phase`: agent finished verification reporting and is blocked, waiting for explicit human approval to start the next phase
- Who updates PHASE_STATUS:
  - Agent sets `in_progress` (normal execution)
  - Agent sets `awaiting_verification` (when last task in phase is complete)
  - Agent sets `awaiting_next_phase` (after posting verification results and waiting for human approval)
  - Human grants approval in chat; then agent sets `in_progress` for the next phase
- If AGENT_STATE.md does not exist, create it with CURRENT_PHASE: 1, CURRENT_TASK: 1.1

---

## 3. Task Execution Rules

### Before starting any task

1. Read the spec files referenced in that task's entry in TASKS.md
2. State out loud which spec sections you are using
3. If a task has ambiguities not covered by the specs, stop and ask — do not guess

### While executing a task

- Implement exactly what the spec says, nothing more
- Do not add features, abstractions, or patterns not specified
- Do not refactor code from previous tasks unless the current task explicitly requires it
- If you discover a conflict between specs, stop and report it — do not resolve it silently

### After completing a task

1. Run the relevant check command (see §5) and show the full output
2. If any check fails:
   - Do **not** treat the task as complete
   - Do **not** update `LAST_COMPLETED_TASK`, `COMPLETED_TASKS`, or advance `CURRENT_TASK`
   - Set `BLOCKED_REASON` in `AGENT_STATE.md` with a short summary of the failure
   - Report the failure and stop; fix and re-run checks, or ask the human — do not start the next task
3. If all checks pass:
   - Clear `BLOCKED_REASON` if it was set
   - Update `AGENT_STATE.md` (`LAST_COMPLETED_TASK`, `LAST_COMPLETED_TASK_NAME`, `COMPLETED_TASKS`, `CURRENT_TASK` per `TASKS.md` ordering)
   - If this was the **last task in the phase**, go to §4 (Phase Completion Protocol) instead of starting the next task
4. Report completion (only after step 3):
   ```
   ✅ Task [ID] complete: [name]
   🔍 Check output: [output]
   ⏭️  Next task: [ID and name]
   Ready to proceed — confirm?
   ```
5. Wait for human confirmation before starting the next task

---

## 4. Phase Completion Protocol

When the last task in a phase is complete **and its required checks have passed** (§3):

1. Update AGENT_STATE.md:
   - PHASE_STATUS: awaiting_verification
2. Run the full phase verification checklist (see §6)
3. Report:
   ```
   🏁 Phase [X] complete.

   Verification checklist:
   [paste results of all checks]

   ⚠️  Do not proceed to Phase [X+1] until you confirm verification passed.
   ```
4. Update AGENT_STATE.md:
   - PHASE_STATUS: awaiting_next_phase
5. Stop. Do not begin Phase X+1 until the human explicitly says to proceed.

When the human approves:

- Update AGENT_STATE.md: PHASE_STATUS: awaiting_next_phase → in_progress, CURRENT_PHASE: X+1
- Begin Phase X+1 session start protocol (§1)

---

## 5. Per-Task Check Commands

Run these after you finish implementing the task and **before** you mark the task complete in `AGENT_STATE.md`. A task counts as complete only when these checks succeed. Show full output.

| Task | Check command |
|------|--------------|
| Any .go file created | `go build ./...` |
| Any .go file created | `go vet ./...` |
| 1.3 Docker Compose | `docker-compose config` |
| 1.5 Migrations | `go run ./cmd/migrate up` then `go run ./cmd/migrate status` |
| 1.6 Config | `go build ./internal/config/...` |
| 2.x Repositories | `go test ./internal/db/... -v -run TestUnit` |
| 3.1-3.2 Queue | `go test ./internal/queue/... -v` |
| 3.3 Rate limiter | `go test ./internal/ratelimit/... -v` |
| 4.x Delivery/Retry | `go test ./internal/delivery/... ./internal/retry/... ./internal/service/... -v` |
| 5.x Handlers | `go test ./internal/api/... -v` |
| 6.x WebSocket | `go test ./internal/api/ws/... -v` |
| 8.x Tests | `go test ./... -v` |
| 9.1 Swagger | `swag init && ls docs/` |
| 9.3 CI | `cat .github/workflows/ci.yml` (syntax check only) |

---

## 6. Phase Verification Checklists

### Phase 1 — Scaffold

```
[ ] docker-compose config passes with no errors
[ ] go build ./... passes
[ ] All directories from ARCHITECTURE.md §Project Layout exist
[ ] .env.example file present with all vars from TASKS.md §1.6
[ ] Migration runner compiles and reports status cleanly
```

### Phase 2 — Data Layer

```
[ ] go build ./... passes
[ ] go vet ./... passes
[ ] All repository methods from TASKS.md §2.x are implemented
[ ] All struct fields match DATA_MODEL.md exactly (field names, bson tags, types)
[ ] All MongoDB indexes from DATA_MODEL.md are created in migration Version 1-4
[ ] Unit tests for repositories pass
[ ] Idempotency: client key → Redis fast path works, content hash → DB fallback works
```

### Phase 3 — Queue & Rate Limiter

```
[ ] go build ./... passes
[ ] Queue producer LPUSH to correct priority list
[ ] Worker poll order: high → normal → low confirmed in test
[ ] Rate limiter Lua script executes atomically (test with concurrent goroutines)
[ ] go test ./internal/queue/... ./internal/ratelimit/... passes
```

### Phase 4 — Delivery & Retry

```
[ ] go build ./... passes
[ ] Delivery client sends correct JSON shape to webhook.site
[ ] Delivery client sets X-Correlation-ID header
[ ] Retry: non-retryable codes (400, 401, 403) do NOT retry
[ ] Retry: retryable codes (5xx, timeout) DO retry
[ ] Backoff delay formula matches RETRY_POLICY.md exactly
[ ] Jitter is within [0, delay * 0.2]
[ ] Worker acquires Redis lock before processing
[ ] Worker updates notification status in MongoDB after each transition
[ ] go test ./internal/delivery/... ./internal/retry/... ./internal/service/... passes
```

### Phase 5 — API Handlers

```
[ ] go build ./... passes
[ ] All endpoints from API_CONTRACT.md are registered in router
[ ] POST /notifications returns 201 with correct shape
[ ] POST /notifications with duplicate idempotency key returns 409
[ ] POST /notifications/batch returns 207 with per-item results
[ ] GET /notifications/:id includes delivery_attempts array
[ ] GET /notifications pagination fields match API_CONTRACT.md
[ ] POST /notifications/:id/cancel returns 409 for delivered/failed
[ ] GET /metrics returns all fields from API_CONTRACT.md §GET /metrics
[ ] GET /health returns 200 with mongodb + redis checks
[ ] go test ./internal/api/... passes
```

### Phase 6 — WebSocket

```
[ ] WebSocket connection upgrades successfully
[ ] Current status sent immediately on connect
[ ] Status updates broadcast within 1s of DB write
[ ] Connection closes on terminal status (delivered/failed/cancelled)
[ ] Ping/pong keepalive every 30s confirmed in code
[ ] go test ./internal/api/ws/... passes
```

### Phase 7 — Scheduler & Templates

```
[ ] Scheduler polls every 5s (configurable)
[ ] Scheduled notifications enqueued within 5s of scheduled_at
[ ] Template renderer resolves all {{variables}} correctly
[ ] Missing variable returns error (not silent empty string)
[ ] go test ./internal/scheduler/... ./internal/template/... passes
```

### Phase 8 — Tests

```
[ ] go test ./... passes with zero failures
[ ] Retry exhaustion test: 4 attempts → status = failed
[ ] Non-retryable test: 400 response → status = failed immediately
[ ] End-to-end test: notification created → delivered → status = delivered
[ ] Race detector clean: go test -race ./...
```

### Phase 9 — Docs & CI

```
[ ] docs/ directory generated by swag init
[ ] Swagger UI accessible at /swagger/index.html after docker-compose up
[ ] README.md has: architecture diagram, quick start, API examples, design decisions
[ ] .github/workflows/ci.yml has lint, test, build jobs
[ ] docker-compose up starts cleanly with no errors
[ ] All TASKS.md Done Criteria checked off
```

---

## 7. Interruption Recovery

If the session ends mid-task (no completion report was given):

1. Read AGENT_STATE.md
2. CURRENT_TASK is the task that was interrupted
3. Check if the task's files already exist and compile
4. If they exist, compile, and pass that task’s required checks (§5): mark it complete in `AGENT_STATE.md`, move to next
5. If they are partial or broken: redo only that task from scratch
6. Report what you found and what you intend to do — wait for human confirmation

You never assume a task is complete. You verify by checking files and running build commands.

---

## 8. What You Must Never Do

- Never proceed to the next phase without explicit human approval
- Never skip a verification checklist item and mark it passed
- Never add code not specified in the specs (no "helpful" extras)
- Never silently resolve a spec conflict — always surface it
- Never set PHASE_STATUS to `in_progress` for Phase X+1 without explicit human approval
- Never start coding without reading the relevant spec sections first
- Never assume a previous session's work is correct without verifying it compiles

# CODING STANDARDS

Read this at the start of every session. These rules are binding for all implementations.

---

## IDs

- All IDs are `UUID` type, generated via `uuid.NewV7()` app-side **before** every INSERT
- Never use `gen_random_uuid()` or database defaults
- Field name in Go: always `id`, lowercase

---

## Errors

- Wrap errors with `fmt.Errorf("context: %w", err)` — preserve the call stack
- No sentinel errors (`var ErrNotFound = errors.New(...)`) except at package boundaries (public repo interfaces)
- Never log-and-return-nil; return the error

---

## Constructors & Dependency Injection

- Constructor signature: `NewX(dep1 InterfaceA, dep2 InterfaceB, ...) *X`
- No option structs (`NewX(opts *Options)`); explicit parameters only
- No `init()` functions or package-level globals
- Dependencies always injected as interfaces, never concrete types

---

## Interfaces

- Define interfaces **in the consumer package**, not in the implementation package
- Example: `api/internal/service/` defines `NotificationRepository` (interface); `api/internal/db/` implements it
- One responsibility per interface (`Reader`, `Writer`, `Closer`, not `ReaderWriter`)
- Keep interfaces small (3–5 methods max)

---

## Logging

- Use `log/slog` only; never `fmt.Printf`, `log.Println`, or other loggers
- Access slog via OTel bridge: `go.opentelemetry.io/contrib/bridges/otelslog`
- Every log key must match an event name in `specs/shared/observability.md`
- Every log must include context fields: `id`, `channel`, `status`, `attempt` (where applicable)
- Log levels: `debug` (flow), `info` (events), `warn` (retries/throttles), `error` (failures)

---

## Testing

- **Packages with database access**: Use `testcontainers` to spin up postgres/redis in tests. Never mock.
- **Packages with pure logic**: Use table-driven tests with `stdlib testing` package
- **No mocking internal packages**: If you want to mock something, extract an interface and test the real implementation in integration tests
- Test file location: `*_test.go` in the same package
- Test naming: `func TestX_behavior(t *testing.T)` (behavior-focused, not method names)

---

## Package Naming

- Repository interfaces: `XRepository` (e.g., `NotificationRepository`)
- Repository implementations: `xRepo` or `xRepositoryImpl` (e.g., `notificationRepo`)
- Service interfaces: `XService` (e.g., `NotificationService`)
- Handlers: `func (s *Server) handleX(w http.ResponseWriter, r *http.Request)`
- Packages with one responsibility have one file; only split when a file exceeds ~400 lines

---

## Transactions & Locks

- All PostgreSQL writes use explicit transactions via `pgx/v5` or `bun` transaction APIs
- For idempotent operations: use `SELECT FOR UPDATE` or table-level locks, never application-level mutexes for database concerns
- Redis keys: use namespaced keys (`notify:stream:{priority}`, `ratelimit:{channel}`, `cancelled:{id}`)

---

## HTTP

- All handlers return `(w http.ResponseWriter, r *http.Request)`, no custom signatures
- Error responses use the standard format (defined in API_CONTRACT spec)
- No middleware that modifies request/response after the handler completes
- All handlers accept `context.Context` as the first parameter to inner functions

---

## Hard Rules (Never Violate)

1. Never add code that is not in the spec
2. Never skip a verification checklist item
3. Never assume a previous session's work is correct — verify build and tests pass before proceeding
4. Never write implementation code without a failing test first (TDD for Go packages)
5. Never commit code that doesn't match its spec; update the spec first if needed

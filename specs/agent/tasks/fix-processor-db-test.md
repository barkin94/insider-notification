# fix-processor-db-test

**Status:** pending

## Context

`processor/internal/db/delivery_attempt_repo_test.go` has two tests skipped with:

```
t.Skip("testcontainers migration issue pending investigation")
```

The TestMain in `processor/internal/db/db_test.go` starts a postgres:16-alpine container
and runs migrations via golang-migrate before connecting with `search_path=processor,public`.
Despite the migration file existing at the correct path, the `delivery_attempts` table is
not present when the tests run, causing "relation delivery_attempts does not exist".

The API's equivalent (`api/internal/db/db_test.go`) uses the same pattern and passes.

## What to investigate

- Whether golang-migrate's pgx5 driver applies the migration successfully against the
  testcontainers postgres URL (add logging or a post-migration row count check)
- Whether `search_path=processor,public` passed via `ConnectionString` is actually set
  on the bun connection (vs being silently ignored by pgdriver)
- Whether bun generates `"processor"."delivery_attempts"` or just `"delivery_attempts"`
  for the model tag `bun:"table:processor.delivery_attempts"`

## What to fix

1. Identify root cause of migration not applying or table not being found
2. Remove the two `t.Skip(...)` calls from `delivery_attempt_repo_test.go`
3. Ensure `go test ./internal/db/...` passes green in the processor module
4. Add equivalent tests for `FindDueRetries` while the testcontainer is available

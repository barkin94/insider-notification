# migrations-ci

**Specs:** `system/ARCHITECTURE.md`
**Status:** pending

## What to build

Move database migrations out of the application startup path and into CI/CD.

### Changes

- Remove the `//go:embed migrations` directive and `runMigrations` call from `api/main.go`
- Remove the `embed` and `golang-migrate` imports from `api/main.go`
- Add a GitHub Actions workflow (`.github/workflows/migrate.yml`) that runs migrations
  against the target database on every merge to `main`:
  ```
  - uses: actions/checkout
  - run: go run github.com/golang-migrate/migrate/v4/cmd/migrate \
           -source file://api/migrations \
           -database $DATABASE_URL up
  ```
- Document the migration command in the README

### Rationale

Running migrations inside the application binary couples schema changes to app deploys
and complicates rollbacks. CI/CD is the correct place for migrations: it runs once per
deploy with explicit credentials, independent of the number of app instances starting up.

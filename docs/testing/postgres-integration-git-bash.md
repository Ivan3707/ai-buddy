# Go integration tests with PostgreSQL (Git Bash)

From the repository root, with Docker Desktop, Go 1.26 and Goose available in `PATH`:

```bash
bash scripts/test-backend-postgres.sh
```

The script starts its own `postgres:16-alpine` container with a random localhost port and `tmpfs` data directory. It creates two fresh databases, `ai_buddy_verify_backend` and `ai_buddy_verify_catalog`, applies all Goose migrations, then runs `go test -count=1 -p 1 ./...` from `apps/backend` with `RUN_E2E=1`. The container is stopped on exit. No Compose application volume or `.env` credentials are used. A random temporary password is passed in `TEST_DATABASE_URL` and copied by CLI/app tests to `DB_PASSWORD` for the processes they exercise.

`TEST_DATABASE_URL` and `TEST_CATALOG_DATABASE_URL` are PostgreSQL URLs, not `DB_*` settings. The CLI and app tests open the URL for schema verification, then set `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` and `DB_SSLMODE` for the command under test. The password must come from the URL: setting `DB_PASSWORD` to an empty string causes `database.OpenPostgres` to use its `change_me` default. With password authentication, this produces a PostgreSQL authentication failure. A `trust` test container masks the mistake.

In Git Bash use `export TEST_DATABASE_URL='postgres://user:password@host:port/ai_buddy_verify_name?sslmode=disable'`; in PowerShell use `$env:TEST_DATABASE_URL = '...'`. VS Code Go tests inherit the environment of the VS Code process; `.vscode/settings.json` does not set these variables. Docker Compose's `DB_*` values point at the application database and are not a safe source for test URLs. `TEST_CATALOG_DATABASE_URL` needs a second fresh database because the catalog CLI test expects an empty catalog. The test databases must be at Goose version 7 before running tests. The script handles all of these settings.

`TestOpenPostgresLive` has a separate opt-in flag, `VERIFY_DATABASE_CONNECTION=1`, and uses `DB_*` rather than the test URL. The script leaves it disabled. All tests gated by `TEST_DATABASE_URL`, `TEST_CATALOG_DATABASE_URL` and `RUN_E2E` run.

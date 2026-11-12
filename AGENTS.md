# AI-buddy Backend — Agent Instructions
## Scope and architecture
- Scope: apps/backend only. Inspect neighboring modules only when needed for API compatibility.
- Go backend: net/http, database/sql with pgx, PostgreSQL, Goose migrations, JWT authentication, Argon2id password hashing.
- Layers: HTTP handlers → services (business rules/transactions) → repositories (SQL); follow existing code structure rather than introducing new abstractions.
- Roles: intern, buddy and teamlead. Buddy reviews submissions of assigned interns; teamlead adds personal professional tasks and sees professional progress of assigned interns.
- тех.md contains project context, but actual code and migrations take precedence when describing current behavior.

## Development and verification
- Before editing, inspect relevant files, migrations, tests and git status; agree on significant design or schema changes first.
- From apps/backend: go test -count=1 -p 1 ./..., go vet ./..., git diff --check.
- Format modified Go files with gofmt -w <files>.
- DB integration tests require TEST_DATABASE_URL pointing to an isolated test DB; report skipped tests explicitly.
- Check the repository's actual README, Compose and environment configuration before suggesting run/build/migration commands. Never assume a database target.
- Build check: go build ./.... Run locally only with a verified, explicitly authorized database/configuration.

## Prohibitions
- Do not change code, dependencies, migrations, database schemas or API contracts outside the approved task.
- Do not drop, recreate, truncate, reset or seed databases without explicit approval. Never run migrations against an unverified target.
- Do not edit secrets or .env files, disclose credentials, or log tokens/passwords.
- Do not rewrite applied migrations without approval; propose a new migration instead.
- Do not commit, push, rebase, force-push, delete branches or discard local changes without approval.
- Do not claim success for unrun tests; distinguish passed, failed and skipped checks.
- Avoid unrelated refactoring, unnecessary dependencies and premature infrastructure.
- After approved substantive changes, update the technical handoff (тех.md) and report changes, checks and remaining issues.

## Terminal and Docker
- Execute terminal commands independently for investigation, development, builds, and tests.
- Docker Desktop and the initial Compose environment are started by the developer.
- You may inspect, build, start, stop, and restart AI-buddy containers as needed for the approved task.
- Scope Docker operations to this project's Compose services only.
- Before disruptive operations, check running containers and their Compose project.
- Never remove Docker volumes, databases, images, or unrelated containers.
- Never run `docker compose down -v`, `docker volume rm`, or destructive cleanup commands.
- Database migrations and data modifications require explicit approval.
- Report container restarts, failed commands, and environment issues.

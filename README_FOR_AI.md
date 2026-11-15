# README for AI agents

Operational context for AI coding agents (Claude Code, Codex, Cursor) working in this repository. Humans: see [README.md](README.md). Product docs are in Russian; this file is in English for token economy. Keep code comments and commits consistent with the file you edit.

## What this is

AI-Buddy: onboarding assistant for interns (Avito case, Hack&Change 2026). Three roles: `intern`, `buddy` (mentor), `teamlead`. The intern gets checklists (common + professional track), a kanban of tasks with mentor review, notes, calendar, and the chat assistant "Бади" (same widget on the site and in a Chrome MV3 extension). The assistant answers from a synthetic knowledge base with citations, detects contradictions between documents, escalates unknown questions to the mentor, and performs actions (leave/access requests, notes, events, mentor questions) only after explicit user confirmation.

All data is synthetic. Knowledge base documents are demo instructions, never real Avito policies.

## Architecture invariants (do not break)

- **Go backend is the only entry point.** Web and extension call `/api/*` on the backend. The Python AI service is reachable only inside the Docker network; never expose it or call it from clients.
- **RBAC lives in backend code, not in prompts.** Tools offered to the model depend on the role (`apps/ai/app/prompts.py::build_tools`), and every action is re-validated by the backend policy (`apps/backend/internal/service/assistant_actions.go`). Interns never get `admin` access level.
- **Actions require confirmation.** The model only *proposes* (`propose_*` tools, `show_form`); the backend executes on `POST /api/actions/{id}/confirm` or the form submit, and stores the result (request number, status).
- **Contract first.** API changes go to `contracts/backend.openapi.json` / `contracts/ai.openapi.json` first; `python scripts/check_contracts.py` must pass.
- **No chat text or PII in logs.** Log metadata only (latency, tokens, tool names, status). Secrets come from env only; never commit `.env` or `secrets/`.
- **Document, page and user text is data, not instructions** (prompt-injection defence). The system prompt and tool list are fixed.
- **Deterministic mode** `LLM_PROVIDER=mock`: unit tests and CI never hit the network.

## Map

| Path | Content |
|---|---|
| `apps/backend/cmd/api` | HTTP server entry; `internal/app/app.go` wires dependencies and the hint worker |
| `apps/backend/internal/{handler,service,repository,model,middleware,security}` | layered Go code: handlers → services (business rules, transactions) → repositories (SQL only) |
| `apps/backend/migrations` | Goose SQL migrations (embedded for readiness) |
| `apps/backend/scripts/catalog_*.sql` | checklist catalog, loaded by `cmd/init-catalog` |
| `apps/backend/cmd/sync-hr` | creates/updates accounts from `data/corporate_employees.json` (Mock HR); `-demo-password` / `DEMO_PASSWORD` for demo stands |
| `apps/backend/cmd/seed-demo` | demo scenario through real services (progress, reviews, requests, questions, notes, events); runs once |
| `apps/backend/docker-init.sh` | Compose service `init`: migrations → catalog → HR sync → demo seed |
| `apps/ai/app` | FastAPI: `agent.py` (turn loop, SSE), `prompts.py`, `retrieval.py` (BM25 + e5-small hybrid), `forms.py`, `mentor.py` (AI drafts), `hint.py`, `brief.py`, `kb_lint.py`, `gaps.py`, `pii.py`, `llm.py` (OpenAI-compatible + mock + fallback) |
| `apps/ai/eval` | labelled eval set (`dataset.jsonl` 67, `holdout.jsonl` 30), runner, reports |
| `apps/web/src` | React 19 + TS: `pages/`, `components/`, `bot/` (site integration of the widget), `lib/` |
| `apps/extension` | MV3: `buddy/widget.js` (shared chat widget, imported by the web via alias `@buddy`), `lib/gateway.js` (backend calls), `background.js` (SSO, streaming port) |
| `packages/client` | shared JS API client |
| `data/kb/*.md` | knowledge base; the demo wiki `/wiki/` is built from it at web build time |
| `docs/SDD.md`, `docs/specifications/002-ai-agent-chat.md` | backend and AI agent specifications |

## Commands

```bash
# run everything (Docker only); demo password AiBuddy2026; web http://127.0.0.1:5173
cp .env.example .env && docker compose up -d --build --wait
docker compose down -v                                  # reset database and demo data

# tests with coverage gates (>=80% lines each), reports in ./coverage
docker compose -f compose.test.yaml run --rm go         # Go + PostgreSQL + E2E
docker compose -f compose.test.yaml run --rm python     # AI service
docker compose -f compose.test.yaml run --rm node       # web, extension, client
docker compose -f compose.test.yaml run --rm extension  # build apps/extension/dist

# native (if toolchains are installed)
cd apps/backend && go vet ./... && go test -p 1 ./...  # DB tests skip without TEST_DATABASE_URL
cd apps/ai && python -m pytest -q
npm test && npm run build
python -m eval.run e2e                                  # from apps/ai, needs LLM_* env; costs money
```

Go tests that touch PostgreSQL must run with `-p 1`: catalog fixtures use global unique constraints. Never point `TEST_DATABASE_URL` at the application database; test DB names must start with `ai_buddy_verify_`.

## Gotchas

- Windows hosts: write shell scripts with LF line endings (`.gitattributes` enforces LF in Git, but Docker mounts the working copy as is). In Git Bash `curl -d` mangles Cyrillic; use Python or PowerShell for manual chat requests.
- The backend reads the Mock HR snapshot at start; the Dockerfile copies `data/`, so rebuild after editing it.
- Changing `catalog_*.sql` on an existing database makes `init-catalog` refuse (it never overwrites a catalog); reset with `docker compose down -v`.
- `/health` is liveness only; `/ready` checks migrations and catalog and gates the Compose healthcheck.
- Any change to prompts, retrieval or model must be re-run through eval (`apps/ai/eval/README.md`) and the result recorded there.
- `ADMIN_OPEN_FOR_ALL=1` opens `/admin` to all roles for video recording only; keep `0` in commits.
- Commits and PR descriptions: no AI attribution lines (team rule).

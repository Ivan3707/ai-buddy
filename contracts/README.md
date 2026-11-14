# API contracts

- [`backend.openapi.json`](backend.openapi.json) — HTTP API Go backend for Web and other clients.
- [`ai.openapi.json`](ai.openapi.json) — HTTP contract used by the backend for AI `/chat` and `/hint`; AI implementation is maintained separately.
- [`../docs/SDD.md`](../docs/SDD.md) — current backend design and business rules.

`GET /health`, `POST /api/auth/login`, and `POST /api/auth/refresh` are public. Every other backend `/api/*` route requires an access Bearer JWT. Role and ownership checks further restrict individual operations; see the SDD and handlers. Logout also requires the refresh token in the JSON body.

The client-facing `POST /api/chat` and internal AI `POST /chat` share the `message` request and `answer`/`sources`/`mode` response contract. The backend rejects malformed, blank, or overlong messages with `400` and returns `502` for an unavailable or invalid AI response. The hint worker calls AI `POST /hint` asynchronously; AI errors are retried or handled by a local fallback. `GET /health` is process liveness, not proof of database or AI availability.

Backend error responses use JSON `{"error":"..."}` with status codes appropriate to each route (`400`, `401`, `403`, `404`, `409`, `500`, `502`, or `503`). AI validation and response codes are defined separately in its contract. Update contracts alongside any incompatible API change.

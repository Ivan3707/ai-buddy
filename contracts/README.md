# API contracts
OpenAPI 3.1 JSON: backend.openapi.json (public API), ai.openapi.json (internal AI).
The chat request/response schema is shared; change contracts before implementing incompatible changes.
GET /health is liveness, not proof of AI availability. POST /api/chat returns 502 when AI fails.
Backend errors are 400/502; FastAPI input validation is 422. No auth in the local demo.

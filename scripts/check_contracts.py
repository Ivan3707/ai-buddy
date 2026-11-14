"""Validate OpenAPI and actual HTTP payloads from the Compose stack."""
import json
import os
from pathlib import Path
from urllib.request import Request, urlopen
from urllib.error import HTTPError
from jsonschema import Draft202012Validator
from openapi_spec_validator import validate_spec

root = Path(__file__).resolve().parents[1]
specs = {s:json.loads((root / f"contracts/{s}.openapi.json").read_text()) for s in ("backend", "ai")}
for spec in specs.values():
    validate_spec(spec)

backend, ai = specs["backend"], specs["ai"]
backend_schemas = backend["components"]["schemas"]
ai_schemas = ai["components"]["schemas"]
required_shared = {"ChatRequest", "ChatResponse"}
if not required_shared <= backend_schemas.keys() or not required_shared <= ai_schemas.keys():
    raise ValueError("ChatRequest and ChatResponse must exist in both contracts")
shared_names = backend_schemas.keys() & ai_schemas.keys()
for name in sorted(shared_names):
    if backend_schemas[name] != ai_schemas[name]:
        raise ValueError(f"shared schema differs: {name}")

backend_chat = backend["paths"]["/api/chat"]["post"]
ai_chat = ai["paths"]["/chat"]["post"]
for label, operation in (("backend /api/chat", backend_chat), ("AI /chat", ai_chat)):
    if not operation["requestBody"]["required"]:
        raise ValueError(f"{label} must require a request body")
for location in (("requestBody", "content", "application/json", "schema"),
                 ("responses", "200", "content", "application/json", "schema")):
    left, right = backend_chat, ai_chat
    for key in location:
        left, right = left[key], right[key]
    if left != right:
        raise ValueError(f"chat contract differs at {'/'.join(location)}")

# The worker sends these four fields to AI /hint and consumes a nonempty hint.
# /hint is an internal AI operation, so it has no matching backend API path.
ai_hint = ai["paths"]["/hint"]["post"]
hint_request = ai_hint["requestBody"]["content"]["application/json"]["schema"]
hint_fields = {"title": "string", "description": "string",
               "expected_result": "string", "estimated_minutes": "integer"}
if any(hint_request["properties"].get(name, {}).get("type") != kind
       for name, kind in hint_fields.items()):
    raise ValueError("AI /hint request fields differ from the backend worker payload")
Draft202012Validator(hint_request).validate({
    "title": "Example task", "description": "", "expected_result": "",
    "estimated_minutes": 1,
})
hint_response = ai_hint["responses"]["200"]["content"]["application/json"]["schema"]
if "hint" not in hint_response.get("required", []) or hint_response.get("properties", {}).get("hint", {}).get("type") != "string":
    raise ValueError("AI /hint must return a required string hint")
Draft202012Validator(hint_response).validate({"hint": "Example hint"})

print(f"OpenAPI documents valid; {', '.join(sorted(shared_names))} and chat/hint integrations agree")

if os.getenv("CHECK_LIVE") == "1":
    base = os.getenv("BACKEND_URL", "http://127.0.0.1:8080")

    credentials_file = os.getenv("E2E_CREDENTIALS_FILE")
    if not credentials_file:
        raise RuntimeError("E2E_CREDENTIALS_FILE is required for live contract checks")
    try:
        credentials = json.loads(Path(credentials_file).read_text(encoding="utf-8"))
    except OSError as error:
        raise RuntimeError("E2E_CREDENTIALS_FILE cannot be read") from error
    except json.JSONDecodeError as error:
        raise RuntimeError("E2E_CREDENTIALS_FILE is not valid JSON") from error

    if not isinstance(credentials, list):
        raise RuntimeError("E2E_CREDENTIALS_FILE must contain an array of accounts")
    email = "buddy@demo.example"
    account = next(
        (item for item in credentials if isinstance(item, dict) and item.get("email") == email),
        None,
    )
    if account is None or not isinstance(account.get("password"), str) or not account["password"]:
        raise RuntimeError("E2E_CREDENTIALS_FILE has no usable credential for buddy@demo.example")

    def request(path, method, body=None, token=None):
        headers = {"Content-Type": "application/json"}
        if token:
            headers["Authorization"] = f"Bearer {token}"

        req = Request(
            base + path,
            data=json.dumps(body).encode() if body is not None else None,
            method=method,
            headers=headers,
        )

        try:
            reply = urlopen(req, timeout=20)
        except HTTPError as error:
            reply = error

        with reply:
            return reply.status, json.load(reply)

    def check(path, method, body, expected_status, token=None):
        status, data = request(path, method, body, token)
        assert status == expected_status, (path, status, data)

        schema = specs["backend"]["paths"][path.split("?")[0]][
            method.lower()
        ]["responses"][str(expected_status)]["content"][
            "application/json"
        ]["schema"]

        Draft202012Validator({
            **schema,
            "components": specs["backend"]["components"],
        }).validate(data)

        return data

    check("/health", "GET", None, 200)

    # Chat must reject unauthenticated requests.
    status, _ = request(
        "/api/chat",
        "POST",
        {"message": "Как оформить отпуск?"},
    )
    assert status == 401, ("unauthenticated chat", status)

    # Authenticate using the Mock HR Buddy account.
    status, login = request(
        "/api/auth/login",
        "POST",
        {"email": email, "password": account["password"]},
    )
    assert status == 200, ("login", status)
    assert login["token_type"] == "Bearer"
    token = login["access_token"]
    assert token, "login returned an empty access token"

    result = check(
        "/api/chat",
        "POST",
        {"message": "Как оформить отпуск?"},
        200,
        token,
    )
    assert result["sources"][0]["id"] == "leave"

    check("/api/chat", "POST", {"message": ""}, 400, token)
    check("/api/chat", "POST", {"message": "xyz"}, 200, token)

    print("Live backend responses conform to contracts")


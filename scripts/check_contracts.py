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
assert specs["backend"]["components"] == specs["ai"]["components"]
print("OpenAPI documents valid; shared chat schemas agree")

if os.getenv("CHECK_LIVE") == "1":
    base = os.getenv("BACKEND_URL", "http://127.0.0.1:8080")

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

    # Authenticate using the CI buddy account.
    email = os.environ["CI_BUDDY_EMAIL"]
    password = os.environ["CI_BUDDY_PASSWORD"]

    status, login = request(
        "/api/auth/login",
        "POST",
        {"email": email, "password": password},
    )
    assert status == 200, ("login", status, login)
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

    check("/api/onboarding?role=ml", "GET", None, 200)
    check("/api/onboarding?role=invalid", "GET", None, 400)

    print("Live backend responses conform to contracts")


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
    def check(path, method, body, status):
        req = Request(base + path, data=json.dumps(body).encode() if body is not None else None, method=method, headers={"Content-Type":"application/json"})
        try:
            reply = urlopen(req, timeout=20)
        except HTTPError as error:
            reply = error
        assert reply.status == status, (path, reply.status)
        data = json.load(reply)
        schema = specs["backend"]["paths"][path.split("?")[0]][method.lower()]["responses"][str(status)]["content"]["application/json"]["schema"]
        Draft202012Validator({**schema, "components":specs["backend"]["components"]}).validate(data)
        return data
    check("/health", "GET", None, 200)
    result = check("/api/chat", "POST", {"message":"Как оформить отпуск?"}, 200)
    assert result["sources"][0]["id"] == "leave"
    check("/api/chat", "POST", {"message":""}, 400)
    check("/api/chat", "POST", {"message":"xyz"}, 200)
    check("/api/onboarding?role=ml", "GET", None, 200)
    check("/api/onboarding?role=invalid", "GET", None, 400)
    print("Live backend responses conform to contracts")

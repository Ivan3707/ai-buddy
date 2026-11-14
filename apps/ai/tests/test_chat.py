from fastapi.testclient import TestClient
from app.main import app
import json
from pathlib import Path

client = TestClient(app)

def test_retrieval_and_no_match():
    result = client.post("/chat", json={"message": "Как оформить отпуск?"})
    assert result.status_code == 200
    assert result.json()["sources"][0]["id"] == "leave"
    assert result.json()["mode"] == "mock"
    assert client.post("/chat", json={"message": "xyz"}).json()["sources"] == []

def test_validation():
    for message in ("", "   ", "a" * 2001):
        assert client.post("/chat", json={"message": message}).status_code == 422
    assert client.get("/health").json()["status"] == "ok"

def test_hint_endpoint_matches_backend_contract():
    contract = json.loads((Path(__file__).resolve().parents[3] / "contracts/ai.openapi.json").read_text(encoding="utf-8"))
    assert "/hint" in contract["paths"]
    assert "/hint" in app.openapi()["paths"]
    payload = {"title": "Оформить доступ", "description": "", "expected_result": "", "estimated_minutes": 30}
    response = client.post("/hint", json=payload)
    assert response.status_code == 200
    assert response.json()["hint"]
    assert client.post("/hint", json={**payload, "estimated_minutes": 0}).status_code == 422
    assert client.post("/chat", json={"message": "access"}).status_code == 200

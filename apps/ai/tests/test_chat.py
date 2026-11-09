from fastapi.testclient import TestClient
from app.main import app

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

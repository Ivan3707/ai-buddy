"""Prometheus metrics (C2): counters move, endpoint serves text format, no message text leaks."""
from fastapi.testclient import TestClient

from app import metrics
from app.config import Settings
from app.main import create_app
from conftest import user_context


def value(name, **labels):
    return metrics.REGISTRY.get_sample_value(name, labels) or 0.0


def test_turn_metrics_and_endpoint():
    client = TestClient(create_app(Settings(rag_mode="bm25")))
    before_answered = value("ai_turns_total", finish_reason="answered")
    before_pii = value("ai_pii_masked_total", kind="phone")
    body = {"message": "Как оформить отпуск? мой тел 89123456789", "user_context": user_context()}
    client.post("/v1/chat/stream", json=body)
    client.post("/v1/chat/stream", json={"message": "Хочу оформить отгул", "user_context": user_context()})
    assert value("ai_turns_total", finish_reason="answered") == before_answered + 1
    assert value("ai_pii_masked_total", kind="phone") == before_pii + 1
    assert value("ai_action_proposals_total", type="leave_request") >= 1
    assert value("ai_turn_latency_seconds_count") >= 2 and value("ai_time_to_first_token_seconds_count") >= 1
    resp = client.get("/metrics")
    assert resp.status_code == 200 and resp.headers["content-type"].startswith("text/plain")
    assert "ai_turns_total" in resp.text and "89123456789" not in resp.text and "отпуск" not in resp.text

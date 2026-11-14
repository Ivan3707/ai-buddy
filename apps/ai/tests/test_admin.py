import asyncio
import json
from pathlib import Path

import httpx
from fastapi.testclient import TestClient

from app.admin import fetch_balance, run_ping
from app.config import Settings
from app.limits import Limiter
from app.main import create_app
from app.retrieval import BM25Retriever, load_markdown_kb

KB = Path(__file__).resolve().parents[3] / "data" / "kb"
OR = Settings(provider="openai_compatible", base_url="https://openrouter.ai/api/v1", api_key="sk-or-secret", model="m/x")


def run(coro):
    return asyncio.run(coro)


def test_openrouter_balance_normalized_and_key_not_leaked():
    seen = {}

    def handler(request: httpx.Request):
        seen["url"], seen["auth"] = str(request.url), request.headers["authorization"]
        return httpx.Response(200, json={"data": {"label": "sk-or-v1-abcdef123", "usage": 1.5, "limit": 10,
                                                  "limit_remaining": 8.5, "usage_daily": 0.1, "usage_weekly": 0.7,
                                                  "usage_monthly": 1.5, "is_free_tier": False}})
    out = run(fetch_balance(OR, httpx.MockTransport(handler)))
    assert seen == {"url": "https://openrouter.ai/api/v1/key", "auth": "Bearer sk-or-secret"}
    assert out["available"] and out["limit"] == 10 and out["limit_remaining"] == 8.5 and out["usage"] == 1.5
    assert "abcdef123" not in json.dumps(out) and "secret" not in json.dumps(out)


def test_balance_null_limit_and_failures_do_not_raise():
    ok = run(fetch_balance(OR, httpx.MockTransport(lambda r: httpx.Response(200, json={"data": {"usage": 2}}))))
    assert ok["available"] and ok["limit"] is None and ok["note"]
    bad = run(fetch_balance(OR, httpx.MockTransport(lambda r: httpx.Response(401, text="sk-or-secret leaked"))))
    assert not bad["available"] and "secret" not in json.dumps(bad) and "401" in bad["note"]

    def boom(request):
        raise httpx.ConnectError("secret host")
    assert not run(fetch_balance(OR, httpx.MockTransport(boom)))["available"]
    assert not run(fetch_balance(OR, httpx.MockTransport(lambda r: httpx.Response(200, text="not json"))))["available"]


def test_balance_not_exposed_by_other_providers_and_mock():
    other = Settings(provider="openai_compatible", base_url="https://api.example.com/v1", api_key="k", model="m")
    assert "кабинет" in run(fetch_balance(other))["note"]
    assert not run(fetch_balance(Settings()))["available"]


def test_ping_uses_model_settings_and_reports_usage():
    seen = {}

    def handler(request: httpx.Request):
        seen["body"] = json.loads(request.content)
        seen["url"] = str(request.url)
        return httpx.Response(200, json={"choices": [{"message": {"content": " pong "}}],
                                         "usage": {"prompt_tokens": 12, "completion_tokens": 2}})
    s = Settings(provider="openai_compatible", base_url="https://llm.test/v1", api_key="k", model="m",
                 extra_body={"reasoning": {"effort": "low"}})
    out = run(run_ping(s, httpx.MockTransport(handler)))
    assert seen["url"] == "https://llm.test/v1/chat/completions"
    assert seen["body"]["max_tokens"] == 16 and seen["body"]["reasoning"] == {"effort": "low"}
    assert out["ok"] and out["answer"] == "pong" and out["input_tokens"] == 12 and out["output_tokens"] == 2


def test_ping_errors_are_reported_without_body():
    s = Settings(provider="openai_compatible", base_url="https://llm.test/v1", api_key="k", model="m")
    out = run(run_ping(s, httpx.MockTransport(lambda r: httpx.Response(500, text="secret"))))
    assert not out["ok"] and "500" in out["error"] and "secret" not in json.dumps(out)

    def slow(request):
        raise httpx.ReadTimeout("t")
    assert "Таймаут" in run(run_ping(s, httpx.MockTransport(slow)))["error"]
    assert not run(run_ping(s, httpx.MockTransport(lambda r: httpx.Response(200, text="x"))))["ok"]


def test_endpoints_in_mock_mode(monkeypatch):
    monkeypatch.setenv("LLM_PROVIDER", "mock")
    app = create_app(retriever=BM25Retriever(load_markdown_kb(KB)))
    client = TestClient(app)
    body = client.get("/v1/admin/llm").json()
    assert body["mode"] == "mock" and body["provider"] == "mock" and body["balance"]["available"] is False
    assert body["budget"]["daily_tokens_limit"] == 0
    ping = client.post("/v1/admin/llm-ping").json()
    assert ping["ok"] and ping["mode"] == "mock" and "pong" in ping["answer"]
    kb = client.get("/v1/admin/kb").json()
    assert kb["documents"] > 0 and kb["chunks"] >= kb["documents"]


def test_llm_endpoint_with_transport_and_budget(monkeypatch):
    monkeypatch.setenv("LLM_PROVIDER", "mock")
    app = create_app(settings=OR, retriever=BM25Retriever(load_markdown_kb(KB)))
    app.state.admin_transport = httpx.MockTransport(lambda r: httpx.Response(200, json={"data": {"usage": 3}}))
    body = TestClient(app).get("/v1/admin/llm").json()
    assert body["provider"] == "openrouter" and body["model"] == "m/x" and body["balance"]["usage"] == 3


def test_limiter_budget_status():
    now = [1_000_000.0]
    lim = Limiter(per_minute=5, daily_tokens=100, clock=lambda: now[0])
    lim.spend(30)
    assert lim.budget_status() == {"daily_tokens_limit": 100, "daily_tokens_used": 30, "per_minute_limit": 5}
    now[0] += 86400 * 2
    assert lim.budget_status()["daily_tokens_used"] == 0

"""Usage limits (C3)."""
from fastapi.testclient import TestClient

from app.config import Settings, load_settings
from app.limits import Limiter
from app.main import create_app
from conftest import parse_sse, user_context


class Clock:
    def __init__(self, t=1_763_000_000.0):
        self.t = t

    def __call__(self):
        return self.t


def test_per_minute_window_slides():
    clock = Clock()
    lim = Limiter(per_minute=2, clock=clock)
    assert lim.check("a") is None and lim.check("a") is None
    assert lim.check("a") == "per_minute" and lim.check("b") is None  # per user
    clock.t += 60
    assert lim.check("a") is None


def test_daily_token_budget_resets_next_day():
    clock = Clock()
    lim = Limiter(daily_tokens=1000, clock=clock)
    assert lim.check("a") is None
    lim.spend(1200)
    assert lim.check("b") == "daily_tokens"
    clock.t += 86_400
    assert lim.check("b") is None
    lim2 = Limiter(daily_tokens=10, clock=clock)
    lim2.spend(5)  # spend before any check initialises the day
    assert lim2.check("a") is None
    assert Limiter().check("a") is None  # both limits off


def test_stream_returns_rate_limited_error_and_settings():
    client = TestClient(create_app(Settings(rag_mode="bm25", rate_limit_per_minute=1)))
    body = {"message": "Как оформить отпуск?", "user_context": user_context()}
    assert parse_sse(client.post("/v1/chat/stream", json=body).text)[-1][0] == "done"
    last = parse_sse(client.post("/v1/chat/stream", json=body).text)[-1]
    assert last == ("error", {"code": "rate_limited", "message": "Слишком много запросов. Подожди минуту и попробуй снова."})
    budget = TestClient(create_app(Settings(rag_mode="bm25", daily_token_budget=1)))
    app_limiter_spent = budget.post("/v1/chat/stream", json=body)  # mock spends 0 tokens -> still allowed
    assert parse_sse(app_limiter_spent.text)[-1][0] == "done"
    s = load_settings({"RATE_LIMIT_PER_MINUTE": "5", "LLM_DAILY_TOKEN_BUDGET": "100000"})
    assert (s.rate_limit_per_minute, s.daily_token_budget) == (5, 100000)
    assert load_settings({}).rate_limit_per_minute == 20


def test_budget_message():
    import asyncio
    from app.agent import run_turn
    from app.retrieval import BM25Retriever
    from app.llm import MockLLM
    from app.schemas import AgentChatRequest
    lim = Limiter(daily_tokens=1)
    lim.spend(5)
    req = AgentChatRequest(message="x", user_context=user_context())

    async def collect():
        return [e async for e in run_turn(req, BM25Retriever([]), MockLLM(), Settings(), limiter=lim)]
    events = asyncio.run(collect())
    assert events == [("error", {"code": "rate_limited",
                                 "message": "Дневной лимит ассистента исчерпан. Задай вопрос наставнику или попробуй завтра."})]

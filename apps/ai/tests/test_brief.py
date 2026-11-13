"""Proactive brief (B1)."""
from fastapi.testclient import TestClient

from app.brief import brief_query, changed_requests, run_brief, template_brief
from app.config import Settings
from app.llm import TextDelta
from app.main import create_app
from app.schemas import BriefRequest, UserContext
from conftest import parse_sse, user_context

REQUESTS = [
    {"id": "1", "number": "ABS-000123", "type": "leave_request", "status": "approved",
     "params": {}, "created_at": "2026-11-10T10:00:00Z", "updated_at": "2026-11-12T09:00:00Z"},
    {"id": "2", "number": "ACC-000045", "type": "access_request", "status": "pending_approval",
     "params": {}, "created_at": "2026-11-01T10:00:00Z", "updated_at": "2026-11-01T10:00:00+00:00"},
]
TASKS = [
    {"id": "t5", "title": "Воспроизвести baseline модели рекомендаций", "status": "active", "checklist": "professional"},
    {"id": "t4", "title": "Настроить окружение", "status": "completed", "checklist": "common",
     "review_comment": "Добавь ссылку на ноутбук"},
    {"id": "t6", "title": "Изучить дашборды", "status": "unavailable", "checklist": "professional"},
]


def body(**kw):
    return BriefRequest(user_context=user_context(tasks=TASKS, open_requests=REQUESTS), **kw)


def test_template_brief_mentions_stage_task_comment_and_changed_requests():
    b = body(last_seen_at="2026-11-11T00:00:00")
    text = template_brief(b.user_context, b.last_seen_at)
    assert "Первая неделя" in text and "baseline" in text and "Добавь ссылку на ноутбук" in text
    assert "ABS-000123: согласована" in text and "ACC-000045" not in text  # unchanged since last visit
    assert "Изучить дашборды" not in text
    assert len(changed_requests(b.user_context, None)) == 2
    assert brief_query(b.user_context).startswith("Воспроизвести baseline")
    assert brief_query(UserContext(**user_context(tasks=[], current_stage=None))) == "план стажировки что делать дальше"


def test_brief_endpoint_mock_streams_template():
    client = TestClient(create_app(Settings(rag_mode="bm25")))
    resp = client.post("/v1/brief/stream", json=body().model_dump(mode="json"))
    events = parse_sse(resp.text)
    assert [e for e, _ in events] == ["status", "delta", "done"]
    assert "baseline" in events[1][1]["text"]


def test_llm_brief_uses_task_query_and_no_tools():
    import asyncio

    class Spy:
        turns = []

        async def stream(self, turn):
            self.turns.append(turn)
            yield TextDelta("Бриф [1]")

    from app.retrieval import BM25Retriever, Doc
    r = BM25Retriever([Doc("ml", "ML", "u", "baseline модели рекомендаций воспроизвести"), Doc("x", "X", "u", "кофе")])
    spy = Spy()

    async def collect():
        return [e async for e in run_brief(body(last_seen_at="2026-11-11T00:00:00Z"), r, spy,
                                           Settings(provider="openai_compatible", model="m"))]
    events = asyncio.run(collect())
    turn = spy.turns[0]
    assert turn.tools == [] and turn.hits[0].doc.id == "ml"
    assert "Прошлый визит" in turn.messages[-1]["content"]
    assert dict(events)["sources"]["sources"][0]["id"] == "ml"

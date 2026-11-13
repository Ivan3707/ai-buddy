"""Answer cache for frequent KB questions (C5)."""
from fastapi.testclient import TestClient

from app.cache import AnswerCache, CachedAnswer, normalize
from app.config import Settings, load_settings
from app.main import create_app
from app.schemas import AgentChatRequest
from conftest import parse_sse, user_context

SRC = [{"id": "leave", "title": "t", "url": "u"}]


def req(message="Как оформить отпуск?", **kw):
    return AgentChatRequest(message=message, user_context=user_context(), **kw)


def test_key_put_get_expiry_and_personal_answers_skipped():
    now = [0.0]
    cache = AnswerCache(ttl_s=10, max_size=2, clock=lambda: now[0])
    k = cache.key(req(), 1)
    assert k == cache.key(req("как  оформить ОТПУСК"), 1) and k != cache.key(req(), 2)
    assert cache.key(req(history=[{"role": "user", "content": "x"}]), 1) is None
    assert cache.key(req(page_context={"url": "https://x"}), 1) is None
    assert cache.put(k, req(), CachedAnswer("Заявку подают за 14 дней [1].", SRC))
    assert cache.get(k).text.startswith("Заявку")
    assert not cache.put(k, req(), CachedAnswer("Олег, твой наставник, подскажет [1].", SRC))  # buddy_name
    assert not cache.put(k, req(), CachedAnswer("без источников", []))
    now[0] = 11
    assert cache.get(k) is None
    for i in range(3):
        cache.put(("q", i), req(), CachedAnswer("ok [1]", SRC))
    assert cache.get(("q", 0)) is None and cache.get(("q", 2)) is not None  # LRU bound
    assert normalize("Ёлка,  ЁЖ!") == "елка еж"


def test_second_identical_question_is_served_from_cache():
    client = TestClient(create_app(Settings(rag_mode="bm25", answer_cache_ttl_s=60)))
    body = {"message": "Как оформить отпуск?", "user_context": user_context()}
    first = dict(parse_sse(client.post("/v1/chat/stream", json=body).text))
    second = parse_sse(client.post("/v1/chat/stream", json=body).text)
    assert "cached" not in first["done"] and dict(second)["done"]["cached"] is True
    assert dict(second)["sources"] == first["sources"]
    pii = {"message": "Как оформить отпуск? тел 89123456789", "user_context": user_context()}
    assert "cached" not in dict(parse_sse(client.post("/v1/chat/stream", json=pii).text))["done"]
    assert load_settings({}).answer_cache_ttl_s == 3600 and load_settings({"ANSWER_CACHE_TTL_SECONDS": "0"}).answer_cache_ttl_s == 0

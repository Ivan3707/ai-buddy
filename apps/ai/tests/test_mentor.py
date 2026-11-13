"""B3: POST /v1/mentor/draft (fake LLMs, no network)."""
import json

import pytest
from fastapi.testclient import TestClient

from app.config import Settings
from app.llm import LLMError, TextDelta, Usage
from app.main import create_app
from app.mentor import MentorDraftRequest, build_messages, suggestion_template
from conftest import user_context

mock_client = TestClient(create_app(Settings(rag_mode="bm25")))


class JsonLLM:
    def __init__(self, reply, error=None):
        self.reply, self.error, self.turns = reply, error, []

    async def stream(self, turn):
        self.turns.append(turn)
        if self.error:
            raise self.error
        text = self.reply if isinstance(self.reply, str) else json.dumps(self.reply, ensure_ascii=False)
        yield TextDelta(text)
        yield Usage(10, 5)


def llm_client(llm):
    return TestClient(create_app(Settings(provider="openai_compatible", model="m", rag_mode="bm25"), llm=llm))


def test_mock_draft_from_kb_is_partial_with_sources_and_suggestion():
    r = mock_client.post("/v1/mentor/draft", json={"question": "Как оформить отпуск?"})
    assert r.status_code == 200
    body = r.json()
    assert body["confidence"] == "partial"
    assert body["sources"][0]["id"] == "leave" and "[1]" in body["draft"]
    assert body["kb_suggestion"]["title"].startswith("FAQ: ")
    assert "Отпуска, отгулы и больничный" in body["kb_suggestion"]["text"]


def test_mock_draft_without_kb_is_none():
    body = mock_client.post("/v1/mentor/draft", json={"question": "Есть ли в офисе спортзал?"}).json()
    assert body["confidence"] == "none" and body["sources"] == []
    assert body["draft"].startswith("Нет в базе знаний")
    assert "[ответ наставника" in body["kb_suggestion"]["text"]


def test_grounded_llm_draft_drops_suggestion_and_maps_citations():
    llm = JsonLLM({"draft": "Подай заявку за 14 дней [1].", "confidence": "grounded",
                   "kb_suggestion": {"title": "x", "text": "y"}})
    body = llm_client(llm).post("/v1/mentor/draft", json={
        "question": "Как оформить отпуск?", "context": "стажёр на 2 месяце",
        "user_context": user_context()}).json()
    assert body == {"draft": "Подай заявку за 14 дней [1].", "confidence": "grounded",
                    "sources": [body["sources"][0]]} and body["sources"][0]["id"] == "leave"
    user = llm.turns[0].messages[-1]["content"]
    assert "<kb>" in user and "<intern>" in user and "<context>" in user and "user_id" not in user


def test_partial_llm_draft_keeps_model_suggestion():
    llm = JsonLLM("```json\n" + json.dumps({"draft": "Срок — [1]. Нет в базе знаний: кто замещает.",
                                            "confidence": "partial",
                                            "kb_suggestion": {"title": "Замещение", "text": "## Кто\n[роль]"}},
                                           ensure_ascii=False) + "\n```")
    body = llm_client(llm).post("/v1/mentor/draft", json={"question": "Как оформить отпуск?"}).json()
    assert body["confidence"] == "partial"
    assert body["kb_suggestion"] == {"title": "Замещение", "text": "## Кто\n[роль]"}


@pytest.mark.parametrize("reply", ["просто текст без json", {"draft": "Без ссылок.", "confidence": "grounded"},
                                   {"draft": "x [1]", "confidence": "maybe", "kb_suggestion": {"title": " "}}])
def test_unreliable_model_output_is_normalised(reply):
    body = llm_client(JsonLLM(reply)).post("/v1/mentor/draft", json={"question": "Как оформить отпуск?"}).json()
    assert body["draft"]
    assert body["confidence"] in ("partial", "none")
    assert body["kb_suggestion"]["title"].startswith("FAQ: ")
    if not body["sources"]:
        assert body["confidence"] == "none"


@pytest.mark.parametrize("code", ["llm_unavailable", "timeout"])
def test_llm_error_is_502(code):
    r = llm_client(JsonLLM("", error=LLMError(code))).post("/v1/mentor/draft", json={"question": "Отпуск?"})
    assert r.status_code == 502 and r.json() == {"error": code}


def test_validation():
    for body in ({"question": " "}, {"question": "a" * 2001}, {"question": "x", "extra": 1}, {}):
        assert mock_client.post("/v1/mentor/draft", json=body).status_code == 422


def test_helpers():
    req = MentorDraftRequest(question="Q?")
    msgs = build_messages(req, [])
    assert msgs[0]["role"] == "system" and "ничего не найдено" in msgs[1]["content"]
    long = suggestion_template("в" * 100, [])
    assert long.title.endswith("...") and "- [ссылка]" in long.text

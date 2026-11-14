"""POST /hint: proactive hint for a stuck intern, called by the backend hint worker (fake LLMs, no network)."""
from fastapi.testclient import TestClient

from app.config import Settings
from app.hint import HintRequest, build_messages, clean
from app.llm import LLMError, TextDelta, Usage
from app.main import create_app

mock_client = TestClient(create_app(Settings(rag_mode="bm25")))


class TextLLM:
    def __init__(self, reply="", error=None):
        self.reply, self.error, self.turns = reply, error, []

    async def stream(self, turn):
        self.turns.append(turn)
        if self.error:
            raise self.error
        yield TextDelta(self.reply)
        yield Usage(10, 5)


def llm_client(llm):
    return TestClient(create_app(Settings(provider="openai_compatible", model="m", rag_mode="bm25"), llm=llm))


def test_mock_hint_points_to_kb_document():
    r = mock_client.post("/hint", json={"title": "Оформить отпуск", "description": "Подать заявку на отпуск",
                                        "expected_result": None, "estimated_minutes": 30})
    assert r.status_code == 200
    body = r.json()
    assert body["hint"].startswith("Загляни в «Отпуска, отгулы и больничный»")
    assert "наставнику" in body["hint"] and body["sources"] == ["Отпуска, отгулы и больничный"]


def test_mock_hint_without_kb_match_is_generic():
    body = mock_client.post("/hint", json={"title": "Покормить офисного кота"}).json()
    assert body["hint"].startswith("Разбей задачу на 2–3 небольших шага")
    assert body["sources"] == []


def test_llm_hint_is_cleaned_and_task_is_data():
    llm = TextLLM("Начни с памятки „Отпуска, отгулы и больничный“ [1].\n Если не выйдет — спроси наставника.")
    body = llm_client(llm).post("/hint", json={"title": "Оформить отпуск", "description": "заявка",
                                               "expected_result": "номер заявки", "estimated_minutes": 20}).json()
    assert body["hint"] == "Начни с памятки „Отпуска, отгулы и больничный“. Если не выйдет — спроси наставника."
    assert body["sources"] == ["Отпуска, отгулы и больничный"]
    user = llm.turns[0].messages[-1]["content"]
    assert "<kb>" in user and "<task>" in user and "Плановое время: 20 мин" in user and "номер заявки" in user


def test_llm_failure_and_empty_reply_are_502():
    assert llm_client(TextLLM(error=LLMError("timeout", "slow"))).post("/hint", json={"title": "x"}).status_code == 502
    assert llm_client(TextLLM("  ")).post("/hint", json={"title": "x"}).status_code == 502


def test_validation_and_helpers():
    assert mock_client.post("/hint", json={"title": ""}).status_code == 422
    assert clean("a [1, 2] b  [3]") == "a b"
    msgs = build_messages(HintRequest(title="T"), [])
    assert msgs[0]["role"] == "system" and "Описание" not in msgs[1]["content"]

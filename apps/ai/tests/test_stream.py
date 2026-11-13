"""/v1/chat/stream contract and agent behaviour (mock provider, no network)."""
import json

import pytest
from fastapi.testclient import TestClient

from app.agent import cited_sources, summarize, to_proposal
from app.config import Settings
from app.llm import LLMError, TextDelta, ToolCall, Usage
from app.main import create_app
from app.retrieval import BM25Retriever, Doc, Hit
from app.schemas import AccessRequestParams, MentorQuestionParams
from conftest import parse_sse

client = TestClient(create_app(Settings()))
TERMINAL = {"done", "error"}


def stream(body):
    resp = client.post("/v1/chat/stream", json=body)
    assert resp.status_code == 200
    assert resp.headers["content-type"].startswith("text/event-stream")
    events = parse_sse(resp.text)
    assert sum(e in TERMINAL for e, _ in events) == 1 and events[-1][0] in TERMINAL  # AC-1
    return events


def kinds(events):
    return [e for e, _ in events]


def test_answer_from_kb_streams_deltas_before_done_with_cited_source(chat_body):
    events = stream(chat_body("Как оформить отпуск?"))
    assert kinds(events)[:2] == ["status", "status"]
    assert kinds(events).index("delta") < kinds(events).index("done")
    sources = dict(events)["sources"]["sources"]
    assert sources[0]["id"] == "leave" and sources[0]["snippet"]
    done = dict(events)["done"]
    assert done["finish_reason"] == "answered" and done["mode"] == "mock"
    assert "action" not in kinds(events)


def test_out_of_kb_is_no_answer_with_mentor_card(chat_body):  # AC-3
    events = dict(stream(chat_body("Какой дресс-код на корпоративе?")))
    assert events["done"]["finish_reason"] == "no_answer"
    proposal = events["action"]["proposal"]
    assert proposal["type"] == "mentor_question"
    assert proposal["params"]["question"] == "Какой дресс-код на корпоративе?"
    assert "sources" not in events


def test_leave_proposal_with_fallback_text(chat_body):  # S2
    events = stream(chat_body("Хочу оформить отгул"))
    d = dict(events)
    p = d["action"]["proposal"]
    assert p["type"] == "leave_request"
    assert p["params"] == {"kind": "day_off", "date_from": "2026-11-20", "date_to": "2026-11-20"}
    assert p["summary"] == "Отгул: 20.11.2026"
    assert d["delta"]["text"].startswith("Подготовил заявку")
    assert {"stage": "preparing_action"} in [data for e, data in events if e == "status"]
    assert d["done"]["finish_reason"] == "action_proposed"


def test_access_proposal(chat_body):  # S3
    p = dict(stream(chat_body("Оформи доступ к grafana")))["action"]["proposal"]
    assert p["type"] == "access_request" and p["params"]["system"] == "grafana"


def test_action_outside_allowed_actions_is_never_proposed(chat_body):  # AC-4
    events = dict(stream(chat_body("Хочу оформить отгул", allowed_actions=["mentor_question"])))
    assert events["action"]["proposal"]["type"] == "mentor_question"
    events = dict(stream(chat_body("Хочу оформить отгул", allowed_actions=[])))
    assert "action" not in events


def test_validation_errors(chat_body):
    assert client.post("/v1/chat/stream", json=chat_body("   ")).status_code == 422
    body = chat_body("ok")
    body["user_context"]["role"] = "admin"
    assert client.post("/v1/chat/stream", json=body).status_code == 422
    body = chat_body("ok")
    body["history"] = [{"role": "user", "content": "x"}] * 21
    assert client.post("/v1/chat/stream", json=body).status_code == 422


class ScriptedLLM:
    def __init__(self, *events, error: Exception | None = None):
        self.events, self.error, self.turns = events, error, []

    async def stream(self, turn):
        self.turns.append(turn)
        for ev in self.events:
            yield ev
        if self.error:
            raise self.error


def scripted_client(llm):
    return TestClient(create_app(Settings(provider="mock"), llm=llm))


@pytest.mark.parametrize("error,code", [(LLMError("timeout"), "timeout"),
                                        (LLMError("llm_unavailable"), "llm_unavailable"),
                                        (RuntimeError("boom"), "internal")])
def test_llm_failure_ends_stream_with_error(chat_body, error, code):  # AC-5
    resp = scripted_client(ScriptedLLM(TextDelta("Нача"), error=error)).post(
        "/v1/chat/stream", json=chat_body("Как оформить отпуск?"))
    events = parse_sse(resp.text)
    assert events[-1][0] == "error" and events[-1][1]["code"] == code
    assert "boom" not in resp.text
    assert "done" not in kinds(events)


def test_invalid_tool_call_is_dropped_and_usage_reported(chat_body):
    llm = ScriptedLLM(ToolCall("propose_leave_request", '{"kind":"day_off","date_from":"2026-11-20"}'),
                      ToolCall("rm_rf", "{}"), Usage(120, 7))
    events = dict(parse_sse(scripted_client(llm).post("/v1/chat/stream", json=chat_body("Как оформить отпуск?")).text))
    assert "action" not in events
    assert events["delta"]["text"].startswith("Не получилось")
    assert events["done"]["usage"] == {"input_tokens": 120, "output_tokens": 7}


def test_llm_sees_kb_in_user_turn_and_only_allowed_tools(chat_body):
    llm = ScriptedLLM(TextDelta("ok"))
    body = chat_body("Как оформить отпуск?", allowed_actions=["mentor_question"])
    body["history"] = [{"role": "user", "content": "привет"}, {"role": "assistant", "content": "привет!"}]
    scripted_client(llm).post("/v1/chat/stream", json=body)
    turn = llm.turns[0]
    roles = [m["role"] for m in turn.messages]
    assert roles == ["system", "user", "assistant", "user"]
    assert "<user_context>" in turn.messages[0]["content"] and "«Заявление на отпуск»" not in turn.messages[0]["content"]
    assert "[1] «Заявление на отпуск»" in turn.messages[-1]["content"]
    assert [t["function"]["name"] for t in turn.tools] == ["propose_mentor_question"]


def test_cited_sources_ignores_out_of_range_and_duplicates():
    doc = Doc("a", "A", "u", "text")
    assert [s.id for s in cited_sources("x [1] y [1] [7] [0]", [Hit(doc, 1.0)])] == ["a"]
    assert cited_sources("no refs", [Hit(doc, 1.0)]) == []


def test_summaries_and_proposal_validation():
    assert summarize("access_request", AccessRequestParams(
        system="jira", resource="ML", access_level="read", justification="x")) == "Доступ jira · ML · read"
    long_q = MentorQuestionParams(question="?" * 200)
    assert summarize("mentor_question", long_q).endswith("...")
    vacation = to_proposal(ToolCall("propose_leave_request", json.dumps(
        {"kind": "vacation", "date_from": "2026-12-01", "date_to": "2026-12-14"})), {"leave_request"})
    assert vacation.summary == "Отпуск: 01.12.2026 — 14.12.2026"
    backwards = ToolCall("propose_leave_request", json.dumps(
        {"kind": "vacation", "date_from": "2026-12-14", "date_to": "2026-12-01"}))
    assert to_proposal(backwards, {"leave_request"}) is None
    assert to_proposal(ToolCall("propose_mentor_question", "not json"), {"mentor_question"}) is None


def test_health_reports_mode():
    assert client.get("/health").json() == {"status": "ok", "mode": "mock"}
    llm_app = TestClient(create_app(Settings(provider="openai_compatible", model="m"), llm=ScriptedLLM()))
    assert llm_app.get("/health").json() == {"status": "ok", "mode": "llm", "model": "m"}


def test_retriever_ranks_and_thresholds():
    r = BM25Retriever([Doc("leave", "Отпуск", "u", "Заявление на отпуск"), Doc("acc", "Доступы", "u", "Jira")])
    assert [h.doc.id for h in r.search("отпуска", 4, 0.1)] == ["leave"]
    assert r.search("погода", 4, 0.1) == [] and r.search("!!!", 4, 0.0) == []
    assert BM25Retriever([]).search("отпуск", 4, 0.0) == []

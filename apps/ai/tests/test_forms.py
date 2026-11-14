"""Forms in the chat: the model asks for a form, the chat draws it, the user submits it (no network, scripted LLM)."""
import json

from app.forms import FIELDS, allowed_forms, sanitize_prefill, to_form
from app.llm import ToolCall
from app.prompts import FORM_TEXT, build_tools
from conftest import parse_sse
from test_stream import ScriptedLLM, scripted_client


def call(form, prefill=None):
    return ToolCall("show_form", json.dumps({"form": form, "prefill": prefill or {}}))


def test_forms_follow_the_allowed_actions():
    assert allowed_forms([]) == ["note", "event"]
    assert allowed_forms({"mentor_question"}) == ["mentor_question", "note", "event"]
    assert set(allowed_forms(["leave_request", "access_request", "mentor_question"])) == set(FIELDS)


def test_the_form_tool_lists_only_allowed_forms():
    tools = {t["function"]["name"]: t for t in build_tools(["leave_request"])}
    enum = tools["show_form"]["function"]["parameters"]["properties"]["form"]["enum"]
    assert enum == ["leave_request", "note", "event"]


def test_prefill_keeps_only_valid_known_values():
    got = sanitize_prefill("leave_request", {
        "kind": "vacation", "date_from": "2026-12-01", "date_to": "01.12.2026", "comment": "  отпуск  ", "secret": "x", "admin": "1"})
    assert got == {"kind": "vacation", "date_from": "2026-12-01", "comment": "отпуск"}
    assert sanitize_prefill("access_request", {"system": "mainframe", "access_level": "admin", "resource": "ML"}, "intern") == {"resource": "ML"}
    assert sanitize_prefill("event", {"starts_at": "2026-11-20T10:00:00+03:00", "ends_at": "завтра"}) == {"starts_at": "2026-11-20T10:00"}
    assert sanitize_prefill("note", {"title": "t" * 500})["title"] == "t" * 160
    assert sanitize_prefill("note", "not an object") == {} and sanitize_prefill("unknown", {"a": "b"}) == {}


def test_to_form_rejects_unknown_or_forbidden_forms_and_bad_json():
    assert to_form(call("note", {"title": "Идея"}), ["note"]) == {"type": "note", "prefill": {"title": "Идея"}}
    assert to_form(call("access_request"), ["note", "event"]) is None
    assert to_form(ToolCall("show_form", "{broken"), ["note"]) is None
    assert to_form(ToolCall("propose_leave_request", "{}"), ["note"]) is None


def test_agent_turn_emits_a_form_event(chat_body):
    llm = ScriptedLLM(call("leave_request", {"kind": "day_off", "date_from": "2026-11-21"}))
    events = parse_sse(scripted_client(llm).post("/v1/chat/stream", json=chat_body("Хочу отгул 21 ноября")).text)
    names = [e for e, _ in events]
    data = dict(events)
    assert "action" not in names and "form" in names
    assert data["form"]["form"] == {"type": "leave_request", "prefill": {"kind": "day_off", "date_from": "2026-11-21"}}
    assert data["delta"]["text"] == FORM_TEXT
    assert data["done"]["finish_reason"] == "form_shown"
    assert any(t["function"]["name"] == "show_form" for t in llm.turns[0].tools)


def test_a_forbidden_form_is_dropped(chat_body):
    llm = ScriptedLLM(call("access_request", {"system": "grafana"}))
    body = chat_body("Нужен доступ", allowed_actions=[])
    events = parse_sse(scripted_client(llm).post("/v1/chat/stream", json=body).text)
    assert "form" not in [e for e, _ in events]

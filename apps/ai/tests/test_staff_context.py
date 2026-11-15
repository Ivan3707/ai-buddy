"""Mentor and team lead context: the model gets their interns from the backend, and staff data never gets cached."""
from app.cache import AnswerCache, CachedAnswer, personal_markers
from app.prompts import build_messages
from app.schemas import AgentChatRequest

TEAM = {
    "my_interns": [{"name": "Алексей Кузнецов", "profession": "ml", "buddy": "Ольга Соколова", "current_stage": "Первая неделя",
                    "checklists": [{"checklist": "common", "done": 3, "total": 8}, {"checklist": "professional", "done": 1, "total": 6}],
                    "current_tasks": ["Настроить окружение (on_review)"], "on_review": 1, "returned": 0, "last_login": "2026-11-15"}],
    "open_mentor_questions": [{"intern": "Алексей Кузнецов", "question": "Когда выдадут ноутбук?", "since": "2026-11-14"}],
}


def request(role, message="Сколько у меня стажёров и какие у них результаты?", **extra):
    ctx = {"user_id": "u1", "role": role, "tasks": [], "open_requests": [], "allowed_actions": ["leave_request"], **extra}
    return AgentChatRequest.model_validate({"message": message, "user_context": ctx})


def test_buddy_prompt_has_team_and_staff_rules():
    system, user = build_messages(request("buddy", **TEAM), [])[0]["content"], build_messages(request("buddy", **TEAM), [])[-1]["content"]
    assert "Алексей Кузнецов" in system and '"done": 3' in system and "Когда выдадут ноутбук?" in system
    assert "role=buddy" in system and "my_interns" in system
    assert "Сообщение пользователя:" in user


def test_teamlead_prompt_mentions_approvals():
    req = request("teamlead", my_interns=[], pending_approvals=[{"intern": "Иван Петров", "number": "ABS-000002", "type": "leave_request", "status": "pending_approval"}])
    system = build_messages(req, [])[0]["content"]
    assert "role=teamlead" in system and "ABS-000002" in system and "pending_approvals" in system


def test_intern_prompt_unchanged_and_has_no_team():
    req = request("intern", message="Что мне делать?")
    msgs = build_messages(req, [])
    assert "my_interns" not in msgs[0]["content"].split("<user_context>")[1]
    assert "Сообщение стажёра:" in msgs[-1]["content"]


def test_staff_answers_with_intern_names_are_not_cached():
    req = request("buddy", **TEAM)
    assert "Алексей Кузнецов" in personal_markers(req)
    cache = AnswerCache(ttl_s=60)
    answer = CachedAnswer("У тебя один стажёр, Алексей Кузнецов [1].", [{"id": "doc"}])
    assert cache.put(("k",), req, answer) is False

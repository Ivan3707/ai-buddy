"""Mock mode answers the questions our own buttons send from the user's context instead of escalating them."""
import asyncio

from app import mock_answers
from app.llm import MockLLM, TextDelta, ToolCall, Turn
from app.schemas import AgentChatRequest

SUMMARY_QUESTION = "Дай краткую сводку на сегодня: мои задачи, заявки и что делать дальше."
TASKS = [
    {"id": "t1", "title": "Собрать бейзлайн", "description": "Обучить простую модель и записать метрики.",
     "status": "active", "checklist": "professional", "requires_result": True},
    {"id": "t2", "title": "Настроить окружение", "status": "returned", "checklist": "professional",
     "review_comment": "Добавь версии библиотек"},
    {"id": "t3", "title": "Изучить выборку", "status": "on_review", "checklist": "professional"},
    {"id": "t4", "title": "Познакомиться с командой", "status": "completed", "checklist": "common"},
]
REQUESTS = [{"id": "r1", "number": "ABS-000003", "type": "leave_request", "status": "pending_approval",
             "params": {"kind": "day_off", "date_from": "2026-11-20", "date_to": "2026-11-20"},
             "created_at": "2026-11-15T10:00:00Z", "updated_at": "2026-11-15T10:00:00Z"}]
INTERN = {"name": "Алексей Кузнецов", "current_stage": "Окружение и данные", "on_review": 1, "returned": 0,
          "checklists": [{"checklist": "professional", "done": 1, "total": 6}, {"checklist": "common", "done": 3, "total": 12}]}


def request(message, role="intern", **extra):
    ctx = {"user_id": "u1", "display_name": "Алексей", "role": role, "tasks": [], "open_requests": [],
           "allowed_actions": ["leave_request", "access_request", "mentor_question"],
           "current_stage": {"title": "Окружение и данные", "position": 2, "total": 5}, **extra}
    return AgentChatRequest.model_validate({"message": message, "user_context": ctx})


def stream(req):
    tools = [{"function": {"name": n}} for n in ("propose_leave_request", "propose_mentor_question")]

    async def collect():
        return [ev async for ev in MockLLM().stream(Turn(req, [], [], tools))]
    return asyncio.run(collect())


def test_summary_button_answers_from_context_without_escalation():
    events = stream(request(SUMMARY_QUESTION, tasks=TASKS, open_requests=REQUESTS))
    text = "".join(e.text for e in events if isinstance(e, TextDelta))
    assert not [e for e in events if isinstance(e, ToolCall)]
    assert "Окружение и данные" in text and "ABS-000003" in text and "ожидает согласования" in text
    assert "«Собрать бейзлайн» — в работе" in text and "Изучить выборку" in text
    assert "Познакомиться с командой" not in text  # finished tasks are not "today"
    assert "доработай «Настроить окружение»" in text  # a returned task comes first


def test_summary_without_tasks_or_requests():
    text = mock_answers.answer(request(SUMMARY_QUESTION))
    assert "Текущих задач нет" in text and "Открытых заявок нет" in text


def test_task_help_button_uses_the_task():
    text = mock_answers.answer(request("Помоги с задачей «Собрать бейзлайн»: с чего начать?", tasks=TASKS))
    assert "Обучить простую модель" in text and "на проверку" in text
    assert "Не нашёл" in mock_answers.answer(request("Помоги с задачей «Чужая»: с чего начать?", tasks=TASKS))


def test_request_status_and_actions_are_not_mixed():
    assert "ABS-000003" in mock_answers.answer(request("Что с моей заявкой на отгул?", open_requests=REQUESTS))
    assert "нет" in mock_answers.answer(request("Какие у меня заявки?"))
    assert mock_answers.answer(request("Оформи отгул на пятницу")) is None  # the action rules handle it
    assert mock_answers.answer(request("Как оформить отпуск?")) is None  # a KB question


def test_mentor_and_teamlead_get_their_team():
    buddy = request("Сколько у меня стажёров и какие у них результаты?", role="buddy", my_interns=[INTERN],
                    open_mentor_questions=[{"intern": "Алексей Кузнецов", "question": "Можно ли забирать ноутбук?", "since": "2026-11-14"}])
    text = mock_answers.answer(buddy)
    assert "стажёров: 1" in text and "профессиональный чек-лист 1/6" in text and "проверить работы (1)" in text
    questions = mock_answers.answer(request("Какие вопросы стажёров ждут моего ответа?", role="buddy", my_interns=[INTERN],
                                            open_mentor_questions=buddy.user_context.open_mentor_questions))
    assert "Можно ли забирать ноутбук?" in questions
    lead = request("Какие заявки ждут моего согласования?", role="teamlead", my_interns=[],
                   pending_approvals=[{"intern": "Иван Петров", "number": "ABS-000002", "type": "leave_request", "status": "pending_approval"}])
    assert "Иван Петров: ABS-000002" in mock_answers.answer(lead)
    empty = request("Как дела у моей команды?", role="teamlead", my_interns=[], pending_approvals=[])
    assert "не закреплены" in mock_answers.answer(empty)


def test_unknown_question_falls_back_to_keyword_rules():
    events = stream(request("Можно ли привести собаку в офис?"))
    assert any(isinstance(e, ToolCall) and e.name == "propose_mentor_question" for e in events)

"""Ready answers for LLM_PROVIDER=mock: the questions our own UI sends, answered from the user's context.

Buddy's proactive buttons send fixed questions ("Дай краткую сводку на сегодня…", "Помоги с задачей «…»",
"Что с моей заявкой?", a mentor's "Сколько у меня стажёров?"). Without a model the keyword mock used to
escalate them to the mentor, which looks broken. The answer to each of them is in the UserContext the
backend sends (RBAC applied there), so a template is enough: no KB search, no network, deterministic.
Returns None for anything else; the mock then falls back to its keyword rules.
"""
from __future__ import annotations

import re

from .schemas import AgentChatRequest, TaskContext, UserContext

TASK_STATUS = {"active": "в работе", "on_review": "на проверке у наставника", "returned": "возвращена на доработку",
               "completed": "выполнена", "unavailable": "откроется позже"}
REQUEST_STATUS = {"pending_approval": "ожидает согласования", "approved": "согласована", "rejected": "отклонена"}
REQUEST_KIND = {"leave_request": "отгул или отпуск", "access_request": "доступ"}
CHECKLIST = {"common": "общий", "professional": "профессиональный"}
NOTE = "\n\n_Демо-режим без LLM: ответ собран из данных сервиса._"

SUMMARY = re.compile(r"сводк|что (?:мне )?(?:делать|дальше)|мои задачи|план на сегодня|как дела у (?:моей )?команд"
                     r"|(?:мои[хм]?|сколько у меня) стаж[её]р|результаты стаж[её]р")
TASK_HELP = re.compile(r"помоги с задачей «([^»]+)»", re.IGNORECASE)
REQUESTS = re.compile(r"заявк")
REQUEST_ASK = re.compile(r"что с|статус|мои заявки|какие (?:у меня )?заявки|ждут|согласован")
ACTION = re.compile(r"оформи|создай|подай|хочу взять|нужен доступ|нужен отгул")
QUESTIONS = re.compile(r"вопрос")
QUESTIONS_ASK = re.compile(r"ждут|открыт|без ответа|какие|сколько")


def _norm(text: str) -> str:
    return text.casefold().replace("ё", "е")


def _task_line(t: TaskContext) -> str:
    line = f"«{t.title}» — {TASK_STATUS.get(t.status, t.status)}"
    if t.review_comment and t.status == "returned":
        line += f". Замечание наставника: {t.review_comment}"
    return line


def _current(ctx: UserContext) -> list[TaskContext]:
    return [t for t in ctx.tasks if t.status in ("active", "on_review", "returned")]


def _next_step(ctx: UserContext) -> str:
    current = _current(ctx)
    returned = next((t for t in current if t.status == "returned"), None)
    if returned:
        return f"Сначала доработай «{returned.title}» по замечанию наставника и отправь на проверку снова."
    active = next((t for t in current if t.status == "active"), None)
    if active:
        return f"Продолжай «{active.title}». Если застрял, спроси меня или наставника."
    if current:
        return "Все текущие задачи на проверке. Пока ждёшь ответа, посмотри, что откроется дальше в разделе «Задачи»."
    return "Текущих задач нет. Загляни в раздел «Задачи»: там видно, что откроется дальше."


def _requests(ctx: UserContext) -> list[str]:
    lines = []
    for r in ctx.open_requests[:5]:
        p = r.params
        what = p.get("system") or ""
        dates = p.get("date_from") or ""
        if p.get("date_to") and p.get("date_to") != dates:
            dates += f" — {p['date_to']}"
        detail = ", ".join(x for x in (REQUEST_KIND.get(r.type, r.type), what, dates) if x)
        lines.append(f"• {r.number} ({detail}): {REQUEST_STATUS.get(r.status, r.status)}")
    return lines


def intern_summary(ctx: UserContext) -> str:
    lines = [f"Сводка на сегодня{', ' + ctx.display_name if ctx.display_name else ''}:"]
    if ctx.current_stage:
        s = ctx.current_stage
        lines.append(f"• Этап «{s.title}» ({s.position} из {s.total}).")
    current = _current(ctx)
    lines += [f"• Задача {_task_line(t)}." for t in current[:4]] or ["• Текущих задач нет."]
    requests = _requests(ctx)
    lines += requests or ["• Открытых заявок нет."]
    lines.append("")
    lines.append("Что дальше: " + _next_step(ctx))
    return "\n".join(lines)


def _intern_line(i) -> str:
    parts = []
    if i.current_stage:
        parts.append(f"этап «{i.current_stage}»")
    for c in i.checklists:
        parts.append(f"{CHECKLIST.get(c.checklist, c.checklist)} чек-лист {c.done}/{c.total}")
    if i.on_review:
        parts.append(f"на проверке {i.on_review}")
    if i.returned:
        parts.append(f"возвращено {i.returned}")
    if i.buddy:
        parts.append(f"наставник {i.buddy}")
    return f"• {i.name}: " + (", ".join(parts) or "данных о прогрессе пока нет")


def staff_summary(ctx: UserContext) -> str:
    interns = ctx.my_interns or []
    if not interns:
        lines = ["За тобой пока не закреплены стажёры."]
    else:
        lines = [f"За тобой закреплено стажёров: {len(interns)}."]
        lines += [_intern_line(i) for i in interns]
    on_review = sum(i.on_review for i in interns)
    todo = []
    if on_review:
        todo.append(f"проверить работы ({on_review}) в разделе «Команда»")
    if ctx.open_mentor_questions:
        todo.append(f"ответить на вопросы стажёров ({len(ctx.open_mentor_questions)}) в разделе «Вопросы»")
    if ctx.pending_approvals:
        todo.append(f"согласовать заявки ({len(ctx.pending_approvals)}) в разделе «Заявки»")
    if todo:
        lines += ["", "Что сделать: " + "; ".join(todo) + "."]
    return "\n".join(lines)


def approvals(ctx: UserContext) -> str:
    items = ctx.pending_approvals or []
    if not items:
        return "Заявок, которые ждут твоего согласования, нет."
    lines = [f"Ждут твоего согласования: {len(items)}."]
    lines += [f"• {a.intern}: {a.number} ({REQUEST_KIND.get(a.type, a.type)})" for a in items]
    lines.append("Открыть и согласовать их можно в разделе «Заявки».")
    return "\n".join(lines)


def mentor_questions(ctx: UserContext) -> str:
    items = ctx.open_mentor_questions or []
    if not items:
        return "Открытых вопросов от стажёров нет."
    lines = [f"Ждут твоего ответа: {len(items)}."]
    lines += [f"• {q.intern}: «{q.question}»" for q in items]
    lines.append("Ответить можно в разделе «Вопросы», там же есть AI-черновик ответа.")
    return "\n".join(lines)


def my_requests(ctx: UserContext) -> str:
    lines = _requests(ctx)
    if not lines:
        return "Открытых заявок у тебя нет. Оформить новую можно прямо здесь: например, «оформи отгул на пятницу»."
    return "Твои заявки:\n" + "\n".join(lines)


def task_help(ctx: UserContext, title: str) -> str:
    task = next((t for t in ctx.tasks if t.title.casefold() == title.casefold()), None)
    if task is None:
        return f"Не нашёл у тебя задачу «{title}». Открой раздел «Задачи» и спроси меня ещё раз оттуда."
    lines = [f"Задача {_task_line(task)}."]
    if task.description:
        lines.append(f"Что нужно сделать: {task.description}")
    if task.requires_result:
        lines.append("Когда закончишь, отправь результат на проверку наставнику прямо на карточке задачи.")
    lines.append("Начни с первого пункта описания. Если что-то непонятно, задай вопрос здесь — найду ответ в базе"
                 " знаний или передам его наставнику.")
    return "\n".join(lines)


def answer(req: AgentChatRequest) -> str | None:
    msg = _norm(req.message)
    ctx = req.user_context
    staff = ctx.role in ("buddy", "teamlead")
    if ACTION.search(msg):
        return None
    if m := TASK_HELP.search(req.message):
        return task_help(ctx, m.group(1)) + NOTE
    if REQUESTS.search(msg) and REQUEST_ASK.search(msg):
        return (approvals(ctx) if ctx.role == "teamlead" and ctx.pending_approvals is not None else my_requests(ctx)) + NOTE
    if staff and QUESTIONS.search(msg) and QUESTIONS_ASK.search(msg) and ctx.open_mentor_questions is not None:
        return mentor_questions(ctx) + NOTE
    if SUMMARY.search(msg):
        return (staff_summary(ctx) if staff else intern_summary(ctx)) + NOTE
    return None

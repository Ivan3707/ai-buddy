"""Proactive brief (B1): the bot speaks first when the intern opens the chat.

Avito's feedback: "привязать LLM к чек-листу — не решение"; proactivity and practice are valued.
The brief turns the verified UserContext into 3-5 concrete points: stage, the active task with a
practical first step from the KB, request status changes since the last visit, mentor's review
comments. No actions are proposed here (the tool list is empty).
"""
from __future__ import annotations

from collections.abc import AsyncIterator
from datetime import datetime, timezone

from .agent import Event, run_turn
from .config import Settings
from .limits import Limiter
from .llm import LLM, MockLLM
from .retrieval import Retriever
from .schemas import AgentChatRequest, BriefRequest, UserContext

ACTIVE = {"active", "in_progress", "returned", "needs_rework", "available"}
STATUS_RU = {"pending_approval": "ожидает согласования", "approved": "согласована", "rejected": "отклонена"}

BRIEF_INSTRUCTION = (
    "Я только что открыл чат. Сделай короткий проактивный бриф для меня, 3–5 пунктов, без приветственной воды:\n"
    "1) мой этап и прогресс;\n"
    "2) активная задача и конкретный первый шаг или практическое упражнение по базе знаний (со ссылками [n]);\n"
    "3) изменения в моих заявках с момента прошлого визита (по open_requests и updated_at), "
    "«ожидает согласования» не называй одобрением;\n"
    "4) замечания наставника к моим заданиям (review_comment), если есть.\n"
    "Пропускай пункты, по которым нечего сказать. В конце одной фразой предложи, с чем можешь помочь. "
    "Ничего не оформляй и не придумывай."
)


def active_tasks(ctx: UserContext) -> list:
    return [t for t in ctx.tasks if t.status in ACTIVE or t.review_comment]


def brief_query(ctx: UserContext) -> str:
    """Retrieval query: what the KB should say something practical about."""
    parts = [t.title for t in active_tasks(ctx)[:3]]
    if ctx.current_stage:
        parts.append(ctx.current_stage.title)
    return " ".join(parts) or "план стажировки что делать дальше"


def _aware(dt: datetime) -> datetime:
    return dt if dt.tzinfo else dt.replace(tzinfo=timezone.utc)


def changed_requests(ctx: UserContext, last_seen_at: datetime | None) -> list:
    if last_seen_at is None:
        return list(ctx.open_requests)
    since = _aware(last_seen_at)
    return [r for r in ctx.open_requests
            if _aware(datetime.fromisoformat(r.updated_at.replace("Z", "+00:00"))) >= since]


def template_brief(ctx: UserContext, last_seen_at: datetime | None) -> str:
    """Deterministic brief for LLM_PROVIDER=mock (tests, CI, demo without a key)."""
    lines = [f"Привет{', ' + ctx.display_name if ctx.display_name else ''}! Коротко о главном:"]
    if ctx.current_stage:
        s = ctx.current_stage
        lines.append(f"• Этап «{s.title}» ({s.position} из {s.total}).")
    for t in active_tasks(ctx)[:2]:
        lines.append(f"• Задача «{t.title}»" + (f": замечание наставника — {t.review_comment}" if t.review_comment
                                                 else " — с неё и начнём."))
    for r in changed_requests(ctx, last_seen_at)[:3]:
        lines.append(f"• Заявка {r.number}: {STATUS_RU.get(r.status, r.status)}.")
    lines.append("Спроси, если нужно разобрать задачу или оформить заявку.")
    return "\n".join(lines)


async def run_brief(body: BriefRequest, retriever: Retriever, llm: LLM, settings: Settings,
                    limiter: Limiter | None = None) -> AsyncIterator[Event]:
    ctx = body.user_context
    if isinstance(llm, MockLLM):
        yield "status", {"stage": "generating"}
        yield "delta", {"text": template_brief(ctx, body.last_seen_at)}
        yield "done", {"finish_reason": "answered", "mode": settings.mode, "latency_ms": 0,
                       "usage": {"input_tokens": 0, "output_tokens": 0, "cached_input_tokens": 0}}
        return
    message = BRIEF_INSTRUCTION
    if body.last_seen_at:
        message += f"\nПрошлый визит: {body.last_seen_at.isoformat()}."
    req = AgentChatRequest(message=message, user_context=ctx.model_copy(update={"allowed_actions": []}))
    async for event in run_turn(req, retriever, llm, settings, query=brief_query(ctx), limiter=limiter):
        yield event

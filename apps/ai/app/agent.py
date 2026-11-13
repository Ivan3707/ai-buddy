"""One agent turn: retrieve -> generate (stream) -> validate proposed action -> SSE events.

The AI never executes actions: a valid tool call becomes an ActionProposal that Backend stores
and runs only after the user confirms.
"""
from __future__ import annotations

import json
import logging
import re
import time
from collections.abc import AsyncIterator
from typing import Any

from pydantic import BaseModel, ValidationError

from .config import Settings
from .llm import LLM, LLMError, TextDelta, ToolCall, Turn, Usage
from .prompts import FALLBACK_TEXT, TOOL_TO_ACTION, build_messages, build_tools
from .retrieval import BM25Retriever, Hit
from .schemas import (AccessRequestParams, ActionProposal, AgentChatRequest, LeaveRequestParams,
                      MentorQuestionParams, Source)

log = logging.getLogger("ai.agent")

PARAMS: dict[str, type[BaseModel]] = {
    "leave_request": LeaveRequestParams,
    "access_request": AccessRequestParams,
    "mentor_question": MentorQuestionParams,
}
ERROR_MESSAGES = {
    "llm_unavailable": "Модель сейчас недоступна. Попробуй ещё раз чуть позже.",
    "timeout": "Модель отвечает слишком долго. Попробуй ещё раз.",
    "internal": "Что-то пошло не так. Попробуй ещё раз.",
}
EMPTY_ANSWER = "Не получилось сформулировать ответ. Переформулируй вопрос или спроси наставника."
CITATION = re.compile(r"\[(\d{1,2})\]")
Event = tuple[str, dict[str, Any]]


def summarize(action: str, p: BaseModel) -> str:
    """Card text is built by our code, not by the model: deterministic and not injectable."""
    if isinstance(p, LeaveRequestParams):
        what = "Отгул" if p.kind == "day_off" else "Отпуск"
        dates = p.date_from.strftime("%d.%m.%Y")
        if p.date_to != p.date_from:
            dates += " — " + p.date_to.strftime("%d.%m.%Y")
        return f"{what}: {dates}"
    if isinstance(p, AccessRequestParams):
        return f"Доступ {p.system} · {p.resource} · {p.access_level}"
    assert isinstance(p, MentorQuestionParams)
    q = p.question if len(p.question) <= 120 else p.question[:117] + "..."
    return f"Вопрос наставнику: {q}"


def to_proposal(call: ToolCall, allowed: set[str]) -> ActionProposal | None:
    action = TOOL_TO_ACTION.get(call.name)
    if action is None or action not in allowed:
        log.warning("dropped tool call: unknown or not allowed tool=%s", call.name)
        return None
    try:
        params = PARAMS[action].model_validate(json.loads(call.arguments or "{}"))
    except (ValueError, ValidationError):  # bad JSON or schema violation
        log.warning("dropped tool call: invalid arguments tool=%s", call.name)
        return None
    return ActionProposal(type=action, params=params.model_dump(mode="json", exclude_none=True),
                          summary=summarize(action, params))


def cited_sources(text: str, hits: list[Hit]) -> list[Source]:
    """Only documents the answer actually cites ([n]) are returned as sources."""
    seen: list[int] = []
    for m in CITATION.finditer(text):
        n = int(m.group(1))
        if 1 <= n <= len(hits) and n not in seen:
            seen.append(n)
    return [Source(id=hits[n - 1].doc.id, title=hits[n - 1].doc.title, url=hits[n - 1].doc.url,
                   snippet=hits[n - 1].doc.text[:200]) for n in seen]


async def run_turn(req: AgentChatRequest, retriever: BM25Retriever, llm: LLM,
                   settings: Settings) -> AsyncIterator[Event]:
    t0 = time.monotonic()
    yield "status", {"stage": "retrieving"}
    hits = retriever.search(req.message, settings.top_k, settings.min_score)
    allowed = set(req.user_context.allowed_actions)
    turn = Turn(req, hits, build_messages(req, hits), build_tools(sorted(allowed)))

    yield "status", {"stage": "generating"}
    parts: list[str] = []
    calls: list[ToolCall] = []
    usage = Usage()
    try:
        async for ev in llm.stream(turn):
            if isinstance(ev, TextDelta):
                parts.append(ev.text)
                yield "delta", {"text": ev.text}
            elif isinstance(ev, ToolCall):
                calls.append(ev)
            else:
                usage = ev
    except LLMError as e:
        log.warning("llm error code=%s detail=%s latency_ms=%d", e.code, e, (time.monotonic() - t0) * 1000)
        yield "error", {"code": e.code, "message": ERROR_MESSAGES[e.code]}
        return
    except Exception:  # never leak internals into the stream
        log.exception("agent turn failed")
        yield "error", {"code": "internal", "message": ERROR_MESSAGES["internal"]}
        return

    proposal = next((p for p in (to_proposal(c, allowed) for c in calls) if p), None)
    text = "".join(parts)
    if proposal:
        yield "status", {"stage": "preparing_action"}
    if not text.strip():
        text = FALLBACK_TEXT[proposal.type] if proposal else EMPTY_ANSWER
        yield "delta", {"text": text}

    sources = cited_sources(text, hits)
    if sources:
        yield "sources", {"sources": [s.model_dump(exclude_none=True) for s in sources]}
    if proposal:
        yield "action", {"proposal": proposal.model_dump(mode="json")}

    if proposal and proposal.type != "mentor_question":
        finish = "action_proposed"
    elif proposal or not hits:
        finish = "no_answer"  # escalation offered or nothing grounded found
    else:
        finish = "answered"
    latency_ms = int((time.monotonic() - t0) * 1000)
    # metadata only: no message text or PII in logs
    log.info("turn finish=%s hits=%d tool_calls=%d action=%s latency_ms=%d in=%d out=%d",
             finish, len(hits), len(calls), proposal.type if proposal else "-", latency_ms,
             usage.input_tokens, usage.output_tokens)
    yield "done", {"finish_reason": finish, "mode": settings.mode, "latency_ms": latency_ms,
                   "usage": {"input_tokens": usage.input_tokens, "output_tokens": usage.output_tokens}}


def sse(event: str, data: dict[str, Any]) -> str:
    return f"event: {event}\ndata: {json.dumps(data, ensure_ascii=False)}\n\n"

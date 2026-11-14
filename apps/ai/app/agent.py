"""One agent turn: retrieve -> generate (stream) -> validate proposed action -> SSE events.

The AI never executes actions: a valid tool call becomes an ActionProposal that Backend stores
and runs only after the user confirms.
"""
from __future__ import annotations

import json
import logging
import re
import time
from collections import Counter
from collections.abc import AsyncIterator
from typing import Any

from pydantic import BaseModel, ValidationError

from .config import Settings
from .forms import allowed_forms, to_form
from .llm import LLM, LLMError, TextDelta, ToolCall, Turn, Usage
from . import metrics
from .cache import AnswerCache, CachedAnswer
from .limits import Limiter
from .pii import mask
from .prompts import ACCESS_LEVELS_BY_ROLE, FALLBACK_TEXT, TOOL_TO_ACTION, build_messages, build_tools, FORM_TEXT
from .retrieval import Hit, Retriever, fuse, tokenize
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
    "rate_limited": "Слишком много запросов. Подожди минуту и попробуй снова.",
}
BUDGET_MESSAGE = "Дневной лимит ассистента исчерпан. Задай вопрос наставнику или попробуй завтра."
EMPTY_ANSWER = "Не получилось сформулировать ответ. Переформулируй вопрос или спроси наставника."
CITATION = re.compile(r"\[([^\[\]]{1,60})\]")
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


def to_proposal(call: ToolCall, allowed: set[str], role: str = "intern") -> ActionProposal | None:
    action = TOOL_TO_ACTION.get(call.name)
    if action is None or action not in allowed:
        log.warning("dropped tool call: unknown or not allowed tool=%s", call.name)
        metrics.DROPPED_TOOL_CALLS.labels("not_allowed").inc()
        return None
    try:
        params = PARAMS[action].model_validate(json.loads(call.arguments or "{}"))
    except (ValueError, ValidationError):  # bad JSON or schema violation
        log.warning("dropped tool call: invalid arguments tool=%s", call.name)
        metrics.DROPPED_TOOL_CALLS.labels("invalid_arguments").inc()
        return None
    if isinstance(params, AccessRequestParams) and params.access_level not in ACCESS_LEVELS_BY_ROLE.get(role, ["read"]):
        log.warning("dropped tool call: access level not allowed for role=%s", role)
        metrics.DROPPED_TOOL_CALLS.labels("access_level_policy").inc()
        return None
    return ActionProposal(type=action, params=params.model_dump(mode="json", exclude_none=True),
                          summary=summarize(action, params))


def cited_sources(text: str, hits: list[Hit]) -> list[Source]:
    """Only documents the answer actually cites are returned as sources.

    Models write citations as [1], [1, 2] or [1, обновлено 01.09.2026]: inside brackets we take
    the comma-separated parts that are plain numbers.
    """
    seen: list[int] = []
    for m in CITATION.finditer(text):
        for part in m.group(1).split(","):
            part = part.strip()
            if part.isdigit() and 1 <= int(part) <= len(hits) and int(part) not in seen:
                seen.append(int(part))
    sources: list[Source] = []
    for n in seen:
        doc = hits[n - 1].doc
        if all(s.id != doc.id for s in sources):  # several chunks of one document -> one source
            sources.append(Source(id=doc.id, title=doc.title, url=doc.url, snippet=best_quote(doc.text, text, n)))
    return sources


SENTENCE_END = re.compile(r"(?<=[.!?…])\s+")


def best_quote(chunk: str, answer: str, n: int, limit: int = 300) -> str:
    """The sentence of the chunk that the answer relies on, for "show the quote" in the UI.

    Compare the answer sentences that cite [n] with each chunk sentence by shared stems; the best
    match wins. Falls back to the chunk start when nothing overlaps.
    """
    cites_n = re.compile(rf"\[(?:[^\]]*,\s*)?{n}\s*(?:,[^\]]*)?\]")  # [n], [1, n], [n, note]; not [1n]
    claims = [s for s in SENTENCE_END.split(answer) if cites_n.search(s)] or [answer]
    claim_terms = set(tokenize(" ".join(claims)))
    sentences = [s.strip() for s in SENTENCE_END.split(chunk) if s.strip()]
    best = max(sentences, key=lambda s: len(claim_terms & set(tokenize(s))), default="")
    if not best or not claim_terms & set(tokenize(best)):
        best = chunk
    return best if len(best) <= limit else best[: limit - 1] + "…"


def mask_request(req: AgentChatRequest) -> tuple[AgentChatRequest, Counter[str]]:
    """Personal data never leaves the service: mask it in everything the user typed."""
    total: Counter[str] = Counter()
    message, found = mask(req.message)
    total += found
    history = []
    for turn in req.history:
        if turn.role == "user":
            content, found = mask(turn.content)
            total += found
            turn = turn.model_copy(update={"content": content})
        history.append(turn)
    page = req.page_context
    if page and page.selection:
        selection, found = mask(page.selection)
        total += found
        page = page.model_copy(update={"selection": selection})
    return req.model_copy(update={"message": message, "history": history, "page_context": page}), total


CANDIDATES = 20  # depth of each partial ranking before fusion
# Questions that point at the open page ("что тут смотреть?", "это ещё актуально?", "как здесь
# написано") make the page the topic. Without such words the page is only weak context, so
# topic switches ("Как оформить отпуск?" while a Grafana tab is open) still find their documents.
DEICTIC = re.compile(r"\b(тут|здесь|это|этом|этой|эту|этот|эти|этого|страниц\w*|выделен\w*|написан\w*)\b")
# Elliptical follow-ups ("а за сколько дней?", "и кто согласует?", "а если я стажёр?") have no topic of their own.
FOLLOW_UP = re.compile(r"^\s*(а|и|но|ну|тогда|ещё|еще)\b")


def retrieve(req: AgentChatRequest, retriever: Retriever, k: int, min_score: float,
             use_history: bool = True) -> list[Hit]:
    """Follow-ups like "а за сколько дней?" carry no topic on their own. We search twice - the message
    alone and the message together with the previous user turn - and fuse both rankings (RRF).
    The message-alone ranking keeps topic switches working ("Во сколько стендап?" after a VPN question).
    """
    depth = max(k, CANDIDATES)  # each ranking contributes a deeper pool; the fused list is cut to k
    alone = retriever.search(req.message, depth, min_score)
    rankings: list[list[Hit]] = []
    previous = next((t.content for t in reversed(req.history) if t.role == "user"), "")
    if use_history and previous:
        rankings.append(retriever.search(f"{previous} {req.message}", depth, min_score))
    page = req.page_context
    if page and (page.title or page.selection):
        # "что тут смотреть?" on a Grafana dashboard: the page title / selection carries the topic
        about = " ".join(x for x in (page.title, (page.selection or "")[:300]) if x)
        rankings.append(retriever.search(f"{about} {req.message}", depth, min_score))
        if DEICTIC.search(req.message.casefold()):
            # the question is ABOUT the page: the page is the topic, the bare message is noise
            rankings.append(retriever.search(about, depth, min_score))
            alone = []
    if alone:
        rankings.insert(0, alone)
    hits = rankings[0][:k] if len(rankings) == 1 else fuse(rankings, k)
    if use_history and previous and FOLLOW_UP.match(req.message.casefold()):
        # "а за сколько дней?" continues the previous answer, so its evidence stays in view: the bare
        # follow-up drags in noise ("сроки" of access requests) and fusion alone can push out a document
        # the previous answer relied on (e.g. the second side of a contradiction).
        seen = {(h.doc.id, h.doc.title) for h in hits}
        carried = [h for h in retriever.search(previous, k, min_score) if (h.doc.id, h.doc.title) not in seen]
        hits = hits + carried
    return hits


async def run_turn(req: AgentChatRequest, retriever: Retriever, llm: LLM,
                   settings: Settings, query: str | None = None,
                   limiter: Limiter | None = None, cache: AnswerCache | None = None) -> AsyncIterator[Event]:
    """query overrides what is searched in the KB (used by the brief, whose message is an instruction)."""
    t0 = time.monotonic()
    exceeded = limiter.check(req.user_context.user_id) if limiter else None
    if exceeded:
        metrics.RATE_LIMITED.labels(exceeded).inc()
        message = BUDGET_MESSAGE if exceeded == "daily_tokens" else ERROR_MESSAGES["rate_limited"]
        yield "error", {"code": "rate_limited", "message": message}
        return
    req, masked = mask_request(req)
    for kind, n in masked.items():
        metrics.PII_MASKED.labels(kind).inc(n)
    cache_key = cache.key(req, id(retriever)) if cache and query is None and not masked else None
    if cache_key:
        cached = cache.get(cache_key)
        metrics.CACHE.labels("hit" if cached else "miss").inc()
        if cached:
            yield "delta", {"text": cached.text}
            yield "sources", {"sources": cached.sources}
            yield "done", {"finish_reason": "answered", "mode": settings.mode, "cached": True,
                           "latency_ms": int((time.monotonic() - t0) * 1000),
                           "usage": {"input_tokens": 0, "output_tokens": 0, "cached_input_tokens": 0}}
            return
    yield "status", {"stage": "retrieving"}
    search_req = req if query is None else req.model_copy(update={"message": query, "history": []})
    hits = retrieve(search_req, retriever, settings.top_k, settings.min_score)
    allowed = set(req.user_context.allowed_actions)
    turn = Turn(req, hits, build_messages(req, hits), build_tools(sorted(allowed), req.user_context.role))

    yield "status", {"stage": "generating"}
    parts: list[str] = []
    calls: list[ToolCall] = []
    usage = Usage()
    try:
        async for ev in llm.stream(turn):
            if isinstance(ev, TextDelta):
                if not parts:
                    metrics.TTFT.observe(time.monotonic() - t0)
                parts.append(ev.text)
                yield "delta", {"text": ev.text}
            elif isinstance(ev, ToolCall):
                calls.append(ev)
            else:
                usage = ev
    except LLMError as e:
        log.warning("llm error code=%s detail=%s latency_ms=%d", e.code, e, (time.monotonic() - t0) * 1000)
        metrics.ERRORS.labels(e.code).inc()
        yield "error", {"code": e.code, "message": ERROR_MESSAGES[e.code]}
        return
    except Exception:  # never leak internals into the stream
        log.exception("agent turn failed")
        metrics.ERRORS.labels("internal").inc()
        yield "error", {"code": "internal", "message": ERROR_MESSAGES["internal"]}
        return

    action_calls = [c for c in calls if c.name != "show_form"]
    proposal = next((p for p in (to_proposal(c, allowed, req.user_context.role) for c in action_calls) if p), None)
    # the model could not create anything yet (details missing): the chat shows a form, the user submits it
    forms = allowed_forms(allowed)
    form = None if proposal else next((f for f in (to_form(c, forms, req.user_context.role) for c in calls) if f), None)
    text = "".join(parts)
    if proposal:
        yield "status", {"stage": "preparing_action"}
    if not text.strip():
        text = FALLBACK_TEXT[proposal.type] if proposal else (FORM_TEXT if form else EMPTY_ANSWER)
        yield "delta", {"text": text}

    sources = cited_sources(text, hits)
    if sources:
        yield "sources", {"sources": [s.model_dump(exclude_none=True) for s in sources]}
    if proposal:
        yield "action", {"proposal": proposal.model_dump(mode="json")}
    if form:
        yield "form", {"form": form}

    if form:
        finish = "form_shown"
    elif proposal and proposal.type != "mentor_question":
        finish = "action_proposed"
    elif proposal and not sources:
        finish = "no_answer"  # nothing grounded to say, escalation offered
    else:
        finish = "answered"  # grounded in KB or in the user's own context (possibly + escalation offer)
    latency_ms = int((time.monotonic() - t0) * 1000)
    metrics.TURNS.labels(finish).inc()
    if cache_key and finish == "answered" and not proposal:
        cache.put(cache_key, req, CachedAnswer(text, [s.model_dump(exclude_none=True) for s in sources]))
    if limiter:
        limiter.spend(usage.input_tokens + usage.output_tokens)
    metrics.LATENCY.observe(latency_ms / 1000)
    for kind, n in (("input", usage.input_tokens), ("output", usage.output_tokens),
                    ("cached_input", usage.cached_input_tokens)):
        metrics.TOKENS.labels(kind).inc(n)
    if proposal:
        metrics.ACTIONS.labels(proposal.type).inc()
    # metadata only: no message text or PII in logs
    log.info("turn finish=%s hits=%d tool_calls=%d action=%s latency_ms=%d in=%d cached=%d out=%d pii_masked=%d",
             finish, len(hits), len(calls), proposal.type if proposal else "-", latency_ms,
             usage.input_tokens, usage.cached_input_tokens, usage.output_tokens, sum(masked.values()))
    yield "done", {"finish_reason": finish, "mode": settings.mode, "latency_ms": latency_ms,
                   "usage": {"input_tokens": usage.input_tokens, "output_tokens": usage.output_tokens,
                             "cached_input_tokens": usage.cached_input_tokens}}


def sse(event: str, data: dict[str, Any]) -> str:
    return f"event: {event}\ndata: {json.dumps(data, ensure_ascii=False)}\n\n"

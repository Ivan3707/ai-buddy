"""LLM providers behind one tiny interface: `stream(turn)` yields TextDelta / ToolCall / Usage.

- OpenAICompatibleLLM: any Chat Completions API (OpenAI, OpenRouter, Gemini, vLLM, Ollama, corporate gateway).
- MockLLM: deterministic, offline; used by tests, CI and runs without a key.
"""
from __future__ import annotations

import json
import time
from collections.abc import AsyncIterator
import logging
from dataclasses import dataclass, field, replace
from datetime import date, timedelta
from typing import Any, Protocol

import httpx

log = logging.getLogger("ai.llm")

from .config import Settings
from .metrics import FALLBACK
from .retrieval import Hit
from .schemas import AgentChatRequest


@dataclass(frozen=True)
class TextDelta:
    text: str


@dataclass(frozen=True)
class ToolCall:
    name: str
    arguments: str  # raw JSON string as produced by the model; validated later


@dataclass(frozen=True)
class Usage:
    input_tokens: int = 0
    output_tokens: int = 0
    cached_input_tokens: int = 0  # prompt prefix served from the provider's cache (cheaper, faster)


LLMEvent = TextDelta | ToolCall | Usage


class LLMError(Exception):
    def __init__(self, code: str, detail: str = ""):
        super().__init__(f"{code}: {detail}" if detail else code)
        self.code = code  # llm_unavailable | timeout (ErrorEvent.code)


@dataclass
class Turn:
    request: AgentChatRequest
    hits: list[Hit]
    messages: list[dict[str, str]]
    tools: list[dict[str, Any]] = field(default_factory=list)


class LLM(Protocol):
    def stream(self, turn: Turn) -> AsyncIterator[LLMEvent]: ...


class OpenAICompatibleLLM:
    def __init__(self, settings: Settings, transport: httpx.AsyncBaseTransport | None = None):
        self.s = settings
        self.transport = transport

    def _body(self, turn: Turn) -> dict[str, Any]:
        body: dict[str, Any] = {
            "model": self.s.model,
            "messages": turn.messages,
            "stream": True,
            "stream_options": {"include_usage": True},
            "max_tokens": self.s.max_output_tokens,
        }
        if turn.tools:
            body["tools"] = turn.tools
        body.update(self.s.extra_body)  # provider-specific knobs, e.g. {"reasoning": {"effort": "low"}}
        if self.s.temperature is not None:  # some reasoning models reject temperature, so it is opt-in
            body["temperature"] = self.s.temperature
        return body

    async def stream(self, turn: Turn) -> AsyncIterator[LLMEvent]:
        # read timeout = max silence between chunks, which bounds time-to-first-byte;
        # the overall deadline is checked per chunk.
        timeout = httpx.Timeout(10.0, read=self.s.first_byte_timeout_s)
        deadline = time.monotonic() + self.s.timeout_s
        headers = {"Authorization": f"Bearer {self.s.api_key}"}
        calls: dict[int, dict[str, str]] = {}
        try:
            async with httpx.AsyncClient(transport=self.transport, timeout=timeout) as client:
                async with client.stream("POST", f"{self.s.base_url}/chat/completions",
                                         json=self._body(turn), headers=headers) as resp:
                    if resp.status_code >= 400:
                        await resp.aread()
                        raise LLMError("llm_unavailable", f"provider HTTP {resp.status_code}")
                    async for line in resp.aiter_lines():
                        if time.monotonic() > deadline:
                            raise LLMError("timeout", "total deadline exceeded")
                        if not line.startswith("data:"):
                            continue  # blank separators and ": keep-alive" comments
                        data = line[5:].strip()
                        if data == "[DONE]":
                            break
                        chunk = json.loads(data)
                        if "error" in chunk:
                            raise LLMError("llm_unavailable", "provider error in stream")
                        if chunk.get("usage"):
                            u = chunk["usage"]
                            cached = (u.get("prompt_tokens_details") or {}).get("cached_tokens") or 0
                            yield Usage(u.get("prompt_tokens", 0), u.get("completion_tokens", 0), cached)
                        for choice in chunk.get("choices") or []:
                            delta = choice.get("delta") or {}
                            if delta.get("content"):
                                yield TextDelta(delta["content"])
                            # tool calls arrive in fragments: name first, then arguments piece by piece
                            for tc in delta.get("tool_calls") or []:
                                acc = calls.setdefault(tc.get("index", 0), {"name": "", "arguments": ""})
                                fn = tc.get("function") or {}
                                acc["name"] += fn.get("name") or ""
                                acc["arguments"] += fn.get("arguments") or ""
        except httpx.TimeoutException as e:
            raise LLMError("timeout", type(e).__name__) from e
        except httpx.HTTPError as e:
            raise LLMError("llm_unavailable", type(e).__name__) from e
        except json.JSONDecodeError as e:
            raise LLMError("llm_unavailable", "malformed stream chunk") from e
        for i in sorted(calls):
            yield ToolCall(calls[i]["name"], calls[i]["arguments"])


ACTION_WORDS = ("оформ", "заявк", "создай", "подай", "запроси")
SYSTEMS = ("jira", "confluence", "gitlab", "grafana", "redash", "kubernetes")
# The mock does not understand language, so a weak keyword match is treated as "not found".
# BM25 top-1 scores on eval: answerable median ~6.4, out-of-KB max ~4.6 (they overlap; the LLM decides better).
MOCK_MIN_SCORE = 3.0  # compared with Hit.lexical (BM25), also in hybrid mode


class MockLLM:
    """Deterministic stand-in: keyword rules, no network. Good enough to exercise every stream branch."""

    async def stream(self, turn: Turn) -> AsyncIterator[LLMEvent]:
        msg = turn.request.message.casefold()
        tools = {t["function"]["name"] for t in turn.tools}
        wants_action = any(w in msg for w in ACTION_WORDS) and not msg.lstrip().startswith("как")
        today = turn.request.user_context.today or date.today()

        if wants_action and "propose_leave_request" in tools and ("отгул" in msg or "отпуск" in msg):
            day = (today + timedelta(days=7)).isoformat()
            kind = "vacation" if "отпуск" in msg else "day_off"
            yield ToolCall("propose_leave_request", json.dumps({"kind": kind, "date_from": day, "date_to": day}))
        elif wants_action and "propose_access_request" in tools and "доступ" in msg:
            system = next((s for s in SYSTEMS if s in msg), "jira")
            yield ToolCall("propose_access_request", json.dumps({
                "system": system, "resource": "demo", "access_level": "read",
                "justification": "Нужно для задач онбординга (демо)"}, ensure_ascii=False))
        elif confident := [h for h in turn.hits[:2] if h.lexical >= MOCK_MIN_SCORE]:
            parts = [f"**{h.doc.title}**: {h.doc.text} [{n}]" for n, h in enumerate(confident, 1)]
            text = "Вот что нашёл в базе знаний (демо-режим без LLM):\n\n" + "\n\n".join(parts)
            for word in text.split(" "):
                yield TextDelta(word + " ")
        else:
            yield TextDelta("В базе знаний этого нет. Могу передать вопрос наставнику.")
            if "propose_mentor_question" in tools:
                yield ToolCall("propose_mentor_question",
                               json.dumps({"question": turn.request.message}, ensure_ascii=False))
        yield Usage()


class FallbackLLM:
    """Primary model, and a fallback (e.g. local Ollama) if the primary fails before saying anything.

    A failure in the middle of an answer is not retried: gluing two different answers together would
    be worse than an honest error the user can retry.
    """

    def __init__(self, primary: LLM, fallback: LLM, fallback_name: str):
        self.primary, self.fallback, self.fallback_name = primary, fallback, fallback_name

    async def stream(self, turn: Turn) -> AsyncIterator[LLMEvent]:
        started = False
        try:
            async for ev in self.primary.stream(turn):
                started = started or not isinstance(ev, Usage)
                yield ev
            return
        except LLMError as e:
            if started:
                raise
            log.warning("primary LLM failed (%s), using fallback model=%s", e.code, self.fallback_name)
            FALLBACK.inc()
        async for ev in self.fallback.stream(turn):
            yield ev


def make_llm(settings: Settings) -> LLM:
    if settings.provider != "openai_compatible":
        return MockLLM()
    primary = OpenAICompatibleLLM(settings)
    if not (settings.fallback_base_url and settings.fallback_model):
        return primary
    fallback = OpenAICompatibleLLM(replace(settings, base_url=settings.fallback_base_url,
                                           api_key=settings.fallback_api_key or "none",
                                           model=settings.fallback_model, extra_body=settings.fallback_extra_body))
    return FallbackLLM(primary, fallback, settings.fallback_model)

"""Non-streaming use of the streaming LLM interface: collect the whole answer, optionally parse JSON.

The chat streams tokens to the user; back-office features (mentor draft, gap labels, KB lint) need
the complete text at once, so they reuse the same adapters through this tiny helper.
"""
from __future__ import annotations

import json
import re
from typing import Any

from .llm import LLM, MockLLM, TextDelta, Turn, Usage
from .schemas import AgentChatRequest, UserContext

JSON_BLOCK = re.compile(r"\{.*\}", re.DOTALL)


def is_mock(llm: LLM) -> bool:
    """MockLLM understands only chat keywords; callers switch to their own deterministic logic."""
    return isinstance(llm, MockLLM)


def _turn(messages: list[dict[str, str]]) -> Turn:
    # Turn requires a chat request; adapters use only `messages` here. model_construct skips validation:
    # there is no real user behind a back-office call.
    ctx = UserContext.model_construct(user_id="system", role="buddy", tasks=[], open_requests=[], allowed_actions=[])
    req = AgentChatRequest.model_construct(message=messages[-1]["content"], history=[], user_context=ctx)
    return Turn(req, [], messages)


async def complete(llm: LLM, messages: list[dict[str, str]]) -> tuple[str, Usage]:
    """Run one non-streaming completion. LLMError propagates to the caller."""
    parts: list[str] = []
    usage = Usage()
    async for ev in llm.stream(_turn(messages)):
        if isinstance(ev, TextDelta):
            parts.append(ev.text)
        elif isinstance(ev, Usage):
            usage = ev
    return "".join(parts).strip(), usage


def parse_json_object(text: str) -> dict[str, Any] | None:
    """Models often wrap JSON in ```json fences or add a sentence around it: take the outer {...}."""
    m = JSON_BLOCK.search(text)
    if not m:
        return None
    try:
        value = json.loads(m.group(0))
    except ValueError:
        return None
    return value if isinstance(value, dict) else None

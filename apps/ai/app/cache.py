"""Answer cache for frequent KB questions (C5).

Answers are personal (stage, tasks, requests, buddy), so only the safe subset is cached: a
standalone question (no history, no page, no masked PII) answered purely from the KB, with no
action card and no trace of the user's own context in the text. The key holds the normalised
question, role, profession and the KB index identity, so a KB reload never serves stale answers.
"""
from __future__ import annotations

import re
import threading
import time
from collections import OrderedDict
from collections.abc import Callable
from dataclasses import dataclass
from typing import Any

from .schemas import AgentChatRequest

PUNCT = re.compile(r"[^\w\s]")


def normalize(text: str) -> str:
    return " ".join(PUNCT.sub(" ", text.casefold().replace("ё", "е")).split())


@dataclass(frozen=True)
class CachedAnswer:
    text: str
    sources: list[dict[str, Any]]


def personal_markers(req: AgentChatRequest) -> list[str]:
    ctx = req.user_context
    markers = [ctx.display_name, ctx.buddy_name, *(t.title for t in ctx.tasks), *(r.number for r in ctx.open_requests),
               *(i.name for i in ctx.my_interns or []), *(q.intern for q in ctx.open_mentor_questions or []),
               *(a.number for a in ctx.pending_approvals or [])]
    return [m for m in markers if m and len(m) > 2]


class AnswerCache:
    def __init__(self, ttl_s: float, max_size: int = 500, clock: Callable[[], float] = time.monotonic):
        self.ttl_s, self.max_size, self.clock = ttl_s, max_size, clock
        self._items: OrderedDict[tuple, tuple[float, CachedAnswer]] = OrderedDict()
        self._lock = threading.Lock()

    @staticmethod
    def key(req: AgentChatRequest, index_id: int) -> tuple | None:
        """None when the request is not cacheable at all."""
        if req.history or req.page_context:
            return None
        return normalize(req.message), req.user_context.role, req.user_context.profession, index_id

    def get(self, key: tuple) -> CachedAnswer | None:
        with self._lock:
            item = self._items.get(key)
            if item is None or self.clock() - item[0] > self.ttl_s:
                self._items.pop(key, None)
                return None
            self._items.move_to_end(key)
            return item[1]

    def put(self, key: tuple, req: AgentChatRequest, answer: CachedAnswer) -> bool:
        text = answer.text.casefold()
        if not answer.sources or any(m.casefold() in text for m in personal_markers(req)):
            return False
        with self._lock:
            self._items[key] = (self.clock(), answer)
            self._items.move_to_end(key)
            while len(self._items) > self.max_size:
                self._items.popitem(last=False)
        return True

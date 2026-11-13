"""Usage limits (C3): per-user request rate and a daily token budget for the whole service.

In-memory and per process: enough for one AI container in the demo. With several replicas the same
counters would move to Redis (already in Avito's stack) without changing the call sites.
"""
from __future__ import annotations

import threading
import time
from collections import defaultdict, deque
from collections.abc import Callable
from datetime import datetime, timezone


class Limiter:
    def __init__(self, per_minute: int = 0, daily_tokens: int = 0,
                 clock: Callable[[], float] = time.time) -> None:
        self.per_minute, self.daily_tokens, self.clock = per_minute, daily_tokens, clock
        self._hits: dict[str, deque[float]] = defaultdict(deque)
        self._day, self._tokens = "", 0
        self._lock = threading.Lock()

    def _today(self) -> str:
        return datetime.fromtimestamp(self.clock(), timezone.utc).strftime("%Y-%m-%d")

    def check(self, user_id: str) -> str | None:
        """Returns the name of the exceeded limit, or None and records the request."""
        with self._lock:
            now = self.clock()
            if self.daily_tokens:
                if self._day != self._today():
                    self._day, self._tokens = self._today(), 0
                if self._tokens >= self.daily_tokens:
                    return "daily_tokens"
            if self.per_minute:
                hits = self._hits[user_id]
                while hits and now - hits[0] >= 60:
                    hits.popleft()
                if len(hits) >= self.per_minute:
                    return "per_minute"
                hits.append(now)
            return None

    def spend(self, tokens: int) -> None:
        with self._lock:
            if self._day != self._today():
                self._day, self._tokens = self._today(), 0
            self._tokens += tokens

"""Prometheus metrics of the AI service (GET /metrics). Metadata only: no message text, no PII."""
from __future__ import annotations

from prometheus_client import CollectorRegistry, Counter, Histogram

REGISTRY = CollectorRegistry()  # own registry: no process/platform noise, isolated in tests

TURNS = Counter("ai_turns", "Completed agent turns", ["finish_reason"], registry=REGISTRY)
ERRORS = Counter("ai_errors", "Turns ended with an error event", ["code"], registry=REGISTRY)
TOKENS = Counter("ai_llm_tokens", "LLM tokens", ["kind"], registry=REGISTRY)  # input | output | cached_input
ACTIONS = Counter("ai_action_proposals", "Action cards proposed to the user", ["type"], registry=REGISTRY)
DROPPED_TOOL_CALLS = Counter("ai_dropped_tool_calls", "Tool calls rejected by validation", ["reason"],
                             registry=REGISTRY)
PII_MASKED = Counter("ai_pii_masked", "Personal data fragments masked before the LLM", ["kind"], registry=REGISTRY)
FALLBACK = Counter("ai_llm_fallback", "Turns answered by the fallback LLM", registry=REGISTRY)
RATE_LIMITED = Counter("ai_rate_limited", "Requests rejected by limits", ["limit"], registry=REGISTRY)
CACHE = Counter("ai_answer_cache", "Answer cache lookups", ["result"], registry=REGISTRY)  # hit | miss

_SECONDS = (0.25, 0.5, 1, 2, 3, 5, 8, 13, 20, 40, 60)
LATENCY = Histogram("ai_turn_latency_seconds", "Full agent turn latency", buckets=_SECONDS, registry=REGISTRY)
TTFT = Histogram("ai_time_to_first_token_seconds", "Time to the first answer text", buckets=_SECONDS,
                 registry=REGISTRY)

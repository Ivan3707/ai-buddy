"""Admin panel endpoints (internal, called only by Backend for the teamlead role).

The LLM key never leaves this service: responses carry only normalized numbers and human notes.
"""
from __future__ import annotations

import logging
import time
from typing import Any
from urllib.parse import urlparse

import httpx
from fastapi import APIRouter, Request

from .config import Settings
from .kb_docs import indexed_docs

log = logging.getLogger("ai.admin")
router = APIRouter()

PING_PROMPT = "Ответь одним словом: pong"
BALANCE_TIMEOUT_S = 8.0


def _is_openrouter(base_url: str) -> bool:
    return "openrouter.ai" in (urlparse(base_url).hostname or "")


def _num(value: Any) -> float | None:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return None
    return float(value)


def _provider_name(settings: Settings) -> str:
    if settings.provider != "openai_compatible":
        return "mock"
    host = urlparse(settings.base_url).hostname or ""
    return "openrouter" if _is_openrouter(settings.base_url) else (host or "openai_compatible")


def _mask_label(label: str) -> str:
    """OpenRouter labels can be a key prefix like 'sk-or-v1-abc...xyz'; show only a short non-secret form."""
    return label[:6] + "…" if label.startswith("sk-") else label[:60]


async def fetch_balance(settings: Settings, transport: httpx.AsyncBaseTransport | None = None) -> dict[str, Any]:
    """Normalized balance. Never raises, never includes the key or a raw provider error body."""
    out: dict[str, Any] = {"available": False, "currency": "USD", "usage": None, "limit": None,
                           "limit_remaining": None, "usage_daily": None, "usage_weekly": None,
                           "usage_monthly": None, "is_free_tier": None, "label": None, "note": ""}
    if settings.provider != "openai_compatible":
        out["note"] = "Режим mock: внешний LLM не используется, баланса нет."
        return out
    if not _is_openrouter(settings.base_url):
        out["note"] = "Провайдер не отдаёт баланс по API: смотрите кабинет провайдера."
        return out
    try:
        async with httpx.AsyncClient(transport=transport, timeout=BALANCE_TIMEOUT_S) as client:
            resp = await client.get(f"{settings.base_url}/key", headers={"Authorization": f"Bearer {settings.api_key}"})
        if resp.status_code != 200:
            out["note"] = f"Не удалось получить баланс: провайдер ответил HTTP {resp.status_code}."
            return out
        data = (resp.json() or {}).get("data") or {}
    except (httpx.HTTPError, ValueError, AttributeError) as e:
        out["note"] = f"Не удалось получить баланс: {type(e).__name__}."
        return out
    label = data.get("label")
    flag = data.get("is_free_tier")
    out.update(available=True, usage=_num(data.get("usage")), limit=_num(data.get("limit")),
               limit_remaining=_num(data.get("limit_remaining")), usage_daily=_num(data.get("usage_daily")),
               usage_weekly=_num(data.get("usage_weekly")), usage_monthly=_num(data.get("usage_monthly")),
               is_free_tier=flag if isinstance(flag, bool) else None,
               label=_mask_label(label) if isinstance(label, str) else None)
    if out["limit"] is None:
        out["note"] = "У ключа нет лимита по расходам."
    return out


@router.get("/v1/admin/llm")
async def admin_llm(request: Request):
    settings: Settings = request.app.state.settings
    limiter = getattr(request.app.state, "limiter", None)
    return {
        "provider": _provider_name(settings),
        "mode": settings.mode,
        "model": settings.model if settings.mode == "llm" else "",
        "fallback_model": settings.fallback_model,
        "balance": await fetch_balance(settings, getattr(request.app.state, "admin_transport", None)),
        "budget": limiter.budget_status() if limiter else None,
    }


async def run_ping(settings: Settings, transport: httpx.AsyncBaseTransport | None = None) -> dict[str, Any]:
    if settings.provider != "openai_compatible":
        return {"ok": True, "mode": "mock", "model": "", "latency_ms": 0, "answer": "pong (mock)",
                "input_tokens": 0, "output_tokens": 0, "error": ""}
    body: dict[str, Any] = {"model": settings.model, "messages": [{"role": "user", "content": PING_PROMPT}],
                            "max_tokens": 16}
    body.update(settings.extra_body)
    if settings.temperature is not None:
        body["temperature"] = settings.temperature
    t0 = time.monotonic()
    result: dict[str, Any] = {"ok": False, "mode": "llm", "model": settings.model, "latency_ms": 0, "answer": "",
                              "input_tokens": 0, "output_tokens": 0, "error": ""}
    try:
        async with httpx.AsyncClient(transport=transport, timeout=httpx.Timeout(10.0, read=30.0)) as client:
            resp = await client.post(f"{settings.base_url}/chat/completions", json=body,
                                     headers={"Authorization": f"Bearer {settings.api_key}"})
        result["latency_ms"] = int((time.monotonic() - t0) * 1000)
        if resp.status_code >= 400:
            result["error"] = f"Провайдер ответил HTTP {resp.status_code}."
            return result
        data = resp.json()
        message = ((data.get("choices") or [{}])[0].get("message")) or {}
        usage = data.get("usage") or {}
        result.update(ok=True, answer=str(message.get("content") or "").strip()[:200],
                      input_tokens=int(usage.get("prompt_tokens") or 0),
                      output_tokens=int(usage.get("completion_tokens") or 0))
    except httpx.TimeoutException:
        result.update(latency_ms=int((time.monotonic() - t0) * 1000), error="Таймаут ответа модели.")
    except (httpx.HTTPError, ValueError, AttributeError, TypeError) as e:
        result.update(latency_ms=int((time.monotonic() - t0) * 1000), error=f"Ошибка запроса: {type(e).__name__}.")
    log.info("admin llm ping ok=%s latency_ms=%d", result["ok"], result["latency_ms"])
    return result


@router.post("/v1/admin/llm-ping")
async def admin_llm_ping(request: Request):
    return await run_ping(request.app.state.settings, getattr(request.app.state, "admin_transport", None))


@router.get("/v1/admin/kb")
def admin_kb(request: Request):
    docs = indexed_docs(request.app.state.retriever)
    source = getattr(request.app.state, "kb_source", None)
    return {"documents": len({d.id for d in docs}), "chunks": len(docs), "source": getattr(source, "name", "")}

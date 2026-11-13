"""Runtime settings from environment variables (see .env.example)."""
from __future__ import annotations

import json
import os
from collections.abc import Mapping
from dataclasses import dataclass, field
from typing import Any

PROVIDERS = ("mock", "openai_compatible")


@dataclass(frozen=True)
class Settings:
    provider: str = "mock"
    base_url: str = ""
    api_key: str = ""
    model: str = ""
    timeout_s: float = 60.0
    first_byte_timeout_s: float = 15.0
    max_output_tokens: int = 1024
    temperature: float | None = None
    extra_body: dict[str, Any] = field(default_factory=dict)
    rag_mode: str = "hybrid"
    embedding_model: str = "intfloat/multilingual-e5-small"
    # Optional second OpenAI-compatible endpoint (e.g. local Ollama) used when the primary fails
    # before producing any output.
    fallback_base_url: str = ""
    fallback_api_key: str = ""
    fallback_model: str = ""
    fallback_extra_body: dict[str, Any] = field(default_factory=dict)
    top_k: int = 4
    min_score: float = 0.2

    @property
    def mode(self) -> str:
        """Value of AiMode in the contract."""
        return "llm" if self.provider == "openai_compatible" else "mock"


def load_settings(env: Mapping[str, str] = os.environ) -> Settings:
    def get(name: str, default: str = "") -> str:
        return (env.get(name) or default).strip()

    provider = get("LLM_PROVIDER", "mock")
    if provider not in PROVIDERS:
        raise ValueError(f"LLM_PROVIDER must be one of {PROVIDERS}, got {provider!r}")
    temperature = get("LLM_TEMPERATURE")
    settings = Settings(
        provider=provider,
        base_url=get("LLM_BASE_URL").rstrip("/"),
        api_key=get("LLM_API_KEY"),
        model=get("LLM_MODEL"),
        timeout_s=float(get("LLM_TIMEOUT_SECONDS", "60")),
        first_byte_timeout_s=float(get("LLM_FIRST_BYTE_TIMEOUT_SECONDS", "15")),
        max_output_tokens=int(get("LLM_MAX_OUTPUT_TOKENS", "1024")),
        temperature=float(temperature) if temperature else None,
        extra_body=json.loads(get("LLM_EXTRA_BODY") or "{}"),
        fallback_base_url=get("LLM_FALLBACK_BASE_URL").rstrip("/"),
        fallback_api_key=get("LLM_FALLBACK_API_KEY"),
        fallback_model=get("LLM_FALLBACK_MODEL"),
        fallback_extra_body=json.loads(get("LLM_FALLBACK_EXTRA_BODY") or "{}"),
        rag_mode=get("RAG_MODE", "hybrid"),
        embedding_model=get("RAG_EMBEDDING_MODEL", "intfloat/multilingual-e5-small"),
        top_k=int(get("RAG_TOP_K", "4")),
        min_score=float(get("RAG_MIN_SCORE", "0.2")),
    )
    if settings.rag_mode not in ("bm25", "hybrid"):
        raise ValueError(f"RAG_MODE must be bm25 or hybrid, got {settings.rag_mode!r}")
    if provider == "openai_compatible":
        missing = [n for n, v in (("LLM_BASE_URL", settings.base_url), ("LLM_API_KEY", settings.api_key),
                                  ("LLM_MODEL", settings.model)) if not v]
        if missing:
            raise ValueError(f"LLM_PROVIDER=openai_compatible requires {', '.join(missing)}")
    return settings

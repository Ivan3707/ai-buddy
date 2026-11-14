"""AI service: internal to the Docker network, called only by Backend."""
from __future__ import annotations

import logging
import os
from collections.abc import AsyncIterator
from pathlib import Path

from fastapi import FastAPI
from fastapi.responses import Response, StreamingResponse
from prometheus_client import CONTENT_TYPE_LATEST, generate_latest

from . import gaps, hint, kb_docs, kb_lint, mentor, sources
from . import metrics
from .agent import run_turn, sse
from .brief import run_brief
from .config import Settings, load_settings
from .cache import AnswerCache
from .limits import Limiter
from .llm import LLM, make_llm
from .retrieval import BM25Retriever, Retriever, build_retriever, load_kb
from .schemas import AgentChatRequest, BriefRequest, ChatRequest, ChatResponse, Source

logging.basicConfig(level=os.getenv("LOG_LEVEL", "INFO"), format="%(asctime)s %(levelname)s %(name)s %(message)s")
DATA = Path(os.getenv("DATA_DIR") or (Path(__file__).resolve().parents[3] / "data"))
NO_ANSWER = "В демо-базе нет ответа. Попробуйте спросить про отпуск или доступ к сервисам."


def create_app(settings: Settings | None = None, retriever: Retriever | None = None,
               llm: LLM | None = None) -> FastAPI:
    settings = settings or load_settings()
    source = sources.make_source(os.environ, DATA / "kb")
    retriever = retriever or build_retriever(source.load(), settings.rag_mode, settings.embedding_model)
    # Legacy /chat keeps the old two-document KB: current web/extension e2e tests assert on it.
    legacy = BM25Retriever(load_kb(DATA / "kb.json"))
    llm = llm or make_llm(settings)
    app = FastAPI(title="AI-buddy AI", version="0.2.0")
    app.state.settings, app.state.retriever, app.state.llm = settings, retriever, llm
    limiter = Limiter(settings.rate_limit_per_minute, settings.daily_token_budget)
    cache = AnswerCache(settings.answer_cache_ttl_s) if settings.answer_cache_ttl_s > 0 else None
    app.state.kb_source = source
    app.include_router(mentor.router)
    app.include_router(gaps.router)
    app.include_router(kb_lint.router)
    app.include_router(sources.router)
    app.include_router(hint.router)
    app.include_router(kb_docs.router)

    @app.get("/metrics")
    def prometheus_metrics() -> Response:
        return Response(generate_latest(metrics.REGISTRY), media_type=CONTENT_TYPE_LATEST)

    @app.get("/health")
    def health():
        body = {"status": "ok", "mode": settings.mode}
        if settings.mode == "llm":
            body["model"] = settings.model
            if settings.fallback_model:
                body["fallback_model"] = settings.fallback_model
        return body

    @app.post("/chat", response_model=ChatResponse, deprecated=True)
    def chat(body: ChatRequest):
        """Legacy non-streaming endpoint: retrieval only, no LLM. Kept until clients move to /v1/chat/stream."""
        hits = legacy.search(body.message, 1, settings.min_score)
        if not hits:
            return ChatResponse(answer=NO_ANSWER, sources=[])
        d = hits[0].doc
        return ChatResponse(answer=d.text, sources=[Source(id=d.id, title=d.title, url=d.url)])

    @app.post("/v1/brief/stream")
    async def brief_stream(body: BriefRequest):
        async def frames() -> AsyncIterator[str]:
            async for event, data in run_brief(body, app.state.retriever, llm, settings, limiter):
                yield sse(event, data)
        return StreamingResponse(frames(), media_type="text/event-stream",
                                 headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"})

    @app.post("/v1/chat/stream")
    async def chat_stream(body: AgentChatRequest):
        async def frames() -> AsyncIterator[str]:
            async for event, data in run_turn(body, app.state.retriever, llm, settings, limiter=limiter,
                                              cache=cache):
                yield sse(event, data)
        return StreamingResponse(frames(), media_type="text/event-stream",
                                 headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"})

    return app


app = create_app()

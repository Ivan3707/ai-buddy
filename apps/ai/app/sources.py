"""C1: where the knowledge base comes from, and hot reload of the search index.

- KB_SOURCE=markdown (default): data/kb/*.md, one '## ' section = one chunk (retrieval.load_markdown_kb).
- KB_SOURCE=confluence: pages of one space via Confluence REST API v1, split into chunks by <h2>,
  the same way. A company plugs in its real wiki without changing retrieval or prompts.
POST /v1/kb/reload re-reads the source and swaps app.state.retriever; KB_RELOAD_SECONDS > 0 does it periodically.
The embedding model is reused on reload: only the chunk vectors are recomputed.
"""
from __future__ import annotations

import asyncio
import logging
import os
import re
import time
from collections.abc import AsyncIterator, Mapping
from contextlib import asynccontextmanager
from dataclasses import dataclass
from html.parser import HTMLParser
from pathlib import Path
from typing import Any, Protocol

import httpx
from fastapi import APIRouter, FastAPI, Request
from fastapi.responses import JSONResponse

from .retrieval import BM25Retriever, DenseRetriever, Doc, HybridRetriever, Retriever, load_markdown_kb

log = logging.getLogger("ai.sources")
MAX_PAGES_REQUESTS = 200  # pagination guard: 200 * 50 = 10k pages


class SourceError(Exception):
    """The KB source could not be read; the current index stays in place."""


class KbSource(Protocol):
    name: str

    def load(self) -> list[Doc]: ...


@dataclass(frozen=True)
class MarkdownSource:
    directory: Path
    name: str = "markdown"
    wiki_url: str = ""

    def load(self) -> list[Doc]:
        return load_markdown_kb(self.directory, self.wiki_url)


# ---- Confluence storage format (XHTML) -> plain-text sections ----

class _StorageParser(HTMLParser):
    BLOCK = {"p", "div", "br", "li", "tr", "h1", "h3", "h4", "h5", "h6", "table", "ul", "ol", "pre", "blockquote"}
    SKIP = {"ac:parameter", "style", "script"}  # macro parameters are config, not content

    def __init__(self) -> None:
        super().__init__(convert_charrefs=True)
        self.sections: list[tuple[str, list[str]]] = [("", [])]
        self.heading: list[str] | None = None
        self.skip = 0

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        if tag in self.SKIP:
            self.skip += 1
        elif tag == "h2":
            self.heading = []
        elif tag in self.BLOCK:
            self.sections[-1][1].append("\n- " if tag == "li" else "\n")

    def handle_endtag(self, tag: str) -> None:
        if tag in self.SKIP:
            self.skip = max(0, self.skip - 1)
        elif tag == "h2" and self.heading is not None:
            self.sections.append((" ".join("".join(self.heading).split()), []))
            self.heading = None
        elif tag in self.BLOCK:
            self.sections[-1][1].append("\n")

    def handle_data(self, data: str) -> None:
        if self.skip:
            return
        (self.heading if self.heading is not None else self.sections[-1][1]).append(data)

    def unknown_decl(self, data: str) -> None:  # <![CDATA[...]]> in code / plain-text macros
        if data.startswith("CDATA["):
            self.handle_data(data[6:])


def storage_to_sections(storage: str) -> list[tuple[str, str]]:
    """[(h2 heading or '', text)] with empty sections dropped and whitespace normalised."""
    parser = _StorageParser()
    parser.feed(storage)
    parser.close()
    out = []
    for heading, parts in parser.sections:
        lines = (re.sub(r"[ \t\xa0]+", " ", line).strip() for line in "".join(parts).splitlines())
        text = "\n".join(line for line in lines if line and line != "-")
        if text:
            out.append((heading, text))
    return out


def page_to_docs(page: Mapping[str, Any], base_url: str) -> list[Doc]:
    title = str(page.get("title") or "").strip()
    storage = ((page.get("body") or {}).get("storage") or {}).get("value") or ""
    updated = str((page.get("version") or {}).get("when") or "")[:10]
    webui = (page.get("_links") or {}).get("webui") or ""
    url = webui if webui.startswith("http") else base_url + webui
    doc_id = f"confluence-{page.get('id')}"
    return [Doc(id=doc_id, title=f"{title} — {heading}" if heading else title, url=url, text=text, updated=updated)
            for heading, text in storage_to_sections(storage)]


@dataclass(frozen=True)
class ConfluenceSource:
    """Confluence Server / Data Center REST API v1 with a personal access token (Bearer)."""
    base_url: str
    token: str
    space: str
    timeout_s: float = 30.0
    page_size: int = 50
    transport: httpx.BaseTransport | None = None
    name: str = "confluence"

    def load(self) -> list[Doc]:
        base = self.base_url.rstrip("/")
        url: str | None = f"{base}/rest/api/content"
        params: dict[str, Any] | None = {"spaceKey": self.space, "type": "page",
                                         "expand": "body.storage,version", "limit": self.page_size}
        docs: list[Doc] = []
        pages = 0
        t0 = time.monotonic()
        try:
            with httpx.Client(transport=self.transport, timeout=self.timeout_s,
                              headers={"Authorization": f"Bearer {self.token}", "Accept": "application/json"}) as client:
                for _ in range(MAX_PAGES_REQUESTS):
                    if url is None:
                        break
                    resp = client.get(url, params=params)
                    if resp.status_code >= 400:
                        raise SourceError(f"confluence HTTP {resp.status_code}")
                    data = resp.json()
                    links = data.get("_links") or {}
                    link_base = str(links.get("base") or base).rstrip("/")
                    for page in data.get("results") or []:
                        docs.extend(page_to_docs(page, link_base))
                        pages += 1
                    nxt = links.get("next")
                    # "next" is relative to the base and already carries the query string
                    url, params = (nxt if nxt.startswith("http") else link_base + nxt, None) if nxt else (None, None)
        except httpx.HTTPError as e:
            raise SourceError(f"confluence {type(e).__name__}") from e
        except ValueError as e:  # malformed JSON
            raise SourceError("confluence malformed response") from e
        log.info("confluence loaded pages=%d chunks=%d latency_ms=%d", pages, len(docs), (time.monotonic() - t0) * 1000)
        return docs


def make_source(env: Mapping[str, str], kb_dir: Path) -> KbSource:
    kind = (env.get("KB_SOURCE") or "markdown").strip()
    if kind == "markdown":
        return MarkdownSource(kb_dir, wiki_url=(env.get("KB_WIKI_URL") or "").strip())
    if kind == "confluence":
        url, token, space = (env.get(n, "").strip() for n in ("CONFLUENCE_URL", "CONFLUENCE_TOKEN", "CONFLUENCE_SPACE"))
        missing = [n for n, v in (("CONFLUENCE_URL", url), ("CONFLUENCE_TOKEN", token),
                                  ("CONFLUENCE_SPACE", space)) if not v]
        if missing:
            raise ValueError(f"KB_SOURCE=confluence requires {', '.join(missing)}")
        return ConfluenceSource(url, token, space)
    raise ValueError(f"KB_SOURCE must be markdown or confluence, got {kind!r}")


def rebuild_retriever(old: Retriever, docs: list[Doc]) -> Retriever:
    """Same kind of index over new chunks. Hybrid keeps its embedder: no second model load."""
    if isinstance(old, HybridRetriever):
        d = old.dense
        passage_prefix = "passage: " if d.query_prefix == "query: " else ""
        return HybridRetriever(BM25Retriever(docs), DenseRetriever(docs, d.embed, d.query_prefix, passage_prefix),
                               old.rrf_k, old.depth)
    return BM25Retriever(docs)


async def reload_kb(state: Any) -> dict[str, Any]:
    if not hasattr(state, "kb_reload_lock"):
        state.kb_reload_lock = asyncio.Lock()
    async with state.kb_reload_lock:
        t0 = time.monotonic()
        source: KbSource = state.kb_source
        docs = await asyncio.to_thread(source.load)
        if not docs:  # an empty space or a wrong filter must not wipe a working index
            raise SourceError("source returned no documents")
        state.retriever = await asyncio.to_thread(rebuild_retriever, state.retriever, docs)
        result = {"documents": len({d.id for d in docs}), "chunks": len(docs), "source": source.name}
        log.info("kb reloaded source=%s documents=%d chunks=%d latency_ms=%d", source.name, result["documents"],
                 result["chunks"], (time.monotonic() - t0) * 1000)
        return result


def reload_interval(env: Mapping[str, str] = os.environ) -> float:
    return float(env.get("KB_RELOAD_SECONDS") or 0)


async def periodic_reload(app: FastAPI, seconds: float) -> None:
    while True:
        await asyncio.sleep(seconds)
        try:
            await reload_kb(app.state)
        except Exception as e:  # keep serving the old index; try again next time
            log.warning("periodic kb reload failed: %s", type(e).__name__)


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    seconds = reload_interval()
    task = asyncio.create_task(periodic_reload(app, seconds)) if seconds > 0 else None
    try:
        yield
    finally:
        if task:
            task.cancel()


router = APIRouter(lifespan=lifespan)


@router.post("/v1/kb/reload")
async def kb_reload(request: Request):
    try:
        return await reload_kb(request.app.state)
    except SourceError as e:
        log.warning("kb reload failed: %s", e)
        return JSONResponse(status_code=502, content={"error": "kb_source_unavailable"})

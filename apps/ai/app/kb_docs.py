"""One knowledge-base document for the reader in the chat ("open the source").

The index stores '## ' sections as chunks that share the document id; this joins them back in order.
Chunk titles are "<document> — <section>", so the section heading is the part after the first " — ".
"""
from __future__ import annotations

import json
from pathlib import Path

from fastapi import APIRouter, Request
from fastapi.responses import JSONResponse

from .retrieval import Doc

router = APIRouter()


def indexed_docs(retriever: object) -> list[Doc]:
    docs = getattr(retriever, "docs", None)
    if docs is None:  # hybrid: the lexical index holds the same chunks
        docs = getattr(getattr(retriever, "lexical", None), "docs", None)
    return list(docs or [])


def assemble(doc_id: str, chunks: list[Doc]) -> dict | None:
    parts = [c for c in chunks if c.id == doc_id]
    if not parts:
        return None
    title = parts[0].title.split(" — ", 1)[0]
    sections = [{"heading": c.title.split(" — ", 1)[1] if " — " in c.title else "", "text": c.text} for c in parts]
    return {"id": doc_id, "title": title, "url": parts[0].url.split("#", 1)[0],
            "updated": parts[0].updated, "sections": sections}


@router.get("/v1/kb/docs/{doc_id}")
def kb_doc(doc_id: str, request: Request):
    doc = assemble(doc_id, indexed_docs(request.app.state.retriever))
    if doc is None:
        return JSONResponse(status_code=404, content={"error": "not_found"})
    return doc


# ---- suggestions: ready-made questions the chat offers in the empty input (answered with a link, no LLM) ----

def _excerpt(text: str, limit: int = 420) -> str:
    text = text.strip()
    if len(text) <= limit:
        return text
    cut = text[:limit]
    end = max(cut.rfind(". "), cut.rfind("\n"))
    return (cut[: end + 1] if end > limit // 2 else cut.rsplit(" ", 1)[0] + "…").strip()


def load_suggestions(path: Path, chunks: list[Doc]) -> list[dict]:
    """Questions from data/suggestions.json whose document section really exists in the current index.

    The file is demo data like the KB; a company with its own wiki ships its own list. Entries that point
    to a missing document or section are dropped, so the chat never offers a hint it cannot show.
    """
    try:
        items = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return []
    out = []
    for item in items if isinstance(items, list) else []:
        doc = assemble(str(item.get("doc", "")), chunks)
        section = next((s for s in (doc or {}).get("sections", []) if s["heading"] == item.get("heading")), None)
        if not doc or not section or not str(item.get("question", "")).strip():
            continue
        out.append({"question": item["question"].strip(), "doc_id": doc["id"], "title": doc["title"],
                    "heading": section["heading"], "url": doc["url"], "excerpt": _excerpt(section["text"])})
    return out


@router.get("/v1/suggestions")
def suggestions(request: Request):
    path = Path(getattr(request.app.state, "data_dir", ".")) / "suggestions.json"
    return {"suggestions": load_suggestions(path, indexed_docs(request.app.state.retriever))}

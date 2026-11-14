"""One knowledge-base document for the reader in the chat ("open the source").

The index stores '## ' sections as chunks that share the document id; this joins them back in order.
Chunk titles are "<document> — <section>", so the section heading is the part after the first " — ".
"""
from __future__ import annotations

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

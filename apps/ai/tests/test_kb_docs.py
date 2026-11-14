from pathlib import Path

from fastapi.testclient import TestClient

from app.kb_docs import assemble, indexed_docs
from app.main import create_app
from app.retrieval import BM25Retriever, Doc, load_markdown_kb

KB = Path(__file__).resolve().parents[3] / "data" / "kb"


def test_assemble_joins_sections_in_order():
    chunks = [Doc("a", "Док — Раз", "u#x", "один", updated="2026-01-01"), Doc("b", "Другой", "v", "x"),
              Doc("a", "Док — Два", "u#y", "два")]
    doc = assemble("a", chunks)
    assert doc == {"id": "a", "title": "Док", "url": "u", "updated": "2026-01-01",
                   "sections": [{"heading": "Раз", "text": "один"}, {"heading": "Два", "text": "два"}]}
    assert assemble("missing", chunks) is None


def test_indexed_docs_reads_hybrid_lexical_index():
    class Hybrid:
        lexical = BM25Retriever([Doc("a", "t", "u", "x")])
    assert [d.id for d in indexed_docs(Hybrid())] == ["a"]
    assert indexed_docs(object()) == []


def test_wiki_url_rewrites_markdown_sources():
    docs = load_markdown_kb(KB, "http://wiki.test/wiki/")
    assert docs and all(d.url == f"http://wiki.test/wiki/{d.id}.html" for d in docs)


def test_endpoint_returns_document_and_404(monkeypatch):
    monkeypatch.setenv("LLM_PROVIDER", "mock")
    client = TestClient(create_app(retriever=BM25Retriever(load_markdown_kb(KB))))
    body = client.get("/v1/kb/docs/leave").json()
    assert body["id"] == "leave" and body["sections"] and body["title"]
    assert client.get("/v1/kb/docs/nope").status_code == 404

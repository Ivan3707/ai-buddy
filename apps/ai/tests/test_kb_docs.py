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


def test_suggestions_come_from_real_sections_only(tmp_path, monkeypatch):
    from app.kb_docs import load_suggestions
    chunks = load_markdown_kb(KB, "http://wiki.test/wiki")
    path = tmp_path / "s.json"
    path.write_text('[{"question":"Как подключиться к VPN?","doc":"it-setup","heading":"VPN"},'
                    '{"question":"нет раздела","doc":"it-setup","heading":"Нет такого"},'
                    '{"question":"нет документа","doc":"missing","heading":"VPN"}]', encoding="utf-8")
    got = load_suggestions(path, chunks)
    assert [g["question"] for g in got] == ["Как подключиться к VPN?"]
    assert got[0]["url"] == "http://wiki.test/wiki/it-setup.html" and got[0]["excerpt"] and len(got[0]["excerpt"]) <= 421
    assert load_suggestions(tmp_path / "missing.json", chunks) == []


def test_shipped_suggestions_all_resolve():
    from app.kb_docs import load_suggestions
    data = Path(__file__).resolve().parents[3] / "data" / "suggestions.json"
    expected = len(__import__("json").loads(data.read_text(encoding="utf-8")))
    assert len(load_suggestions(data, load_markdown_kb(KB))) == expected


def test_suggestions_endpoint(monkeypatch):
    monkeypatch.setenv("LLM_PROVIDER", "mock")
    client = TestClient(create_app(retriever=BM25Retriever(load_markdown_kb(KB))))
    body = client.get("/v1/suggestions").json()
    assert body["suggestions"] and {"question", "doc_id", "title", "heading", "url", "excerpt"} <= set(body["suggestions"][0])

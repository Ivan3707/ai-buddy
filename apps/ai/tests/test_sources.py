"""C1: KB sources (Markdown, Confluence via httpx.MockTransport) and POST /v1/kb/reload."""
import time

import httpx
import pytest
from fastapi.testclient import TestClient

from app import sources
from app.config import Settings
from app.llm import TextDelta
from app.main import DATA, create_app
from app.retrieval import BM25Retriever, DenseRetriever, Doc, HybridRetriever
from app.sources import (ConfluenceSource, MarkdownSource, SourceError, make_source, rebuild_retriever,
                         storage_to_sections)
from conftest import parse_sse

PAGE_1 = {
    "id": "101", "title": "Парковка",
    "body": {"storage": {"value": (
        "<p>Вводный абзац&nbsp;про офис.</p>"
        "<h2>Где парковаться</h2><p>Гостевая парковка — уровень <strong>-2</strong>.</p>"
        "<ul><li>Пропуск заказывает офис-менеджер</li><li></li></ul>"
        "<ac:structured-macro ac:name=\"info\"><ac:parameter ac:name=\"title\">скрыто</ac:parameter>"
        "<ac:rich-text-body><p>Мест мало, приезжай до 10:00.</p></ac:rich-text-body></ac:structured-macro>"
        "<h2>Пустой раздел</h2>"
        "<h2>Велосипеды</h2><ac:plain-text-body><![CDATA[Велопарковка у входа]]></ac:plain-text-body>")}},
    "version": {"when": "2026-10-01T12:00:00.000+03:00"},
    "_links": {"webui": "/display/DEMO/Parking"},
}
PAGE_2 = {"id": "102", "title": "Без разделов", "body": {"storage": {"value": "<p>Один абзац.</p>"}},
          "version": {}, "_links": {"webui": "https://wiki.example/x/102"}}
PAGE_EMPTY = {"id": "103", "title": "Пустая"}


def confluence_transport(pages, seen=None, status=200):
    def handler(request: httpx.Request) -> httpx.Response:
        if seen is not None:
            seen.append(request)
        if status != 200:
            return httpx.Response(status, json={"message": "nope"})
        start = int(request.url.params.get("start", "0"))
        body = {"results": pages[start:start + 1], "_links": {"base": "https://wiki.example/confluence"}}
        if start + 1 < len(pages):
            body["_links"]["next"] = f"/rest/api/content?spaceKey=DEMO&limit=1&start={start + 1}"
        return httpx.Response(200, json=body)
    return httpx.MockTransport(handler)


def test_storage_html_is_split_by_h2():
    sections = storage_to_sections(PAGE_1["body"]["storage"]["value"])
    assert sections == [
        ("", "Вводный абзац про офис."),
        ("Где парковаться", "Гостевая парковка — уровень -2.\n- Пропуск заказывает офис-менеджер\n"
                            "Мест мало, приезжай до 10:00."),
        ("Велосипеды", "Велопарковка у входа"),
    ]


def test_confluence_source_paginates_and_maps_pages():
    seen = []
    src = ConfluenceSource("https://wiki.example/confluence/", "secret", "DEMO",
                           transport=confluence_transport([PAGE_1, PAGE_2, PAGE_EMPTY], seen))
    docs = src.load()
    first = seen[0]
    assert first.headers["Authorization"] == "Bearer secret"
    assert first.url.path == "/confluence/rest/api/content"
    assert dict(first.url.params) == {"spaceKey": "DEMO", "type": "page", "expand": "body.storage,version",
                                      "limit": "50"}
    assert len(seen) == 3 and seen[2].url.params["start"] == "2"
    assert [d.title for d in docs] == ["Парковка", "Парковка — Где парковаться", "Парковка — Велосипеды",
                                       "Без разделов"]
    assert docs[0].id == "confluence-101" and docs[0].updated == "2026-10-01"
    assert docs[0].url == "https://wiki.example/confluence/display/DEMO/Parking"
    assert docs[3].url == "https://wiki.example/x/102" and docs[3].updated == ""


@pytest.mark.parametrize("transport", [
    confluence_transport([], status=401),
    httpx.MockTransport(lambda r: httpx.Response(200, text="<html>login</html>")),
    httpx.MockTransport(lambda r: (_ for _ in ()).throw(httpx.ConnectError("down"))),
])
def test_confluence_errors_become_source_error(transport):
    with pytest.raises(SourceError):
        ConfluenceSource("https://wiki.example", "t", "DEMO", transport=transport).load()


def test_make_source():
    kb = DATA / "kb"
    assert make_source({}, kb) == MarkdownSource(kb)
    src = make_source({"KB_SOURCE": "confluence", "CONFLUENCE_URL": "https://w", "CONFLUENCE_TOKEN": "t",
                       "CONFLUENCE_SPACE": "S"}, kb)
    assert isinstance(src, ConfluenceSource) and src.name == "confluence"
    with pytest.raises(ValueError, match="CONFLUENCE_TOKEN, CONFLUENCE_SPACE"):
        make_source({"KB_SOURCE": "confluence", "CONFLUENCE_URL": "https://w"}, kb)
    with pytest.raises(ValueError, match="markdown or confluence"):
        make_source({"KB_SOURCE": "notion"}, kb)


def test_rebuild_keeps_embedder_and_prefixes():
    docs = [Doc("a", "A", "u", "текст")]
    embed = lambda texts: [[1.0, float(t.startswith("passage: "))] for t in texts]  # noqa: E731
    old = HybridRetriever(BM25Retriever(docs), DenseRetriever(docs, embed, "query: ", "passage: "), rrf_k=5, depth=7)
    new = rebuild_retriever(old, docs + [Doc("b", "B", "u", "ещё")])
    assert isinstance(new, HybridRetriever) and new.dense.embed is embed and (new.rrf_k, new.depth) == (5, 7)
    assert new.dense.vectors[1][1] > 0  # passage prefix applied again
    assert isinstance(rebuild_retriever(BM25Retriever(docs), docs), BM25Retriever)


class FakeSource:
    name = "confluence"

    def __init__(self, docs, error=None):
        self.docs, self.error = docs, error

    def load(self):
        if self.error:
            raise self.error
        return self.docs


class SpyLLM:
    def __init__(self):
        self.hits = []

    async def stream(self, turn):
        self.hits = [h.doc.id for h in turn.hits]
        yield TextDelta("ok")


def test_reload_swaps_index_used_by_chat(chat_body):
    llm = SpyLLM()
    app = create_app(Settings(rag_mode="bm25"), llm=llm)
    client = TestClient(app)
    assert client.post("/v1/kb/reload").json() == {"documents": 18, "chunks": len(app.state.retriever.docs),
                                                   "source": "markdown"}
    app.state.kb_source = FakeSource([Doc("confluence-1", "Парковка", "https://w/1",
                                          "Гостевая парковка на уровне -2, пропуск заказывает офис-менеджер.")])
    assert client.post("/v1/kb/reload").json() == {"documents": 1, "chunks": 1, "source": "confluence"}
    events = dict(parse_sse(client.post("/v1/chat/stream", json=chat_body("Где гостевая парковка?")).text))
    assert "done" in events and llm.hits == ["confluence-1"]


@pytest.mark.parametrize("source", [FakeSource([]), FakeSource([], error=SourceError("down"))])
def test_failed_or_empty_reload_keeps_old_index(source):
    app = create_app(Settings(rag_mode="bm25"))
    old = app.state.retriever
    app.state.kb_source = source
    r = TestClient(app).post("/v1/kb/reload")
    assert r.status_code == 502 and r.json() == {"error": "kb_source_unavailable"}
    assert app.state.retriever is old


def test_periodic_reload(monkeypatch):
    monkeypatch.setenv("KB_RELOAD_SECONDS", "0.02")
    app = create_app(Settings(rag_mode="bm25"))
    old = app.state.retriever
    with TestClient(app):
        app.state.kb_source = FakeSource([], error=SourceError("down"))  # a failure does not stop the loop
        time.sleep(0.1)
        app.state.kb_source = MarkdownSource(DATA / "kb")
        deadline = time.monotonic() + 2
        while app.state.retriever is old and time.monotonic() < deadline:
            time.sleep(0.02)
    assert app.state.retriever is not old


def test_no_periodic_reload_by_default(monkeypatch):
    monkeypatch.delenv("KB_RELOAD_SECONDS", raising=False)
    assert sources.reload_interval() == 0
    app = create_app(Settings(rag_mode="bm25"))
    old = app.state.retriever
    with TestClient(app):
        time.sleep(0.05)
    assert app.state.retriever is old

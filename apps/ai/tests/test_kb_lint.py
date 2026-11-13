"""B5: GET /v1/kb/lint and the CLI (our synthetic KB, fake LLMs, no network)."""
import json
from datetime import date

import pytest
from fastapi.testclient import TestClient

from app import kb_lint
from app.config import Settings
from app.kb_lint import (LintOptions, candidates, conflicting_units, docs_of, lint_options, numeric_facts,
                         render_markdown, stale_issues)
from app.llm import LLMError, TextDelta
from app.main import DATA, create_app
from app.retrieval import BM25Retriever, DenseRetriever, Doc, HybridRetriever, load_markdown_kb

KB = load_markdown_kb(DATA / "kb")
PLANTED = {frozenset({"leave", "first-week"}), frozenset({"git-workflow", "code-review"})}
STALE = ["wiki-faq", "git-workflow", "first-week"]  # older than 365 days on 2026-11-13, oldest first


@pytest.fixture(autouse=True)
def demo_today(monkeypatch):
    monkeypatch.setenv("KB_TODAY", "2026-11-13")


class Judge:
    """Says 'contradiction' only for the planted pairs, like a good model would."""

    def __init__(self, error_for=None, raw=None):
        self.calls, self.error_for, self.raw = 0, error_for, raw

    async def stream(self, turn):
        self.calls += 1
        text = turn.messages[-1]["content"]
        if self.error_for and self.error_for in text:
            raise LLMError("timeout")
        if self.raw is not None:
            yield TextDelta(self.raw)
            return
        hit = ("Отгул" in text and "Памятка первой недели" in text) or ("Git и merge" in text and "Код-ревью —" in text)
        yield TextDelta(json.dumps({"contradiction": hit, "detail": "Расходятся сроки [demo]" if hit else ""},
                                   ensure_ascii=False))


def kinds(report, kind):
    return [i for i in report["issues"] if i["kind"] == kind]


def test_mock_lint_finds_planted_contradictions_and_stale_docs():
    report = TestClient(create_app(Settings(rag_mode="bm25"))).get("/v1/kb/lint").json()
    contradictions = kinds(report, "contradiction")
    assert {frozenset(i["doc_ids"]) for i in contradictions} == PLANTED
    leave = next(i for i in contradictions if "leave" in i["doc_ids"])
    assert "2 в «Отпуска" in leave["detail"] and "3 в «Памятка" in leave["detail"]
    assert len(leave["sources"]) == 2 and leave["sources"][0]["snippet"]
    stale = kinds(report, "stale")
    assert [i["doc_ids"] for i in stale] == [[d] for d in STALE]
    assert "порог — 365" in stale[0]["detail"] and "snippet" not in stale[0]["sources"][0]
    assert report["generated_at"].endswith("+00:00")


def test_llm_judges_capped_candidates_and_report_is_cached(monkeypatch):
    monkeypatch.setenv("KB_LINT_MAX_LLM_CALLS", "5")
    llm = Judge()
    app = create_app(Settings(provider="openai_compatible", model="m", rag_mode="bm25"), llm=llm)
    client = TestClient(app)
    report = client.get("/v1/kb/lint").json()
    assert llm.calls == 5
    assert {frozenset(i["doc_ids"]) for i in kinds(report, "contradiction")} == PLANTED
    assert {i["detail"] for i in kinds(report, "contradiction")} == {"Расходятся сроки [demo]"}
    assert client.get("/v1/kb/lint").json() == report and llm.calls == 5  # cached
    app.state.retriever = BM25Retriever(KB)  # what /v1/kb/reload does
    client.get("/v1/kb/lint")
    assert llm.calls == 10


def test_llm_failure_on_a_pair_falls_back_to_heuristic():
    llm = Judge(error_for="Памятка первой недели")
    report = TestClient(create_app(Settings(provider="openai_compatible", model="m", rag_mode="bm25"),
                                   llm=llm)).get("/v1/kb/lint").json()
    details = {"leave" in i["doc_ids"]: i["detail"] for i in kinds(report, "contradiction")}
    assert details[True].startswith("Разные числа") and details[False] == "Расходятся сроки [demo]"


@pytest.mark.parametrize("raw,expected", [('{"contradiction": true}', "Разные числа"), ("мусор", None)])
def test_unparsable_or_empty_llm_detail(raw, expected):
    report = TestClient(create_app(Settings(provider="openai_compatible", model="m", rag_mode="bm25"),
                                   llm=Judge(raw=raw))).get("/v1/kb/lint").json()
    contradictions = kinds(report, "contradiction")
    if expected is None:
        assert contradictions == []
    else:
        assert contradictions[0]["detail"].startswith(expected)
        assert any(i["detail"].endswith("противоречат друг другу.") for i in contradictions)


def test_dense_vectors_rank_fill_candidates():
    docs = [Doc("a", "A — x", "u", "Нужно 2 одобрения для мержа.", updated="2026-01-01"),
            Doc("b", "B — x", "u", "Для мержа достаточно одного одобрения.", updated="bad"),
            Doc("c", "C — x", "u", "Только по пятницам деплой.")]
    dense = DenseRetriever(docs, lambda texts: [[1.0, 0.0], [1.0, 0.1], [0.0, 1.0]][: len(texts)])
    hybrid = HybridRetriever(BM25Retriever(docs), dense)
    assert docs_of(hybrid) == docs
    pairs = candidates(docs_of(hybrid), dense.vectors)
    assert [(p.a.id, p.b.id) for p in pairs] == [("a", "b"), ("b", "c"), ("a", "c")]
    assert pairs[0].units == ("одобр",)
    assert "Подсказка" in kb_lint.judge_messages(pairs[0])[1]["content"]
    assert "Подсказка" not in kb_lint.judge_messages(pairs[1])[1]["content"]


def test_helpers():
    assert numeric_facts("не позже чем за 2 рабочих дня; одного одобрения; 1:1") == {"рабоч": {2}, "одобр": {1}}
    assert conflicting_units({"x": {1}, "y": {2}}, {"x": {1, 3}, "y": {3}}) == ["y"]
    opts = lint_options({"KB_STALE_DAYS": "30", "KB_TODAY": "2026-01-31"})
    assert opts == LintOptions(today=date(2026, 1, 31), stale_days=30, max_llm_calls=15)
    assert lint_options({}).today == date.today()
    assert stale_issues([Doc("x", "X", "u", "t", updated="")], opts) == []
    assert docs_of(object()) == []


def test_markdown_and_cli(capsys, monkeypatch):
    report = TestClient(create_app(Settings(rag_mode="bm25"))).get("/v1/kb/lint").json()
    md = render_markdown(kb_lint.KbLintReport.model_validate(report))
    assert "## Противоречия: 2" in md and "## Устаревшие документы: 3" in md
    monkeypatch.setenv("RAG_MODE", "bm25")
    monkeypatch.setenv("LLM_PROVIDER", "mock")
    kb_lint.main(["--kb-dir", str(DATA / "kb")])
    out = capsys.readouterr().out
    assert out.startswith("# Проверка базы знаний") and "first-week / leave" in out

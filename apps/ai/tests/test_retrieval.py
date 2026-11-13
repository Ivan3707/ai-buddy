"""Lexical, dense and hybrid retrievers with a fake embedder (no model download)."""
import hashlib

import pytest

import app.retrieval as retrieval
from app.config import load_settings
from app.retrieval import (BM25Retriever, DenseRetriever, Doc, HybridRetriever, LemmaTokenizer, build_retriever,
                           tokenize)

DOCS = [
    Doc("leave", "Отпуск", "u1", "Заявление на отпуск подают за 14 дней"),
    Doc("vpn", "VPN", "u2", "Как подключить VPN из дома"),
    Doc("standup", "Встречи", "u3", "Стендап каждый день в 10:30"),
]


def fake_embed(texts):
    """Bag of crude stems hashed into 32 dims: similar wording -> similar vectors."""
    out = []
    for t in texts:
        v = [0.0] * 32
        for tok in tokenize(t.removeprefix("query: ").removeprefix("passage: ")):
            v[int(hashlib.md5(tok.encode()).hexdigest(), 16) % 32] += 1.0
        out.append(v)
    return out


def test_dense_ranks_by_cosine_and_uses_prefixes():
    seen = []

    def spy(texts):
        seen.extend(texts)
        return fake_embed(texts)

    dense = DenseRetriever(DOCS, spy, "query: ", "passage: ")
    assert seen[0].startswith("passage: Отпуск.")
    hits = dense.search("отпуск заявление", 2, -1.0)
    assert hits[0].doc.id == "leave" and seen[-1] == "query: отпуск заявление"
    assert dense.search("   ", 2, -1.0) == [] and DenseRetriever([], spy).search("x", 2, 0.0) == []
    assert dense.search("отпуск", 3, 0.99) == [h for h in dense.search("отпуск", 3, 0.99) if h.score >= 0.99]


def test_hybrid_fuses_ranks_and_keeps_lexical_score():
    lexical = BM25Retriever(DOCS)
    hybrid = HybridRetriever(lexical, DenseRetriever(DOCS, fake_embed))
    hits = hybrid.search("стендап во сколько", 3, 0.0)
    assert hits[0].doc.id == "standup"
    assert hits[0].lexical > 0 and hits[0].score == pytest.approx(2 / 61)  # rank 1 in both rankers
    assert all(h.lexical == 0.0 for h in hits if h.doc.id != "standup")


def test_lemmatizer_merges_word_forms():
    lemma = LemmaTokenizer()
    assert lemma("отпуска отпуском") == ["отпуск", "отпуск"]
    assert lemma("стендап") != lemma("стенд")
    assert BM25Retriever(DOCS, lemma).search("отпуском", 1, 0.1)[0].doc.id == "leave"


def test_build_retriever(monkeypatch):
    assert isinstance(build_retriever(DOCS, "bm25", "any"), BM25Retriever)
    created = []

    class FakeFastEmbedder:
        def __init__(self, name):
            created.append(name)

        def __call__(self, texts):
            return fake_embed(texts)

    monkeypatch.setattr(retrieval, "FastEmbedder", FakeFastEmbedder)
    r = build_retriever(DOCS, "hybrid", "intfloat/multilingual-e5-small")
    assert isinstance(r, HybridRetriever) and r.dense.query_prefix == "query: "
    assert created == ["intfloat/multilingual-e5-small"]
    assert build_retriever(DOCS, "hybrid", "other/model").dense.query_prefix == ""


def test_rag_settings():
    s = load_settings({"RAG_MODE": "bm25", "RAG_EMBEDDING_MODEL": "m"})
    assert (s.rag_mode, s.embedding_model) == ("bm25", "m")
    assert load_settings({}).rag_mode == "hybrid"
    with pytest.raises(ValueError, match="RAG_MODE"):
        load_settings({"RAG_MODE": "magic"})

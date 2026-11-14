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


def test_follow_up_uses_previous_user_turn_and_topic_switch_still_works():
    from app.agent import retrieve
    from app.schemas import AgentChatRequest
    from conftest import user_context
    r = BM25Retriever(DOCS)
    history = [{"role": "user", "content": "Как оформить отпуск?"}, {"role": "assistant", "content": "Заявлением."}]
    follow_up = AgentChatRequest(message="а за сколько дней подают", history=history, user_context=user_context())
    assert retrieve(follow_up, r, 2, 0.1)[0].doc.id == "leave"
    assert retrieve(follow_up, r, 2, 0.1, use_history=False) == r.search("а за сколько дней подают", 2, 0.1)
    switch = AgentChatRequest(message="во сколько стендап", history=history, user_context=user_context())
    assert retrieve(switch, r, 1, 0.1)[0].doc.id == "standup"
    alone = AgentChatRequest(message="стендап", user_context=user_context())
    assert retrieve(alone, r, 1, 0.1)[0].doc.id == "standup"


def test_elliptical_follow_up_keeps_previous_turn_evidence():
    from app.agent import retrieve
    from app.schemas import AgentChatRequest
    from conftest import user_context
    r = BM25Retriever(DOCS)
    history = [{"role": "user", "content": "Как оформить отпуск?"}, {"role": "assistant", "content": "Заявлением."}]
    previous = {(h.doc.id, h.doc.title) for h in r.search("Как оформить отпуск?", 1, 0.1)}
    follow_up = AgentChatRequest(message="а во сколько стендап", history=history, user_context=user_context())
    got = {(h.doc.id, h.doc.title) for h in retrieve(follow_up, r, 1, 0.1)}
    assert previous <= got and len(got) == 2  # own top hit + the previous answer's evidence
    switch = AgentChatRequest(message="во сколько стендап", history=history, user_context=user_context())
    assert [h.doc.id for h in retrieve(switch, r, 1, 0.1)] == ["standup"]  # no carry-over on a topic switch


def test_page_context_steers_retrieval_and_is_masked_and_rendered():
    from app.agent import mask_request, retrieve
    from app.prompts import build_messages
    from app.schemas import AgentChatRequest
    from conftest import user_context
    r = BM25Retriever(DOCS)
    page = {"url": "https://grafana.demo.local/d/1", "title": "Стендап команды", "selection": "тел 89123456789"}
    req = AgentChatRequest(message="что тут смотреть?", user_context=user_context(), page_context=page)
    assert retrieve(req, r, 1, 0.1)[0].doc.id == "standup"
    masked, found = mask_request(req)
    assert masked.page_context.selection == "тел [телефон скрыт]" and found["phone"] == 1
    user_turn = build_messages(masked, [])[-1]["content"]
    assert "<page>" in user_turn and "заголовок: Стендап команды" in user_turn and "8912" not in user_turn
    assert "<page>" not in build_messages(AgentChatRequest(message="x", user_context=user_context()), [])[-1]["content"]
    with pytest.raises(ValueError):
        AgentChatRequest(message="x", user_context=user_context(), page_context={"url": "u", "html": "<b>"})


def test_page_is_weak_context_without_pointing_words():
    from app.agent import DEICTIC, retrieve
    from app.schemas import AgentChatRequest
    from conftest import user_context
    r = BM25Retriever(DOCS)
    page = {"url": "https://wiki/vpn", "title": "VPN из дома"}
    switch = AgentChatRequest(message="во сколько стендап", user_context=user_context(), page_context=page)
    assert retrieve(switch, r, 1, 0.1)[0].doc.id == "standup"
    about_page = AgentChatRequest(message="что тут важно", user_context=user_context(), page_context=page)
    assert retrieve(about_page, r, 1, 0.1)[0].doc.id == "vpn"
    assert DEICTIC.search("это ещё актуально?") and not DEICTIC.search("этаж и этап")

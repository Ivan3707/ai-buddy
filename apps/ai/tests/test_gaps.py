"""B4: POST /v1/analytics/gaps (token and vector clustering, LLM labels with fallback)."""
import json
import time

import pytest
from fastapi.testclient import TestClient

from app.config import Settings
from app.gaps import cluster_by_tokens, cluster_by_vectors, keyword_label
from app.llm import LLMError, TextDelta
from app.main import create_app
from app.retrieval import BM25Retriever, DenseRetriever, HybridRetriever, load_markdown_kb

QUESTIONS = [
    {"id": "1", "text": "Есть ли парковка у офиса?"},
    {"id": "2", "text": "Где парковка для сотрудников у офиса?"},
    {"id": "3", "text": "Как получить доступ к Kubernetes?"},
    {"id": "4", "text": "Есть ли парковка у офиса?"},
    {"id": "5", "text": "Помогите!"},
    {"id": "6", "text": "Есть ли парковка возле офиса?"},
]
mock_client = TestClient(create_app(Settings(rag_mode="bm25")))


class Labeler:
    def __init__(self, reply="", error=None):
        self.reply, self.error, self.calls = reply, error, []

    async def stream(self, turn):
        self.calls.append(turn.messages)
        if self.error:
            raise self.error
        yield TextDelta(self.reply)


def llm_client(llm, retriever=None):
    return TestClient(create_app(Settings(provider="openai_compatible", model="m", rag_mode="bm25"),
                                 retriever=retriever, llm=llm))


def test_mock_report_groups_by_tokens_and_sorts_by_count():
    body = mock_client.post("/v1/analytics/gaps", json={"questions": QUESTIONS}).json()
    assert body["total_questions"] == 6
    top = body["clusters"][0]
    assert top["count"] == 4 and top["question_ids"] == ["1", "2", "4", "6"]
    assert top["examples"] == ["Есть ли парковка у офиса?", "Где парковка для сотрудников у офиса?",
                               "Есть ли парковка возле офиса?"]  # duplicates dropped, at most 3
    assert top["label"].startswith("парковка, офиса")
    assert [c["count"] for c in body["clusters"]] == [4, 1, 1]
    assert {"Без темы", "получить, доступ, kubernetes"} == {c["label"] for c in body["clusters"][1:]}


def test_empty_request():
    assert mock_client.post("/v1/analytics/gaps", json={"questions": []}).json() == {"total_questions": 0,
                                                                                      "clusters": []}


def test_validation():
    for body in ({"questions": [{"id": "1"}]}, {"questions": [{"id": "1", "text": "x" * 2001}]},
                 {"questions": [{"id": str(i), "text": "x"} for i in range(2001)]}, {"questions": [], "x": 1}):
        assert mock_client.post("/v1/analytics/gaps", json=body).status_code == 422


def test_llm_labels_clusters_in_one_call():
    llm = Labeler(json.dumps({"labels": ["Парковка у офиса", " ", "Доступ к Kubernetes"]}, ensure_ascii=False))
    body = llm_client(llm).post("/v1/analytics/gaps", json={"questions": QUESTIONS}).json()
    assert len(llm.calls) == 1 and "Группа 3" in llm.calls[0][1]["content"]
    assert [c["label"] for c in body["clusters"]] == ["Парковка у офиса", "получить, доступ, kubernetes",
                                                      "Доступ к Kubernetes"]


@pytest.mark.parametrize("llm", [Labeler("не json"), Labeler('{"labels": ["одна"]}'),
                                 Labeler(error=LLMError("timeout"))])
def test_bad_or_failed_labels_fall_back_to_keywords(llm):
    body = llm_client(llm).post("/v1/analytics/gaps", json={"questions": QUESTIONS}).json()
    assert body["clusters"][0]["label"].startswith("парковка, офиса")


def test_hybrid_retriever_embedder_is_reused():
    docs = load_markdown_kb(__import__("app.main", fromlist=["DATA"]).DATA / "kb")[:3]
    seen = []

    def embed(texts):
        seen.extend(texts)
        return [[1.0, 0.0] if "парков" in t else [0.0, 1.0] for t in texts]

    hybrid = HybridRetriever(BM25Retriever(docs), DenseRetriever(docs, embed, query_prefix="query: "))
    seen.clear()
    body = llm_client(Labeler(error=LLMError("timeout")), hybrid).post(
        "/v1/analytics/gaps", json={"questions": QUESTIONS}).json()
    assert seen[0] == "query: Есть ли парковка у офиса?"
    assert [c["count"] for c in body["clusters"]] == [4, 2]


def test_vector_clustering_uses_mean_cosine():
    # c is close to a (0.9) but the mean over {a, b} is below the threshold -> new cluster
    a, b, c = [1.0, 0.0], [0.8, 0.6], [0.6, 0.8]
    assert cluster_by_vectors([a, b, c], threshold=0.85) == [0, 1, 1]
    assert cluster_by_vectors([a, a, c], threshold=0.5) == [0, 0, 0]


def test_token_clustering_is_fast_for_2000_questions():
    topics = ["парковка офис", "доступ kubernetes", "зарплата аванс", "пропуск гость", "монитор заказать"]
    texts = [f"вопрос про {topics[i % 5]} номер {i % 7}" for i in range(2000)]
    t0 = time.monotonic()
    labels = cluster_by_tokens(texts)
    assert time.monotonic() - t0 < 5
    assert len(set(labels)) <= 35
    assert keyword_label(["!", "?"]) == "Без темы"
